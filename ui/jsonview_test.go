package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/input"
)

func TestJSONSourceLazy(t *testing.T) {
	src, err := JSONSource([]byte(` {"a": [1, "x\"y", true, null, {"b": 2.5}], "k 1": {}, "e": []} `))
	if err != nil {
		t.Fatal(err)
	}
	roots := src.Roots()
	if len(roots) != 1 || roots[0].Kind != ValueObject || roots[0].Count != 3 {
		t.Fatalf("roots %+v", roots)
	}
	kids, _ := src.Children(&roots[0], 0, -1)
	if len(kids) != 3 || kids[0].Name != "a" || kids[0].Count != 5 || kids[1].Count != 0 || kids[2].Count != 0 {
		t.Fatalf("kids %+v", kids)
	}
	arr, _ := src.Children(&kids[0], 1, 4)
	if len(arr) != 3 || arr[0].Value != `x"y` || arr[0].Name != "1" || !arr[0].Index || arr[1].Kind != ValueBool || arr[2].Kind != ValueNull {
		t.Errorf("range %+v", arr)
	}
	if cp := src.(ValueCopier).CopyValue(&kids[0]); cp != `[1, "x\"y", true, null, {"b": 2.5}]` {
		t.Errorf("copy value %q", cp)
	}
	if _, err := JSONSource([]byte(`{"a":`)); err == nil {
		t.Error("invalid JSON must fail")
	}
}

func TestJSONSourceLargeArrayRanges(t *testing.T) {
	var items []string
	for i := 0; i < 1000; i++ {
		items = append(items, fmt.Sprint(i))
	}
	src, _ := JSONSource([]byte("[" + strings.Join(items, ",") + "]"))
	root := src.Roots()[0]
	if root.Count != 1000 {
		t.Fatal(root.Count)
	}
	kids, _ := src.Children(&root, 750, 753)
	if len(kids) != 3 || kids[0].Value != "750" || kids[2].Name != "752" {
		t.Errorf("%+v", kids)
	}
}

func TestGoValueSource(t *testing.T) {
	type inner struct {
		X int `json:"x"`
	}
	type outer struct {
		Name   string         `json:"name"`
		Skip   int            `json:"-"`
		M      map[string]int `json:"m"`
		P      *inner         `json:"p"`
		Nil    *inner         `json:"nil"`
		L      []string       `json:"l"`
		hidden int
	}
	src := GoValueSource(&outer{Name: "n", M: map[string]int{"b": 2, "a": 1}, P: &inner{7}, L: []string{"q"}})
	root := src.Roots()[0]
	if root.Kind != ValueObject || root.Count != 5 {
		t.Fatalf("root %+v", root)
	}
	kids, _ := src.Children(&root, 0, -1)
	names := []string{}
	for _, k := range kids {
		names = append(names, k.Name)
	}
	if strings.Join(names, ",") != "name,m,p,nil,l" {
		t.Errorf("fields %v", names)
	}
	m, _ := src.Children(&kids[1], 0, -1)
	if m[0].Name != "a" || m[1].Value != "2" {
		t.Errorf("map keys sorted %+v", m)
	}
	if kids[3].Kind != ValueNull || kids[2].Count != 1 {
		t.Errorf("pointers %+v %+v", kids[2], kids[3])
	}
}

type slowSource struct {
	ready bool
	asked int
}

func (s *slowSource) Roots() []ValueNode {
	return []ValueNode{{Name: "vars", Kind: ValueObject, Count: -1, Ref: 1}}
}

func (s *slowSource) Children(p *ValueNode, from, to int) ([]ValueNode, bool) {
	s.asked++
	if !s.ready {
		return nil, false
	}
	return []ValueNode{{Name: "x", Kind: ValueNumber, Value: "1", Type: "int"}, {Name: "s", Kind: ValueString, Value: "hi"}}, true
}

func TestJSONViewLazySource(t *testing.T) {
	src := &slowSource{}
	var w *JSONViewW
	d := NewDriver(func(c *Context) {
		w = JSONViewSource(c, src).Height(300).NoToolbar().ShowTypes()
	}, DriverOptions{Width: 600, Height: 400})
	d.Frame()
	if w.Rows() != 1 || src.asked == 0 {
		t.Fatalf("rows %d asked %d", w.Rows(), src.asked)
	}
	src.ready = true
	d.Advance(100e6)
	d.Settle()
	if w.Rows() != 3 {
		t.Errorf("rows after load = %d", w.Rows())
	}
	if _, ok := d.FindText("x 1"); !ok {
		t.Error("row label")
	}
}

func TestJSONViewGroupsAndKeyboard(t *testing.T) {
	var items []string
	for i := 0; i < 250; i++ {
		items = append(items, fmt.Sprintf(`{"id": %d}`, i))
	}
	data := []byte(`{"items": [` + strings.Join(items, ",") + `], "z": "end"}`)
	var w *JSONViewW
	d := NewDriver(func(c *Context) {
		w = JSONView(c, data).Height(300).ExpandDepth(2)
	}, DriverOptions{Width: 600, Height: 400})
	d.Frame()
	d.Frame()
	// root, items, 3 groups, z
	if w.Rows() != 6 {
		t.Fatalf("rows = %d", w.Rows())
	}
	if _, ok := d.FindText("[200 … 249]"); !ok {
		t.Error("group row")
	}
	d.ClickLabel("[100 … 199]")
	d.Key(input.KeyRight, 0)
	if w.Rows() != 106 {
		t.Errorf("expanded group rows = %d", w.Rows())
	}
	d.Key(input.KeyDown, 0)
	d.Key(input.KeyRight, 0)
	d.Key(input.KeyDown, 0)
	if w.Current() != "$.items[100].id" {
		t.Errorf("current = %q", w.Current())
	}
	d.Key(input.KeyC, input.ModCtrl|input.ModMeta|input.ModShift)
	if d.Clipboard() != "$.items[100].id" {
		t.Errorf("copy path = %q", d.Clipboard())
	}
	d.Key(input.KeyC, input.ModCtrl|input.ModMeta)
	if d.Clipboard() != "100" {
		t.Errorf("copy value = %q", d.Clipboard())
	}
	d.Key(input.KeyLeft, 0)
	if w.Current() != "$.items[100]" {
		t.Errorf("left goes to the parent: %q", w.Current())
	}
}

func TestJSONViewSearchReveals(t *testing.T) {
	data := []byte(`{"a": {"b": {"needle": 1}}, "c": ["x", "has needle"]}`)
	q := "needle"
	var w *JSONViewW
	d := NewDriver(func(c *Context) {
		w = JSONView(c, data).Height(300).ExpandDepth(0).Query(&q)
	}, DriverOptions{Width: 600, Height: 400})
	d.Frame()
	d.Frame()
	if w.Matches() != 2 {
		t.Fatalf("matches = %d", w.Matches())
	}
	if w.Current() != "$.a.b.needle" {
		t.Errorf("first match revealed: %q", w.Current())
	}
	d.ClickLabel("下一个匹配")
	if w.Current() != "$.c[1]" {
		t.Errorf("next match: %q", w.Current())
	}
	if _, ok := d.FindText("2/2"); !ok {
		t.Error("counter")
	}
}

func TestJSONViewContextMenuCopy(t *testing.T) {
	data := []byte(`{"k": "v"}`)
	d := NewDriver(func(c *Context) {
		JSONView(c, data).Height(300).NoToolbar()
	}, DriverOptions{Width: 600, Height: 400})
	d.Frame()
	d.Frame()
	r, ok := d.Find(`k "v"`)
	if !ok {
		t.Fatal("row not found")
	}
	d.Inject(input.Pointer{Kind: input.Move, Pos: r.Center()})
	d.Inject(input.Pointer{Kind: input.Press, Pos: r.Center(), Button: input.ButtonRight})
	d.Inject(input.Pointer{Kind: input.Release, Pos: r.Center(), Button: input.ButtonRight})
	d.Settle()
	if err := d.ClickLabel("复制路径"); err != nil {
		t.Fatal(err)
	}
	if d.Clipboard() != "$.k" {
		t.Errorf("clipboard %q", d.Clipboard())
	}
}

func TestJSONViewParseError(t *testing.T) {
	d := NewDriver(func(c *Context) { JSONView(c, []byte(`{"a":`)).Height(200) }, DriverOptions{Width: 600, Height: 300})
	d.Frame()
	if _, ok := d.FindText("JSON 解析失败"); !ok {
		t.Error("parse error shown")
	}
}

func BenchmarkJSONViewOpenLarge(b *testing.B) {
	var sb strings.Builder
	sb.WriteString("[")
	for i := 0; i < 200_000; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `{"id":%d,"name":"user %d","tags":["a","b"],"ok":true}`, i, i)
	}
	sb.WriteString("]")
	data := []byte(sb.String())
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := append([]byte(nil), data...) // a new slice: a fresh tree
		d := NewDriver(func(c *Context) { JSONView(c, buf).Height(600).ExpandDepth(2) }, DriverOptions{Width: 800, Height: 700})
		d.Frame()
	}
}
