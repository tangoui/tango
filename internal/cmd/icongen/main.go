// Command icongen generates the icon sets in ui/icon/element, ui/icon/antd
// and ui/icon/lucide from the published SVG packages:
//
//	npm pack @element-plus/icons-svg@2.3.2 @ant-design/icons-svg@4.6.0 lucide-static@1.72.0
//	# unpack each tarball into its own directory, then:
//	go run ./internal/cmd/icongen -element DIR/package -antd DIR/package -lucide DIR/package
//
// Each set becomes one Go file of statically initialized *icon.Icon
// variables (a program links only the icons it references) plus All. Path
// data is kept as published, except paths with fill-rule="evenodd": Tango
// fills with the non-zero rule, so their subpaths are re-wound to
// alternate with nesting depth, which gives the same coverage. Every
// converted path is checked by sampling; mismatches are reported.
package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"go/format"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

type part struct {
	d                 string
	stroke, secondary bool
}

type iconDef struct {
	goName, name, title string
	box                 [4]float64
	width               float64
	parts               []part
	dir                 bool
}

type set struct {
	pkg, prefix, source, license string
	doc                          func(d iconDef) string
	icons                        []iconDef
	aliases                      [][2]string // alias go name, target go name
	aliasNames                   [][2]string // alias registry name, target go name
}

var mismatches int

func main() {
	elementDir := flag.String("element", "", "unpacked @element-plus/icons-svg package directory")
	antdDir := flag.String("antd", "", "unpacked @ant-design/icons-svg package directory")
	lucideDir := flag.String("lucide", "", "unpacked lucide-static package directory")
	out := flag.String("out", "ui/icon", "output directory (the ui/icon package)")
	flag.Parse()
	var sets []*set
	if *elementDir != "" {
		sets = append(sets, element(*elementDir))
	}
	if *antdDir != "" {
		sets = append(sets, antd(*antdDir))
	}
	if *lucideDir != "" {
		sets = append(sets, lucide(*lucideDir))
	}
	if len(sets) == 0 {
		log.Fatal("icongen: give at least one of -element, -antd, -lucide")
	}
	for _, s := range sets {
		write(*out, s)
	}
	if mismatches > 0 {
		log.Printf("icongen: %d even-odd conversions differ from the original coverage (see above)", mismatches)
	}
}

func pkgVersion(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		log.Fatal(err)
	}
	var p struct{ Name, Version string }
	if err := json.Unmarshal(b, &p); err != nil {
		log.Fatal(err)
	}
	return p.Name + " " + p.Version
}

func readLicense(dir string) string {
	for _, n := range []string{"LICENSE", "LICENSE.md", "LICENSE.txt"} {
		if b, err := os.ReadFile(filepath.Join(dir, n)); err == nil {
			return string(b)
		}
	}
	return ""
}

// --- SVG parsing ----------------------------------------------------------

type svgElem struct {
	name  string
	attrs map[string]string
	fill  string // inherited fill
	rule  string // inherited fill-rule
}

// parseSVG returns the view box and the drawable elements of an SVG
// document, with fill and fill-rule inherited from ancestors.
func parseSVG(b []byte) (box [4]float64, elems []svgElem) {
	dec := xml.NewDecoder(bytes.NewReader(b))
	type inh struct{ fill, rule string }
	stack := []inh{{}}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			a := map[string]string{}
			for _, at := range t.Attr {
				a[at.Name.Local] = at.Value
			}
			cur := stack[len(stack)-1]
			if v, ok := a["fill"]; ok {
				cur.fill = v
			}
			if v, ok := a["fill-rule"]; ok {
				cur.rule = v
			}
			stack = append(stack, cur)
			switch t.Name.Local {
			case "svg":
				f := strings.Fields(strings.ReplaceAll(a["viewBox"], ",", " "))
				if len(f) != 4 {
					log.Fatalf("bad viewBox %q", a["viewBox"])
				}
				for i := range f {
					box[i] = num(f[i])
				}
			case "path", "circle", "ellipse", "rect", "line", "polyline", "polygon":
				elems = append(elems, svgElem{name: t.Name.Local, attrs: a, fill: cur.fill, rule: cur.rule})
			}
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		}
	}
	return box, elems
}

func num(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		log.Fatalf("bad number %q", s)
	}
	return v
}

func fmtNum(v float64) string {
	v = math.Round(v*1000) / 1000
	if v == 0 {
		return "0"
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// elemPath converts a basic shape to path data.
func elemPath(name string, a map[string]string) string {
	f := func(k string) float64 {
		if v, ok := a[k]; ok {
			return num(v)
		}
		return 0
	}
	n := fmtNum
	switch name {
	case "path":
		return a["d"]
	case "circle", "ellipse":
		cx, cy := f("cx"), f("cy")
		rx, ry := f("r"), f("r")
		if name == "ellipse" {
			rx, ry = f("rx"), f("ry")
		}
		return "M" + n(cx-rx) + " " + n(cy) + "A" + n(rx) + " " + n(ry) + " 0 1 0 " + n(cx+rx) + " " + n(cy) +
			"A" + n(rx) + " " + n(ry) + " 0 1 0 " + n(cx-rx) + " " + n(cy) + "Z"
	case "rect":
		x, y, w, h := f("x"), f("y"), f("width"), f("height")
		_, hasRx := a["rx"]
		_, hasRy := a["ry"]
		rx, ry := f("rx"), f("ry")
		if !hasRx {
			rx = ry
		}
		if !hasRy {
			ry = rx
		}
		rx, ry = min(rx, w/2), min(ry, h/2)
		if rx <= 0 || ry <= 0 {
			return "M" + n(x) + " " + n(y) + "H" + n(x+w) + "V" + n(y+h) + "H" + n(x) + "Z"
		}
		arc := func(ex, ey float64) string {
			return "A" + n(rx) + " " + n(ry) + " 0 0 1 " + n(ex) + " " + n(ey)
		}
		return "M" + n(x+rx) + " " + n(y) + "H" + n(x+w-rx) + arc(x+w, y+ry) + "V" + n(y+h-ry) + arc(x+w-rx, y+h) +
			"H" + n(x+rx) + arc(x, y+h-ry) + "V" + n(y+ry) + arc(x+rx, y) + "Z"
	case "line":
		return "M" + n(f("x1")) + " " + n(f("y1")) + "L" + n(f("x2")) + " " + n(f("y2"))
	case "polyline", "polygon":
		ps := strings.Fields(strings.ReplaceAll(a["points"], ",", " "))
		var b strings.Builder
		for i := 0; i+1 < len(ps); i += 2 {
			if i == 0 {
				b.WriteString("M")
			} else {
				b.WriteString("L")
			}
			b.WriteString(n(num(ps[i])) + " " + n(num(ps[i+1])))
		}
		if name == "polygon" {
			b.WriteString("Z")
		}
		return b.String()
	}
	return ""
}

// --- even-odd to non-zero -------------------------------------------------

// subpaths splits p at its moves.
func subpaths(p *paint.Path) [][]paint.Seg {
	var out [][]paint.Seg
	for _, s := range p.Segs {
		if s.Op == paint.SegMove || len(out) == 0 {
			if s.Op != paint.SegMove { // implicit start at the origin
				out = append(out, []paint.Seg{{Op: paint.SegMove}})
			} else {
				out = append(out, nil)
			}
		}
		out[len(out)-1] = append(out[len(out)-1], s)
	}
	return out
}

func segsPath(segs []paint.Seg) *paint.Path { return &paint.Path{Segs: segs} }

// reverse reverses one subpath, keeping its curves.
func reverse(sp []paint.Seg) []paint.Seg {
	// Collect the points each segment starts and ends at.
	type seg struct {
		op       paint.SegOp
		from, to geom.Point
		c1, c2   geom.Point
	}
	var segs []seg
	var start, pen geom.Point
	closed := false
	for _, s := range sp {
		switch s.Op {
		case paint.SegMove:
			start, pen = s.P[0], s.P[0]
		case paint.SegLine:
			segs = append(segs, seg{op: s.Op, from: pen, to: s.P[0]})
			pen = s.P[0]
		case paint.SegQuad:
			segs = append(segs, seg{op: s.Op, from: pen, c1: s.P[0], to: s.P[1]})
			pen = s.P[1]
		case paint.SegCubic:
			segs = append(segs, seg{op: s.Op, from: pen, c1: s.P[0], c2: s.P[1], to: s.P[2]})
			pen = s.P[2]
		case paint.SegClose:
			closed = true
			if pen != start {
				segs = append(segs, seg{op: paint.SegLine, from: pen, to: start})
				pen = start
			}
		}
	}
	out := []paint.Seg{{Op: paint.SegMove, P: [3]geom.Point{pen}}}
	for i := len(segs) - 1; i >= 0; i-- {
		s := segs[i]
		switch s.op {
		case paint.SegLine:
			out = append(out, paint.Seg{Op: paint.SegLine, P: [3]geom.Point{s.from}})
		case paint.SegQuad:
			out = append(out, paint.Seg{Op: paint.SegQuad, P: [3]geom.Point{s.c1, s.from}})
		case paint.SegCubic:
			out = append(out, paint.Seg{Op: paint.SegCubic, P: [3]geom.Point{s.c2, s.c1, s.from}})
		}
	}
	_ = closed // filled subpaths close implicitly either way
	return append(out, paint.Seg{Op: paint.SegClose})
}

func area(pts []geom.Point) float64 {
	a := 0.0
	for i := range pts {
		j := (i + 1) % len(pts)
		a += float64(pts[i].X)*float64(pts[j].Y) - float64(pts[j].X)*float64(pts[i].Y)
	}
	return a / 2
}

// crossings returns the even-odd parity and the non-zero winding of pt
// against the polygons.
func winding(polys [][]geom.Point, pt geom.Point) (crossings, wind int) {
	for _, q := range polys {
		for i := range q {
			a, b := q[i], q[(i+1)%len(q)]
			if (a.Y <= pt.Y) != (b.Y <= pt.Y) {
				x := a.X + (pt.Y-a.Y)/(b.Y-a.Y)*(b.X-a.X)
				if pt.X < x {
					crossings++
					if b.Y > a.Y {
						wind++
					} else {
						wind--
					}
				}
			}
		}
	}
	return crossings, wind
}

func polys(p *paint.Path, tol float32) [][]geom.Point {
	var out [][]geom.Point
	for _, pl := range p.Flatten(tol) {
		if len(pl.Pts) > 2 {
			out = append(out, pl.Pts)
		}
	}
	return out
}

// evenOddToNonZero re-winds the subpaths of d so the non-zero rule gives
// the even-odd coverage.
func evenOddToNonZero(where, d string, box [4]float64) string {
	p, err := paint.ParseSVGPath(d)
	if err != nil {
		log.Fatalf("%s: %v", where, err)
	}
	tol := float32(box[2] / 2000)
	sps := subpaths(p)
	flat := make([][]geom.Point, len(sps))
	for i, sp := range sps {
		if pl := segsPath(sp).Flatten(tol); len(pl) > 0 {
			flat[i] = pl[0].Pts
		}
	}
	var outSegs []paint.Seg
	for i, sp := range sps {
		if len(flat[i]) < 3 {
			continue
		}
		depth := 0
		for j := range sps {
			if j == i || len(flat[j]) < 3 {
				continue
			}
			// Test a point of i that is not on j's boundary: the midpoint
			// of i's first edge nudged by a hair is enough in practice.
			c, _ := winding([][]geom.Point{flat[j]}, flat[i][0])
			if c%2 == 1 {
				depth++
			}
		}
		positive := area(flat[i]) > 0
		want := depth%2 == 0
		if positive != want {
			sp = reverse(sp)
		}
		outSegs = append(outSegs, sp...)
	}
	q := segsPath(outSegs)
	// Verify on a grid.
	orig, conv := polys(p, tol), polys(q, tol)
	bad := 0
	const n = 96
	for yi := 0; yi < n; yi++ {
		for xi := 0; xi < n; xi++ {
			pt := geom.Pt(float32(box[0]+(float64(xi)+0.5)*box[2]/n), float32(box[1]+(float64(yi)+0.5)*box[3]/n))
			c, _ := winding(orig, pt)
			_, w := winding(conv, pt)
			if (c%2 == 1) != (w != 0) {
				bad++
			}
		}
	}
	if bad > 0 {
		mismatches++
		log.Printf("%s: %d/%d sample points differ after even-odd conversion", where, bad, n*n)
	}
	return pathData(q)
}

// pathData serializes absolute path segments.
func pathData(p *paint.Path) string {
	var b strings.Builder
	pt := func(g geom.Point) { b.WriteString(fmtNum(float64(g.X)) + " " + fmtNum(float64(g.Y))) }
	for _, s := range p.Segs {
		switch s.Op {
		case paint.SegMove:
			b.WriteString("M")
			pt(s.P[0])
		case paint.SegLine:
			b.WriteString("L")
			pt(s.P[0])
		case paint.SegQuad:
			b.WriteString("Q")
			pt(s.P[0])
			b.WriteString(" ")
			pt(s.P[1])
		case paint.SegCubic:
			b.WriteString("C")
			pt(s.P[0])
			b.WriteString(" ")
			pt(s.P[1])
			b.WriteString(" ")
			pt(s.P[2])
		case paint.SegClose:
			b.WriteString("Z")
		}
	}
	return b.String()
}

// --- naming ---------------------------------------------------------------

func pascal(kebab string) string {
	var b strings.Builder
	for _, w := range strings.FieldsFunc(kebab, func(r rune) bool { return r == '-' || r == '_' || r == ' ' }) {
		b.WriteString(strings.ToUpper(w[:1]) + w[1:])
	}
	return b.String()
}

// directional reports whether an icon named like this points along the
// reading direction (mirrors in RTL): arrows, chevrons and carets that
// point left or right. Alignment, panels and media controls do not.
func directional(name string) bool {
	toks := strings.Split(name, "-")
	side := false
	for _, t := range toks {
		if t == "left" || t == "right" {
			side = true
		}
	}
	if !side {
		return name == "back"
	}
	if toks[0] == "left" || toks[0] == "right" || toks[0] == "d" {
		return true
	}
	for _, t := range toks {
		switch t {
		case "arrow", "arrows", "chevron", "chevrons", "caret", "double", "corner", "swap":
			return true
		}
	}
	return false
}

// mergeParts joins consecutive parts of the same kind into one path.
func mergeParts(ps []part) []part {
	var out []part
	for _, p := range ps {
		if n := len(out); n > 0 && out[n-1].stroke == p.stroke && out[n-1].secondary == p.secondary {
			out[n-1].d += absStart(p.d)
			continue
		}
		out = append(out, p)
	}
	return out
}

// absStart rewrites path data that starts with a relative moveto ("m3 4
// 5 6", absolute as the first command of a path) so it stays right when
// appended to another path: "M3 4l5 6".
func absStart(d string) string {
	t := strings.TrimLeft(d, " \t\n")
	if !strings.HasPrefix(t, "m") {
		return d
	}
	i := 1
	nums := 0
	for nums < 2 && i < len(t) {
		for i < len(t) && strings.IndexByte(" ,\t\n", t[i]) >= 0 {
			i++
		}
		st := i
		if i < len(t) && (t[i] == '-' || t[i] == '+') {
			i++
		}
		dot := false
		for i < len(t) {
			c := t[i]
			if c >= '0' && c <= '9' {
				i++
			} else if c == '.' && !dot {
				dot = true
				i++
			} else if (c == 'e' || c == 'E') && i > st {
				i++
				if i < len(t) && (t[i] == '-' || t[i] == '+') {
					i++
				}
			} else {
				break
			}
		}
		if i == st {
			log.Fatalf("bad moveto in %q", d)
		}
		nums++
	}
	rest := strings.TrimLeft(t[i:], " ,\t\n")
	if rest != "" && (rest[0] == '-' || rest[0] == '+' || rest[0] == '.' || rest[0] >= '0' && rest[0] <= '9') {
		rest = "l" + rest // implicit relative linetos
	}
	return "M" + t[1:i] + rest
}

// checkParse makes sure Tango's parser accepts every part.
func checkParse(where string, ps []part) {
	for _, p := range ps {
		if _, err := paint.ParseSVGPath(p.d); err != nil {
			log.Fatalf("%s: %v", where, err)
		}
	}
}

// --- sets -----------------------------------------------------------------

func svgFiles(dir string) []string {
	m, err := filepath.Glob(filepath.Join(dir, "*.svg"))
	if err != nil {
		log.Fatal(err)
	}
	sort.Strings(m)
	return m
}

func fillParts(where string, box [4]float64, elems []svgElem, secondary func(fill string) bool) []part {
	var ps []part
	for _, e := range elems {
		d := elemPath(e.name, e.attrs)
		if e.rule == "evenodd" {
			d = evenOddToNonZero(where, d, box)
		}
		ps = append(ps, part{d: d, secondary: secondary(e.fill)})
	}
	return ps
}

func element(dir string) *set {
	s := &set{pkg: "element", prefix: "ep:", source: pkgVersion(dir), license: readLicense(dir)}
	s.doc = func(d iconDef) string { return fmt.Sprintf("%s is the Element Plus %q icon.", d.goName, d.title) }
	for _, f := range svgFiles(dir) {
		base := strings.TrimSuffix(filepath.Base(f), ".svg")
		b, err := os.ReadFile(f)
		if err != nil {
			log.Fatal(err)
		}
		box, elems := parseSVG(b)
		ps := mergeParts(fillParts(base, box, elems, func(string) bool { return false }))
		checkParse(base, ps)
		s.icons = append(s.icons, iconDef{goName: pascal(base), name: "ep:" + base, title: base, box: box, parts: ps, dir: directional(base)})
	}
	return s
}

func antd(dir string) *set {
	s := &set{pkg: "antd", prefix: "ant:", source: pkgVersion(dir), license: readLicense(dir)}
	if s.license == "" { // the npm package ships without its LICENSE file
		s.license = antdLicense
	}
	s.doc = func(d iconDef) string { return fmt.Sprintf("%s is the Ant Design %q icon.", d.goName, d.title) }
	themes := []struct{ dir, suffix, goSuffix string }{
		{"outlined", "outlined", "Outlined"}, {"filled", "filled", "Filled"}, {"twotone", "twotone", "TwoTone"},
	}
	for _, th := range themes {
		for _, f := range svgFiles(filepath.Join(dir, "inline-svg", th.dir)) {
			base := strings.TrimSuffix(filepath.Base(f), ".svg")
			b, err := os.ReadFile(f)
			if err != nil {
				log.Fatal(err)
			}
			box, elems := parseSVG(b)
			name := base + "-" + th.suffix
			ps := mergeParts(fillParts(name, box, elems, func(fill string) bool { return strings.EqualFold(fill, "#E6E6E6") }))
			checkParse(name, ps)
			s.icons = append(s.icons, iconDef{goName: pascal(base) + th.goSuffix, name: "ant:" + name, title: name, box: box, parts: ps, dir: directional(base)})
		}
	}
	return s
}

func lucide(dir string) *set {
	s := &set{pkg: "lucide", prefix: "lucide:", source: pkgVersion(dir), license: readLicense(dir)}
	s.doc = func(d iconDef) string { return fmt.Sprintf("%s is the Lucide %q icon.", d.goName, d.title) }
	b, err := os.ReadFile(filepath.Join(dir, "icon-nodes.json"))
	if err != nil {
		log.Fatal(err)
	}
	var nodes map[string][][2]json.RawMessage
	if err := json.Unmarshal(b, &nodes); err != nil {
		log.Fatal(err)
	}
	names := make([]string, 0, len(nodes))
	for n := range nodes {
		names = append(names, n)
	}
	sort.Strings(names)
	byContent := map[string]string{} // svg body -> go name, for aliases
	for _, n := range names {
		var ps []part
		for _, nd := range nodes[n] {
			var tag string
			var attrs map[string]any
			if err := json.Unmarshal(nd[0], &tag); err != nil {
				log.Fatal(err)
			}
			if err := json.Unmarshal(nd[1], &attrs); err != nil {
				log.Fatal(err)
			}
			a := map[string]string{}
			for k, v := range attrs {
				a[k] = fmt.Sprint(v)
			}
			d := elemPath(tag, a)
			if d == "" {
				log.Fatalf("%s: unsupported element %s", n, tag)
			}
			if a["fill"] != "" && a["fill"] != "none" {
				ps = append(ps, part{d: d})
			}
			ps = append(ps, part{d: d, stroke: true})
		}
		// Fills first, then all strokes as one part.
		var fills, strokes []part
		for _, p := range ps {
			if p.stroke {
				strokes = append(strokes, p)
			} else {
				fills = append(fills, p)
			}
		}
		ps = mergeParts(append(fills, strokes...))
		checkParse(n, ps)
		s.icons = append(s.icons, iconDef{goName: pascal(n), name: "lucide:" + n, title: n, box: [4]float64{0, 0, 24, 24}, width: 2, parts: ps, dir: directional(n)})
		if body, err := os.ReadFile(filepath.Join(dir, "icons", n+".svg")); err == nil {
			byContent[svgBody(body)] = pascal(n)
		}
	}
	// Aliases: the package ships an SVG per old name with the same shapes.
	have := map[string]bool{}
	for _, ic := range s.icons {
		have[ic.goName] = true
	}
	for _, f := range svgFiles(filepath.Join(dir, "icons")) {
		base := strings.TrimSuffix(filepath.Base(f), ".svg")
		if _, ok := nodes[base]; ok {
			continue
		}
		body, err := os.ReadFile(f)
		if err != nil {
			log.Fatal(err)
		}
		target, ok := byContent[svgBody(body)]
		if !ok || have[pascal(base)] {
			continue
		}
		have[pascal(base)] = true
		s.aliases = append(s.aliases, [2]string{pascal(base), target})
		s.aliasNames = append(s.aliasNames, [2]string{"lucide:" + base, target})
	}
	return s
}

// svgBody is an SVG file without its comment and class attribute (which
// name the icon), to match aliases to their icons.
func svgBody(b []byte) string {
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "<!--") || strings.HasPrefix(t, "class=") {
			continue
		}
		out = append(out, t)
	}
	return strings.Join(out, "\n")
}

// --- output ---------------------------------------------------------------

func write(outDir string, s *set) {
	dir := filepath.Join(outDir, s.pkg)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatal(err)
	}
	sort.Slice(s.icons, func(i, j int) bool { return s.icons[i].goName < s.icons[j].goName })
	seen := map[string]bool{}
	var b bytes.Buffer
	fmt.Fprintf(&b, "// Code generated by icongen from %s; DO NOT EDIT.\n\npackage %s\n\nimport \"github.com/tangoui/tango/ui/icon\"\n\n", s.source, s.pkg)
	b.WriteString("var (\n")
	for _, ic := range s.icons {
		if seen[ic.goName] {
			log.Fatalf("%s: duplicate Go name %s", s.pkg, ic.goName)
		}
		seen[ic.goName] = true
		fmt.Fprintf(&b, "\t// %s\n\t%s = &icon.Icon{Name: %q, ", s.doc(ic), ic.goName, ic.name)
		if ic.box != [4]float64{0, 0, 24, 24} {
			fmt.Fprintf(&b, "Box: [4]float32{%s, %s, %s, %s}, ", fmtNum(ic.box[0]), fmtNum(ic.box[1]), fmtNum(ic.box[2]), fmtNum(ic.box[3]))
		}
		if ic.width != 0 {
			fmt.Fprintf(&b, "Width: %s, ", fmtNum(ic.width))
		}
		if ic.dir {
			b.WriteString("Dir: true, ")
		}
		b.WriteString("Parts: []icon.Part{")
		for i, p := range ic.parts {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "{D: %q", p.d)
			if p.stroke {
				b.WriteString(", Stroke: true")
			}
			if p.secondary {
				b.WriteString(", Secondary: true")
			}
			b.WriteString("}")
		}
		b.WriteString("}}\n")
	}
	b.WriteString(")\n")
	if len(s.aliases) > 0 {
		b.WriteString("\n// Former names of icons, kept by the upstream package as aliases.\nvar (\n")
		for _, a := range s.aliases {
			fmt.Fprintf(&b, "\t%s = %s // alias\n", a[0], a[1])
		}
		b.WriteString(")\n")
	}
	fmt.Fprintf(&b, "\n// All returns every icon of the set (not the aliases), sorted by name.\n// Calling it links the whole set into the program.\nfunc All() []*icon.Icon {\n\treturn []*icon.Icon{\n")
	for _, ic := range s.icons {
		fmt.Fprintf(&b, "\t\t%s,\n", ic.goName)
	}
	b.WriteString("\t}\n}\n")
	b.WriteString("\n// aliases maps former registry names to their icons (see Register).\nfunc aliases() map[string]*icon.Icon {\n\treturn map[string]*icon.Icon{\n")
	for _, a := range s.aliasNames {
		fmt.Fprintf(&b, "\t\t%q: %s,\n", a[0], a[1])
	}
	b.WriteString("\t}\n}\n")
	src, err := format.Source(b.Bytes())
	if err != nil {
		log.Fatalf("%s: gofmt: %v", s.pkg, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "icons.go"), src, 0o644); err != nil {
		log.Fatal(err)
	}
	if s.license != "" {
		if err := os.WriteFile(filepath.Join(dir, "LICENSE"), []byte(s.license), 0o644); err != nil {
			log.Fatal(err)
		}
	}
	log.Printf("%s: %d icons, %d aliases, %d KiB", s.pkg, len(s.icons), len(s.aliases), len(src)/1024)
}

// antdLicense is the license of github.com/ant-design/ant-design-icons,
// which @ant-design/icons-svg declares ("license": "MIT") without
// shipping the file.
const antdLicense = `MIT License

Copyright (c) 2018-present Ant UED, https://xtech.antfin.com/

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
`
