// decksh: a little language that generates deck markup
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"strconv"
	"text/scanner"
)

// types of for loops
const (
	noloop = iota
	numloop
	fileloop
	vectloop
)
const doublequote = 0x22
const stdnotch = 0.75

// emap is the id=expression map
var emap = map[string]string{}

// xmlmap defines the XML substitutions
var xmlmap = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;")

// xmlesc escapes XML
func xmlesc(s string) string {
	return xmlmap.Replace(s)
}

// assign creates an assignment by filling in the global id map
func assign(s []string, linenumber int) error {
	switch len(s) {
	case 3:
		return simpleassign(s, linenumber)
	case 5:
		return binop(s, linenumber)
	default:
		return fmt.Errorf("line %d: %v is a illegal assignment", linenumber, s)
	}
}

// assign creates an simple assignment id=number
func simpleassign(s []string, linenumber int) error {
	if len(s) < 3 {
		return fmt.Errorf("line %d: assignment needs id=<expression>", linenumber)
	}
	emap[s[0]] = s[2]
	return nil
}

// binop processes a binary expression: id=id op number
func binop(s []string, linenumber int) error {
	es := fmt.Errorf("line %d: id=id operation number or id]", linenumber)
	if len(s) < 5 {
		return es
	}
	if s[1] != "=" {
		return es
	}
	target := s[0]
	ls := s[2]
	op := s[3]
	rs := s[4]
	
	lv, err := strconv.ParseFloat(eval(ls), 64)
	if err != nil {
		return fmt.Errorf("line %d: %v is not a number", linenumber, ls)
	}
	rv, err := strconv.ParseFloat(eval(rs), 64)
	if err != nil {
		return fmt.Errorf("line %d: %v is not a number", linenumber, rs)
	}
	switch op {
	case "+":
		emap[target] = fmt.Sprintf("%v", lv+rv)
	case "-":
		emap[target] = fmt.Sprintf("%v", lv-rv)
	case "*":
		emap[target] = fmt.Sprintf("%v", lv*rv)
	case "/":
		if rv == 0 {
			return fmt.Errorf("line %d: you cannot divide by zero (%v / %v)", linenumber, lv, rv)
		}
		emap[target] = fmt.Sprintf("%v", lv/rv)
	default:
		return es
	}
	return nil
}

// assignop creates an assignment by computing an addition or substraction on an identifier
func assignop(s []string, linenumber int) error {
	operr := fmt.Errorf("line %d:  id += number or id -= number", linenumber)
	if len(s) < 4 {
		return operr
	}
	
	e, err := strconv.ParseFloat(eval(s[0]), 64)
	if err != nil {
		return fmt.Errorf("line %d: %v is not a number", linenumber, s[0])
	}
	v, err := strconv.ParseFloat(s[3], 64)
	if err != nil {
		return fmt.Errorf("line %d: %v is not a number", linenumber, s[3])
	}

	switch s[1] {
	case "+":
		emap[s[0]] = fmt.Sprintf("%v", e+v)
	case "-":
		emap[s[0]] = fmt.Sprintf("%v", e-v)
	case "*":
		emap[s[0]] = fmt.Sprintf("%v", e*v)
	case "/":
		if v == 0 {
			return fmt.Errorf("line %d: you cannot divide by zero (%v / %v)", linenumber, e, v)
		}
		emap[s[0]] = fmt.Sprintf("%v", e/v)
	default:
		return operr
	}
	return nil
}

// eval evaluates an id string
func eval(s string) string {
	v, ok := emap[s]
	if ok {
		return v
	}
	return s
}

// parse takes a line of input and returns a string slice containing the parsed tokens
func parse(src string) []string {
	var s scanner.Scanner
	s.Init(strings.NewReader(src))

	tokens := []string{}
	for tok := s.Scan(); tok != scanner.EOF; tok = s.Scan() {
		tokens = append(tokens, s.TokenText())
	}
	for i := 1; i < len(tokens); i++ {
		tokens[i] = eval(tokens[i])
	}
	return tokens
}

// dumptokens show the parsed tokens
func dumptokens(w io.Writer, s []string, linenumber int) {
	fmt.Fprintf(w, "line %d: args=%d [ ", linenumber, len(s))
	for i, t := range s {
		fmt.Fprintf(w, "%d:%s ", i, t)
	}
	fmt.Fprintln(w, "]")
}

// deck produces the "deck" element
func deck(w io.Writer, s []string, linenumber int) error {
	_, err := fmt.Fprintln(w, "<deck>")
	return err
}

// canvas produces the "canvas" element
func canvas(w io.Writer, s []string, linenumber int) error {
	e := fmt.Errorf("line %d: %s width height", linenumber, s[0])
	if len(s) != 3 {
		return e
	}
	for i := 1; i < 3; i++ {
		s[i] = eval(s[i])
	}
	fmt.Fprintf(w, "<canvas width=%q height=%q/>\n", s[1], s[2])
	return nil
}

// slide produces the "slide" element
func slide(w io.Writer, s []string, linenumber int) error {
	switch len(s) {
	case 1:
		fmt.Fprintln(w, "<slide>")
	case 2:
		fmt.Fprintf(w, "<slide bg=%s>\n", s[1])
	case 3:
		fmt.Fprintf(w, "<slide bg=%s fg=%s>\n", s[1], s[2])
	default:
		return fmt.Errorf("line %d: slide [bgcolor] [fgcolor]", linenumber)
	}
	return nil
}

// elist ends a deck, slide, or list
func endtag(w io.Writer, s []string, linenumber int) error {
	tag := s[0]
	if len(tag) < 2 || tag[0:1] != "e" {
		return fmt.Errorf("line %d: edeck, eslide, or elist", linenumber)
	}
	fmt.Fprintf(w, "</%s>\n", tag[1:])
	return nil
}

// loadata creates a file using the  data keyword
func loadata(s []string, linenumber int, scanner *bufio.Scanner) error {
	if len(s) != 2 {
		return fmt.Errorf("line %d: data \"file\"...edata", linenumber)
	}
	filearg := s[1]
	end := len(filearg) - 1
	if len(filearg) < 3 {
		return fmt.Errorf("line %d: %v is not a valid filename", linenumber, filearg)
	}
	if filearg[0] != '"' && filearg[end] != '"' {
		return fmt.Errorf("line %d: %v is not a valid filename", linenumber, filearg)
	}
	dataw, err := os.Create(filearg[1:end])
	if err != nil {
		return fmt.Errorf("line %d: %v (%v)", linenumber, s, err)
	}
	for scanner.Scan() {
		t := scanner.Text()
		if strings.TrimSpace(t) == "edata" {
			break
		}
		f := strings.Fields(t)
		if len(f) != 2 {
			continue
		}
		fmt.Fprintf(dataw, "%v\t%v\n", f[0], f[1])
	}
	err = dataw.Close()
	return err
}

// fontColorOp generates markup for font, color, and opacity
func fontColorOp(s []string) string {
	switch len(s) {
	case 1:
		return fmt.Sprintf("font=%s", s[0])
	case 2:
		return fmt.Sprintf("font=%s color=%s", s[0], s[1])
	case 3:
		return fmt.Sprintf("font=%s color=%s opacity=%q", s[0], s[1], s[2])
	case 4:
		return fmt.Sprintf("font=%s color=%s opacity=%q link=%s", s[0], s[1], s[2], s[3])
	default:
		return ""
	}
}

// fontColorOpLp generates markup for font, color, and opacity and linespacing
func fontColorOpLp(s []string) string {
	switch len(s) {
	case 1:
		return fmt.Sprintf("font=%s", s[0])
	case 2:
		return fmt.Sprintf("font=%s color=%s", s[0], s[1])
	case 3:
		return fmt.Sprintf("font=%s color=%s opacity=%q", s[0], s[1], s[2])
	case 4:
		return fmt.Sprintf("font=%s color=%s opacity=%q lp=%q", s[0], s[1], s[2], s[3])
	case 5:
		return fmt.Sprintf("font=%s color=%s opacity=%q lp=%q link=%s", s[0], s[1], s[2], s[3], s[4])
	default:
		return ""
	}
}

func textattr(s string) string {
	f := strings.Split(s, "/")
	switch len(f) {
	case 1:
		return fmt.Sprintf("font=%q", f[0])
	case 2:
		return fmt.Sprintf("font=%q color=%q", f[0], f[1])
	case 3:
		return fmt.Sprintf("font=%q color=%q opacity=%q", f[0], f[1], f[2])
	default:
		return ""
	}
}

// remove quotes from a string, and XML escape it
func qesc(s string) string {
	if len(s) < 3 {
		return ""
	}
	return (xmlesc(s[1 : len(s)-1]))
}

// text generates markup for text
func text(w io.Writer, s []string, linenumber int) error {
	n := len(s)
	if n < 5 {
		return fmt.Errorf("line %d: %s \"text\" x y size [font] [color] [opacity] [link]", linenumber, s[0])
	}
	fco := fontColorOp(s[5:])
	switch s[0] {
	case "text":
		fmt.Fprintf(w, "<text xp=%q yp=%q sp=%q %s>%s</text>\n", s[2], s[3], s[4], fco, qesc(s[1]))
	case "ctext":
		fmt.Fprintf(w, "<text align=\"c\" xp=%q yp=%q sp=%q %s>%s</text>\n", s[2], s[3], s[4], fco, qesc(s[1]))
	case "etext":
		fmt.Fprintf(w, "<text align=\"e\" xp=%q yp=%q sp=%q %s>%s</text>\n", s[2], s[3], s[4], fco, qesc(s[1]))
	case "textfile":
		fmt.Fprintf(w, "<text file=%s xp=%q yp=%q sp=%q %s/>\n", s[1], s[2], s[3], s[4], fontColorOpLp(s[5:]))
	}
	return nil
}

// text generates markup for a block of text
func textblock(w io.Writer, s []string, linenumber int) error {
	if len(s) < 6 {
		return fmt.Errorf("line %d: %s \"text\" x y width size [font] [color] [opacity] [link]", linenumber, s[0])
	}
	fmt.Fprintf(w, "<text type=\"block\" xp=%q yp=%q wp=%q sp=%q %s>%s</text>\n", s[2], s[3], s[4], s[5], fontColorOp(s[6:]), qesc(s[1]))
	return nil
}

// textcode generates markup for a block of code
func textcode(w io.Writer, s []string, linenumber int) error {
	switch len(s) {
	case 6:
		fmt.Fprintf(w, "<text type=\"code\" file=%s xp=%q yp=%q wp=%q sp=%q/>\n", s[1], s[2], s[3], s[4], s[5])
	case 7:
		fmt.Fprintf(w, "<text type=\"code\" file=%s xp=%q yp=%q wp=%q sp=%q color=%s/>\n", s[1], s[2], s[3], s[4], s[5], s[6])
	default:
		return fmt.Errorf("line %d: %s \"file\" x y width size [color]", linenumber, s[0])
	}
	return nil
}

// image generates markup for images (plain and captioned)
func image(w io.Writer, s []string, linenumber int) error {
	n := len(s)
	e := fmt.Errorf("line %d: %s \"image-file\" x y w h [scale] [link]", linenumber, s[0])

	switch n {
	case 6:
		fmt.Fprintf(w, "<image name=%s xp=%q yp=%q width=%q height=%q/>\n", s[1], s[2], s[3], s[4], s[5])
	case 7:
		fmt.Fprintf(w, "<image name=%s xp=%q yp=%q width=%q height=%q scale=%q/>\n", s[1], s[2], s[3], s[4], s[5], s[6])
	case 8:
		fmt.Fprintf(w, "<image name=%s xp=%q yp=%q width=%q height=%q scale=%q link=%s/>\n", s[1], s[2], s[3], s[4], s[5], s[6], s[7])
	default:
		return e
	}
	return nil
}

// cimage makes a captioned image
func cimage(w io.Writer, s []string, linenumber int) error {
	n := len(s)
	e := fmt.Errorf("line %d: %s \"image-file\" \"caption\" x y w h [scale] [link]", linenumber, s[0])
	if n < 6 {
		return e
	}
	caption := xmlesc(s[2])
	switch n {
	case 7:
		fmt.Fprintf(w, "<image name=%s caption=%s xp=%q yp=%q width=%q height=%q/>\n", s[1], caption, s[3], s[4], s[5], s[6])
	case 8:
		fmt.Fprintf(w, "<image name=%s caption=%s xp=%q yp=%q width=%q height=%q scale=%q/>\n", s[1], caption, s[3], s[4], s[5], s[6], s[7])
	case 9:
		fmt.Fprintf(w, "<image name=%s caption=%s xp=%q yp=%q width=%q height=%q scale=%q link=%s/>\n", s[1], caption, s[3], s[4], s[5], s[6], s[7], s[8])
	default:
		return e
	}
	return nil
}

// list generates markup for lists
func list(w io.Writer, s []string, linenumber int) error {
	n := len(s)
	if n < 4 {
		return fmt.Errorf("line %d: %s x y size [font] [color] [opacity] [lp] [link]", linenumber, s[0])
	}
	var fco string
	if n > 4 {
		fco = fontColorOpLp(s[4:])
	}

	switch s[0] {
	case "list":
		fmt.Fprintf(w, "<list xp=%q yp=%q sp=%q %s>\n", s[1], s[2], s[3], fco)
	case "blist":
		fmt.Fprintf(w, "<list type=\"bullet\" xp=%q yp=%q sp=%q %s>\n", s[1], s[2], s[3], fco)
	case "nlist":
		fmt.Fprintf(w, "<list type=\"number\" xp=%q yp=%q sp=%q %s>\n", s[1], s[2], s[3], fco)
	}
	return nil
}

// listitem generates list items
func listitem(w io.Writer, s []string, linenumber int) error {
	ls := len(s)
	switch {
	case ls == 1:
		fmt.Fprintln(w, "<li/>")
	case ls == 2:
		fmt.Fprintf(w, "<li>%s</li>\n", qesc(s[1]))
	case ls > 2:
		fmt.Fprintf(w, "<li %s>%s</li>\n", fontColorOp(s[2:]), qesc(s[1]))
	}
	return nil
}

// shapes generates markup for rectangle and ellipse
func shapes(w io.Writer, s []string, linenumber int) error {
	n := len(s)
	e := fmt.Errorf("line %d: %s x y w h [color] [opacity]", linenumber, s[0])
	if n < 5 {
		return e
	}
	dim := fmt.Sprintf("xp=%q yp=%q wp=%q hp=%q", s[1], s[2], s[3], s[4])
	switch n {
	case 5:
		fmt.Fprintf(w, "<%s %s/>\n", s[0], dim)
	case 6:
		fmt.Fprintf(w, "<%s %s color=%s/>\n", s[0], dim, s[5])
	case 7:
		fmt.Fprintf(w, "<%s %s color=%s opacity=%q/>\n", s[0], dim, s[5], s[6])
	default:
		return e
	}
	return nil
}

// regshapes generates markup for square and circle
func regshapes(w io.Writer, s []string, linenumber int) error {
	n := len(s)
	e := fmt.Errorf("line %d: %s x y w [color] [opacity]", linenumber, s[0])
	if n < 4 {
		return e
	}
	switch s[0] {
	case "square":
		s[0] = "rect"
	case "circle":
		s[0] = "ellipse"
	}
	dim := fmt.Sprintf("xp=%q yp=%q wp=%q hr=\"100\"", s[1], s[2], s[3])
	switch n {
	case 4:
		fmt.Fprintf(w, "<%s %s/>\n", s[0], dim)
	case 5:
		fmt.Fprintf(w, "<%s %s color=%s/>\n", s[0], dim, s[4])
	case 6:
		fmt.Fprintf(w, "<%s %s color=%s opacity=%q/>\n", s[0], dim, s[4], s[5])
	default:
		return e
	}
	return nil
}

// polygon generates markup for polygons
func polygon(w io.Writer, s []string, linenumber int) error {
	n := len(s)
	e := fmt.Errorf("line %d: %s \"xcoord\" \"ycoord\" [color] [opacity]", linenumber, s[0])
	if n < 3 {
		return e
	}
	switch n {
	case 3:
		fmt.Fprintf(w, "<%s xc=%s yc=%s/>\n", s[0], s[1], s[2])
	case 4:
		fmt.Fprintf(w, "<%s xc=%s yc=%s color=%s/>\n", s[0], s[1], s[2], s[3])
	case 5:
		fmt.Fprintf(w, "<%s xc=%s yc=%s color=%s opacity=%q/>\n", s[0], s[1], s[2], s[3], s[4])
	default:
		return e
	}
	return nil
}

// line generates markup for lines
func line(w io.Writer, s []string, linenumber int) error {
	n := len(s)
	e := fmt.Errorf("line %d: %s x1 y1 x2 y2 [size] [color] [opacity]", linenumber, s[0])
	if n < 5 {
		return e
	}
	lc := fmt.Sprintf("xp1=%q yp1=%q xp2=%q yp2=%q", s[1], s[2], s[3], s[4])
	switch n {
	case 5:
		fmt.Fprintf(w, "<%s %s/>\n", s[0], lc)
	case 6:
		fmt.Fprintf(w, "<%s %s sp=%q/>\n", s[0], lc, s[5])
	case 7:
		fmt.Fprintf(w, "<%s %s sp=%q color=%s/>\n", s[0], lc, s[5], s[6])
	case 8:
		fmt.Fprintf(w, "<%s %s sp=%q color=%s opacity=%q/>\n", s[0], lc, s[5], s[6], s[7])
	default:
		return e
	}
	return nil
}

// hline makes a horizontal line
func hline(w io.Writer, s []string, linenumber int) error {
	e := fmt.Errorf("line %d: %s x y length [size] [color] [opacity]", linenumber, s[0])
	n := len(s)
	if n < 4 {
		return e
	}
	
	x1, err := strconv.ParseFloat(s[1], 64)
	if err != nil {
		return err
	}
	
	l, err := strconv.ParseFloat(s[3], 64)
	if err != nil {
		return err
	}
	lc := fmt.Sprintf("xp1=%q yp1=%q xp2=\"%v\" yp2=%q", s[1], s[2], x1+l, s[2])
	switch n {
	case 4:
		fmt.Fprintf(w, "<line %s/>\n", lc)
	case 5:
		fmt.Fprintf(w, "<line %s sp=%q/>\n", lc, s[4])
	case 6:
		fmt.Fprintf(w, "<line %s sp=%q color=%s/>\n", lc, s[4], s[5])
	case 7:
		fmt.Fprintf(w, "<line %s sp=%q color=%s opacity=%q/>\n", lc, s[4], s[5], s[6])
	default:
		return e
	}
	return nil
}

// vline makes a vertical line
func vline(w io.Writer, s []string, linenumber int) error {
	e := fmt.Errorf("line %d: %s x y length [size] [color] [opacity]", linenumber, s[0])
	n := len(s)
	if n < 4 {
		return e
	}
	
	y1, err := strconv.ParseFloat(s[2], 64)
	if err != nil {
		return err
	}
	l, err := strconv.ParseFloat(s[3], 64)
	if err != nil {
		return err
	}
	lc := fmt.Sprintf("xp1=%q yp1=%q xp2=%q yp2=\"%v\"", s[1], s[2], s[1], y1+l)
	switch n {
	case 4:
		fmt.Fprintf(w, "<line %s/>\n", lc)
	case 5:
		fmt.Fprintf(w, "<line %s sp=%q/>\n", lc, s[4])
	case 6:
		fmt.Fprintf(w, "<line %s sp=%q color=%s/>\n", lc, s[4], s[5])
	case 7:
		fmt.Fprintf(w, "<line %s sp=%q color=%s opacity=%q/>\n", lc, s[4], s[5], s[6])
	default:
		return e
	}
	return nil
}

// arc makes the markup for arc
func arc(w io.Writer, s []string, linenumber int) error {
	n := len(s)
	e := fmt.Errorf("line %d: %s cx cy w h a1 a2 [size] [color] [opacity]", linenumber, s[0])
	if n < 7 {
		return e
	}
	ac := fmt.Sprintf("xp=%q yp=%q wp=%q hp=%q a1=%q a2=%q", s[1], s[2], s[3], s[4], s[5], s[6])
	switch n {
	case 7:
		fmt.Fprintf(w, "<%s %s/>\n", s[0], ac)
	case 8:
		fmt.Fprintf(w, "<%s %s sp=%q/>\n", s[0], ac, s[7])
	case 9:
		fmt.Fprintf(w, "<%s %s sp=%q color=%s/>\n", s[0], ac, s[7], s[8])
	case 10:
		fmt.Fprintf(w, "<%s %s sp=%q color=%s opacity=%q/>\n", s[0], ac, s[7], s[8], s[9])
	default:
		return e
	}
	return nil
}

// curve make quadratic Bezier curve
func curve(w io.Writer, s []string, linenumber int) error {
	n := len(s)
	e := fmt.Errorf("line %d: %s x1 y1 x2 y2 x3 y3 [size] [color] [opacity]", linenumber, s[0])
	if n < 7 {
		return e
	}
	ac := fmt.Sprintf("xp1=%q yp1=%q xp2=%q yp2=%q xp3=%q yp3=%q", s[1], s[2], s[3], s[4], s[5], s[6])
	switch n {
	case 7:
		fmt.Fprintf(w, "<%s %s/>\n", s[0], ac)
	case 8:
		fmt.Fprintf(w, "<%s %s sp=%q/>\n", s[0], ac, s[7])
	case 9:
		fmt.Fprintf(w, "<%s %s sp=%q color=%s/>\n", s[0], ac, s[7], s[8])
	case 10:
		fmt.Fprintf(w, "<%s %s sp=%q color=%s opacity=%q/>\n", s[0], ac, s[7], s[8], s[9])
	default:
		return e
	}
	return nil
}

// legend makes the markup for the legend keyword
func legend(w io.Writer, s []string, linenumber int) error {
	n := len(s)
	if n < 7 {
		return fmt.Errorf("line %d: legend \"text\" x y size font color", linenumber)
	}
	
	tx, err := strconv.ParseFloat(s[2], 64)
	if err != nil {
		return err
	}
	cy, err := strconv.ParseFloat(s[3], 64)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "<text xp=%q yp=%q sp=%q %s>%s</text>\n", fmt.Sprintf("%.3f", tx+2), s[3], s[4], fontColorOp(s[5:]), qesc(s[1]))
	fmt.Fprintf(w, "<ellipse xp=%q yp=%q wp=%q hr=\"100\" color=%s/>\n", s[2], fmt.Sprintf("%.3f", cy+.5), s[4], s[6])
	return nil
}

func angle(x1, y1, x2, y2 float64) float64 {
	return math.Atan2(y2-y1, x2-x1)
}

func distance(x1, y1, x2, y2 float64) float64 {
	dx := x2 - x1
	dy := y2 - y1
	return math.Sqrt((dx * dx) + (dy * dy))
}

func rt(x1, y1, x2, y2 float64) (float64, float64) {
	dx := x2 - x1
	dy := y2 - y1
	return math.Sqrt((dx * dx) + (dy * dy)), math.Atan2(dy, dx)
}

func polar(cx, cy, r, t float64) (float64, float64) {
	return ((r * math.Cos(t)) + cx), ((r * math.Sin(t)) + cy)
}

func genarrow(x1, y1, x2, y2, aw, ah float64) (float64, float64, float64, float64, float64, float64, float64, float64) {
	r, t := rt(x1, y1, x2, y2)
	n := r - (aw * 0.75)
	nt := angle(x1, y1, x1+n, y1+(ah/2))
	ax1, ay1 := polar(x1, y1, r, t)
	ax2, ay2 := polar(x1, y1, r-aw, t+nt)
	ax3, ay3 := polar(x1, y1, n, t)
	ax4, ay4 := polar(x1, y1, r-aw, t-nt)

	return ax1, ay1, ax2, ay2, ax3, ay3, ax4, ay4
}

// arrow draws a general arrow given two points.
// The rotation of the arrowhead is computed.
func arrow(w io.Writer, s []string, linenumber int) error {
	ls := len(s)
	e := fmt.Errorf("line: %d arrow x1 y1 x2 y2 [linewidth] [arrowidth] [arrowheight] [color] [opacity]", linenumber)
	if ls < 5 {
		return e
	}
	aw := 3.0
	ah := 3.0
	lw := "0.2"
	color := `"gray"`
	opacity := "100"
	
	
	x1, err := strconv.ParseFloat(s[1], 64)
	if err != nil {
		return err
	}
	y1, err := strconv.ParseFloat(s[2], 64)
	if err != nil {
		return err
	}
	x2, err := strconv.ParseFloat(s[3], 64)
	if err != nil {
		return err
	}
	y2, err := strconv.ParseFloat(s[4], 64)
	if err != nil {
		return err
	}

	if ls >= 6 {
		lw = s[5] // linewidth
	}
	if ls >= 7 {
		aw, err = strconv.ParseFloat(s[6], 64)
		if err != nil {
			return err
		}
	}
	if ls >= 8 {
		ah, err = strconv.ParseFloat(s[7], 64)
		if err != nil {
			return err
		}
	}
	if ls >= 9 {
		color = s[8] // color
	}
	if ls == 10 {
		opacity = s[9] // opacity
	}
	ax1, ay1, ax2, ay2, ax3, ay3, ax4, ay4 := genarrow(x1, y1, x2, y2, aw, ah)
	fmt.Fprintf(w, "<line xp1=%q yp1=%q xp2=\"%v\" yp2=\"%v\" sp=%q color=%s opacity=%q/>\n", s[1], s[2], ax3, ay3, lw, color, opacity)
	fmt.Fprintf(w, "<polygon xc=\"%v %v %v %v\" yc=\"%v %v %v %v\" color=%s opacity=%q/>\n", ax1, ax2, ax3, ax4, ay1, ay2, ay3, ay4, color, opacity)
	return nil
}

// arrowhead returns the coordinates for left, right, up, down arrowheads.
// x, y is the point of the arrow, aw, ah are width, height
func arrowhead(x, y, ah, aw, notch float64, arrowtype byte) (float64, float64, float64, float64, float64, float64, float64, float64) {
	var ax1, ax2, ax3, ax4, ay1, ay2, ay3, ay4 float64
	switch arrowtype {
	case 'r':
		ax1 = x
		ax2 = ax1 - aw
		ax3 = x - (aw * notch)
		ax4 = ax2

		ay1 = y
		ay2 = y + (ah / 2)
		ay3 = y
		ay4 = y - (ah / 2)
	case 'l':
		ax1 = x
		ax2 = ax1 + aw
		ax3 = x + (aw * notch)
		ax4 = ax2

		ay1 = y
		ay2 = y + (ah / 2)
		ay3 = y
		ay4 = y - (ah / 2)
	case 'u':
		ax1 = x
		ax2 = x + (aw / 2)
		ax3 = x
		ax4 = x - (aw / 2)

		ay1 = y
		ay2 = ay1 - ah
		ay3 = ay1 - (ah * notch)
		ay4 = ay2
	case 'd':
		ax1 = x
		ax2 = x + (aw / 2)
		ax3 = x
		ax4 = x - (aw / 2)

		ay1 = y
		ay2 = ay1 + ah
		ay3 = ay1 + (ah * notch)
		ay4 = ay2
	}
	return ax1, ax2, ax3, ax4, ay1, ay2, ay3, ay4
}

// carrow makes a arrow with a curved line
func carrow(w io.Writer, s []string, linenumber int) error {
	ls := len(s)
	e := fmt.Errorf("line: %d [l|r|u|d]carrow x1 y1 x2 y2 x3 y3 [linewidth] [arrowidth] [arrowheight] [color] [opacity]", linenumber)
	if len(s[0]) < 7 {
		return e
	}
	if ls < 7 {
		return e
	}
	aw := 3.0
	ah := 3.0

	color := `"gray"`
	opacity := "100"

	// copy the curve portion
	curvestring := make([]string, 10)
	curvestring[0] = "curve"
	for i := 1; i < 7; i++ {
		curvestring[i] = s[i]
	}
	// set defaults for linewidth, color, and opacity
	curvestring[7] = "0.2"
	curvestring[8] = color
	curvestring[9] = opacity

	// override settings for  linewidth, color, and opacity
	if ls >= 8 {
		curvestring[7] = s[7] // linewidth
	}
	if ls >= 11 {
		color = s[10]
		curvestring[8] = color // color
	}
	if ls == 12 {
		opacity = s[11]
		curvestring[9] = opacity // opacity
	}

	// end point of the curve is the point of the arrow	
	x, err := strconv.ParseFloat(s[5], 64)
	if err != nil {
		return err
	}
	y, err := strconv.ParseFloat(s[6], 64)
	if err != nil {
		return nil
	}

	// override width and height of the arrow
	if ls >= 9 {
		aw, err = strconv.ParseFloat(s[8], 64)
		if err != nil {
			return err
		}
	}
	if ls >= 10 {
		ah, err = strconv.ParseFloat(s[9], 64)
		if err != nil {
			return err
		}
	}
	// compute the coordinates for the arrowhead
	ax1, ax2, ax3, ax4, ay1, ay2, ay3, ay4 := arrowhead(x, y, ah, aw, stdnotch, s[0][0])
	// adjust the end point of the curve to be the notch point
	curvestring[5] = fmt.Sprintf("%v", ax3)
	curvestring[6] = fmt.Sprintf("%v", ay3)

	curve(w, curvestring, linenumber)
	fmt.Fprintf(w, "<polygon xc=\"%v %v %v %v\" yc=\"%v %v %v %v\" color=%s opacity=%q/>\n", ax1, ax2, ax3, ax4, ay1, ay2, ay3, ay4, color, opacity)
	return nil
}

// chart runs the chart command
func chart(w io.Writer, s string, linenumber int) error {
	// copy the command line into fields, evaluating as we go
	args := strings.Fields(s)
	for i := 1; i < len(args); i++ {
		args[i] = eval(args[i])
		// unquote substituted strings
		la := len(args[i])
		if la > 2 && args[i][0] == doublequote && args[i][la-1] == doublequote {
			args[i] = args[i][1 : la-1]
		}
	}
	// glue the arguments back into a single string
	s = args[0]
	for i := 1; i < len(args); i++ {
		s = s + " " + args[i]
	}
	// separate again
	args = strings.Fields(s)

	// exec directly without the shell to avoid injection bugs
	name := args[0]
	cmd := &exec.Cmd{Path: name, Args: args}
	if filepath.Base(name) == name {
		lp, err := exec.LookPath(name)
		if err != nil {
			return fmt.Errorf("line: %d, %v - %v", linenumber, name, err)
		}
		cmd.Path = lp
	}
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("line: %d [%v] - %v", linenumber, s, err)
	}
	fmt.Fprintf(w, "%s\n", out)
	return err
}

// isaop tests for assignment operators
func isaop(s []string) bool {
	if len(s) < 4 {
		return false
	}
	op := s[1]
	if (op == "+" || op == "-" || op == "*" || op == "/") && s[2] == "=" {
		return true
	}
	return false
}

// fortype returns the type of for loop; either:
// for v = begin end incr
// for v = ["abc" "123"]
// for v = "file"
func fortype(s []string) int {
	n := len(s)
	// for x = ...
	if n < 4 || s[2] != "=" {
		return noloop
	}
	// for x = [...]
	if s[3] == "[" && s[len(s)-1] == "]" {
		return vectloop
	}
	// for x = "foo.d"
	if n == 4 && len(s[3]) > 3 && s[3][0] == doublequote && s[3][len(s[3])-1] == doublequote {
		return fileloop
	}
	// for x = begin end [increment]
	if n == 5 || n == 6 {
		return numloop
	}
	return noloop
}

// forvector returns the elements between "[" and "]"
func forvector(s []string) ([]string, error) {
	n := len(s)
	if n < 5 {
		return nil, fmt.Errorf("incomplete for: %v", s)
	}
	elements := make([]string, n-5)
	for i := 4; i < n-1; i++ {
		elements[i-4] = s[i]
	}
	return elements, nil
}

// forfile reads and returns the contents of the file in for x = "file"
func forfile(s []string) ([]string, error) {
	var contents []string
	fname := s[3][1 : len(s[3])-1] // remove quotes
	r, err := os.Open(fname)
	if err != nil {
		return contents, err
	}
	fs := bufio.NewScanner(r)
	for fs.Scan() {
		contents = append(contents, fs.Text())
	}
	return contents, fs.Err()
}

// fornum returns the arguments for for x=begin end [incr]
func fornum(s []string, linenumber int) (float64, float64, float64, error) {
	var incr float64
	if len(s) < 5 {
		return 0, -1, 0, fmt.Errorf("line %d: for begin end [incr] ... efor", linenumber)
	}
	
	begin, err := strconv.ParseFloat(s[3], 64)
	if  err != nil {
		return 0, -1, 0, err
	}
	end, err := strconv.ParseFloat(s[4], 64)
	if  err != nil {
		return 0, -1, 0, err
	}
	
	incr = 1.0
	if len(s) > 5 {
		var ierr error
		incr, ierr = strconv.ParseFloat(s[5], 64)
		if ierr != nil {
			return 0, -1, 0, ierr
		}
	}
	return begin, end, incr, nil
}

// forbody collects items within a for loop body
func forbody(scanner *bufio.Scanner) [][]string {
	elements := [][]string{}
	for scanner.Scan() {
		p := parse(scanner.Text())
		if len(p) < 1 {
			continue
		}
		if p[0] == "efor" {
			break
		}
		elements = append(elements, p)
	}
	return elements
}

// parsefor collects and evaluates a loop body
func parsefor(w io.Writer, s []string, linenumber int, scanner *bufio.Scanner) error {

	forvar := s[1]
	body := forbody(scanner)
	// determine the type of loop
	switch fortype(s) {
	case numloop:
		begin, end, incr, err := fornum(s, linenumber)
		if err != nil {
			return err
		}
		for v := begin; v <= end; v += incr {
			for _, fb := range body {
				evaloop(w, forvar, "%s", fmt.Sprintf("%v", v), fb, scanner, linenumber)
			}
		}
		return err
	case vectloop:
		vl, err := forvector(s)
		if err != nil {
			return err
		}
		for _, v := range vl {
			for _, fb := range body {
				evaloop(w, forvar, "\"%s\"", v, fb, scanner, linenumber)
			}
		}
		return err
	case fileloop:
		fl, err := forfile(s)
		if err != nil {
			return err
		}
		for _, v := range fl {
			for _, fb := range body {
				evaloop(w, forvar, "\"%s\"", v, fb, scanner, linenumber)
			}
		}
		return err
	default:
		return fmt.Errorf("line %d: incorrect for loop: %v", linenumber, s)
	}
}

// evaloop evaluates items in a loop body
func evaloop(w io.Writer, forvar string, format string, v string, s []string, scanner *bufio.Scanner, linenumber int) {
	e := make([]string, len(s))
	copy(e, s)
	for i := 0; i < len(s); i++ {
		if s[i] == forvar {
			e[i] = fmt.Sprintf(format, v)
		}
	}
	keyparse(w, e, "", scanner, linenumber)
}

// keyparse parses keywords and executes
func keyparse(w io.Writer, tokens []string, t string, sc *bufio.Scanner, n int) error {
	//fmt.Fprintf(os.Stderr, "%v\n", emap)
	switch tokens[0] {
	case "deck":
		return deck(w, tokens, n)

	case "canvas":
		return canvas(w, tokens, n)

	case "slide":
		return slide(w, tokens, n)

	case "text", "ctext", "etext", "textfile":
		return text(w, tokens, n)

	case "textblock":
		return textblock(w, tokens, n)

	case "textcode":
		return textcode(w, tokens, n)

	case "image":
		return image(w, tokens, n)

	case "cimage":
		return cimage(w, tokens, n)

	case "list", "blist", "nlist":
		return list(w, tokens, n)

	case "elist", "eslide", "edeck":
		return endtag(w, tokens, n)

	case "li":
		return listitem(w, tokens, n)

	case "ellipse", "rect":
		return shapes(w, tokens, n)

	case "circle", "square":
		return regshapes(w, tokens, n)

	case "polygon", "poly":
		return polygon(w, tokens, n)

	case "line":
		return line(w, tokens, n)

	case "arc":
		return arc(w, tokens, n)

	case "curve":
		return curve(w, tokens, n)

	case "legend":
		return legend(w, tokens, n)

	case "arrow":
		return arrow(w, tokens, n)

	// case "larrow", "rarrow", "uarrow", "darrow":
	//	return arrow(w, tokens, n)

	case "lcarrow", "rcarrow", "ucarrow", "dcarrow":
		return carrow(w, tokens, n)

	case "vline":
		return vline(w, tokens, n)

	case "hline":
		return hline(w, tokens, n)

	case "dchart", "chart":
		return chart(w, t, n)

	default: // not a keyword, process assignments
		if len(tokens) > 1 && tokens[1] == "=" {
			return assign(tokens, n)
		}
		if isaop(tokens) {
			return assignop(tokens, n)
		}
	}

	return nil
}

// process reads input, parses, dispatches functions for code generation
func process(w io.Writer, r io.Reader) error {
	scanner := bufio.NewScanner(r)
	errors := []error{}

	// For every line in the input, parse into tokens,
	// call the appropriate function, collecting errors as we go.
	// If any errors occurred, print them at the end, and return the latest
	for n := 1; scanner.Scan(); n++ {
		t := scanner.Text()
		tokens := parse(t)
		if len(tokens) < 1 || t[0] == '#' {
			continue
		}
		if tokens[0] == "for" {
			errors = append(errors, parsefor(w, tokens, n, scanner))
		}
		if tokens[0] == "data" {
			errors = append(errors, loadata(tokens, n, scanner))
		}
		errors = append(errors, keyparse(w, tokens, t, scanner, n))
	}
	// report any collected errors
	nerrs := 0
	for _, e := range errors {
		if e != nil {
			nerrs++
			fmt.Fprintf(os.Stderr, "%v\n", e)
		}
	}

	// handle read errors from scanning
	if err := scanner.Err(); err != nil {
		return err
	}

	// return the latest error
	if nerrs > 0 {
		return errors[nerrs-1]
	}

	// all is well, no errors
	return nil
}

// $ decksh                   # input from stdin, output to stdout
// $ decksh -o foo.xml        # input from stdin, output to foo.xml
// $ decksh foo.sh            # input from foo.sh output to stdout
// $ decksh -o foo.xml foo.sh # input from foo.sh output to foo.xml
func main() {
	var dest = flag.String("o", "", "output destination")
	var input io.ReadCloser = os.Stdin
	var output io.WriteCloser = os.Stdout
	var rerr, werr error

	flag.Parse()

	if len(flag.Args()) > 0 {
		input, rerr = os.Open(flag.Args()[0])
		if rerr != nil {
			fmt.Fprintf(os.Stderr, "%v\n", rerr)
			os.Exit(1)
		}
	}

	if len(*dest) > 0 {
		output, werr = os.Create(*dest)
		if werr != nil {
			fmt.Fprintf(os.Stderr, "%v\n", werr)
			os.Exit(2)
		}
	}

	err := process(output, input)
	if err != nil {
		os.Exit(3)
	}

	input.Close()
	output.Close()
	os.Exit(0)
}
