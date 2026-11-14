package vt

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// recorder serializes every Handler call into a string so that action
// sequences can be compared. Adjacent PrintASCII calls are merged, because
// the parser may split a printable run at any point.
type recorder struct {
	out   []string
	onCSI func(p *Params)
}

func (r *recorder) add(s string) { r.out = append(r.out, s) }

func (r *recorder) Print(c rune) { r.add(fmt.Sprintf("P(%U)", c)) }

func (r *recorder) PrintASCII(b []byte) {
	if n := len(r.out); n > 0 && strings.HasPrefix(r.out[n-1], "A(") {
		last := r.out[n-1]
		r.out[n-1] = last[:len(last)-1] + string(b) + ")"
		return
	}
	r.add("A(" + string(b) + ")")
}

func (r *recorder) Execute(b byte) { r.add(fmt.Sprintf("X(%02x)", b)) }

func (r *recorder) CSI(p *Params, inter []byte, final byte, private byte) {
	if r.onCSI != nil {
		r.onCSI(p)
	}
	priv := "0"
	if private != 0 {
		priv = string(private)
	}
	r.add(fmt.Sprintf("C(%s|%s|%c|%s)", fmtParams(p), inter, final, priv))
}

func (r *recorder) ESC(inter []byte, final byte) { r.add(fmt.Sprintf("E(%s|%c)", inter, final)) }

func (r *recorder) OSC(data []byte) { r.add(fmt.Sprintf("O(%q)", data)) }

func (r *recorder) DCS(p *Params, inter []byte, final byte, data []byte) {
	r.add(fmt.Sprintf("D(%s|%s|%c|%q)", fmtParams(p), inter, final, data))
}

func fmtParams(p *Params) string {
	parts := make([]string, p.Len())
	for i := range parts {
		var sub []string
		for _, v := range p.Sub(i) {
			sub = append(sub, fmt.Sprint(v))
		}
		parts[i] = strings.Join(sub, ":")
	}
	return strings.Join(parts, ";")
}

func record(chunks ...string) []string {
	r := &recorder{}
	p := NewParser(r)
	for _, c := range chunks {
		p.Write([]byte(c))
	}
	return r.out
}

type parserCase struct {
	name string
	in   string
	want []string
}

var parserCases = []parserCase{
	{"ascii", "hello", []string{"A(hello)"}},
	{"utf8", "a中😀", []string{"A(a)", "P(U+4E2D)", "P(U+1F600)"}},
	{"invalid", "a\xffb", []string{"A(a)", "P(U+FFFD)", "A(b)"}},
	{"truncated utf8 before control", "\xe4\xb8\n", []string{"P(U+FFFD)", "P(U+FFFD)", "X(0a)"}},
	{"sgr", "\x1b[1;31m", []string{"C(1;31||m|0)"}},
	{"private", "\x1b[?25l", []string{"C(25||l|?)"}},
	{"subparams", "\x1b[4:3m", []string{"C(4:3||m|0)"}},
	{"empty", "\x1b[m", []string{"C(||m|0)"}},
	{"default first param", "\x1b[;5H", []string{"C(0;5||H|0)"}},
	{"csi intermediate", "\x1b[2 q", []string{"C(2| |q|0)"}},
	{"osc bel", "\x1b]0;title\x07", []string{`O("0;title")`}},
	{"osc st", "\x1b]0;title\x1b\\", []string{`O("0;title")`}},
	{"osc utf8", "\x1b]2;中\x07x", []string{`O("2;中")`, "A(x)"}},
	{"esc", "\x1b7", []string{"E(|7)"}},
	{"esc intermediate", "\x1b(0", []string{"E((|0)"}},
	{"c0 inside csi", "\x1b[1\n;2H", []string{"X(0a)", "C(1;2||H|0)"}},
	{"can abort", "\x1b[1\x18m", []string{"X(18)", "A(m)"}},
	{"sub abort", "\x1b[1\x1am", []string{"X(1a)", "A(m)"}},
	{"esc restarts csi", "\x1b[1\x1b[2m", []string{"C(2||m|0)"}},
	{"csi ignore", "\x1b[1<2mX", []string{"A(X)"}},
	{"dcs", "\x1bP1$qm\x1b\\", []string{`D(1|$|q|"m")`}},
	{"dcs private", "\x1bP>|x\x1b\\", []string{`D(|>|||"x")`}},
	{"apc ignored", "\x1b_Gabc\x1b\\z", []string{"A(z)"}},
	{"pm ignored", "\x1b^abc\x1b\\z", []string{"A(z)"}},
	{"string esc then csi", "\x1b_x\x1b[1m", []string{"C(1||m|0)"}},
	{"del ignored in ground", "a\x7fb", []string{"A(ab)"}},
	{"c1 bytes are not controls", "\x9b1m", []string{"P(U+FFFD)", "A(1m)"}},
}

func runCases(t *testing.T, names ...string) {
	t.Helper()
	for _, c := range parserCases {
		if len(names) > 0 && !slices.Contains(names, c.name) {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			if got := record(c.in); !slices.Equal(got, c.want) {
				t.Errorf("Write(%q)\n got %q\nwant %q", c.in, got, c.want)
			}
		})
	}
}

func TestParserPrintFastPath(t *testing.T) {
	r := &recorder{}
	NewParser(r).Write([]byte("hello"))
	// The recorder merges adjacent PrintASCII calls, so count calls directly.
	if !slices.Equal(r.out, []string{"A(hello)"}) {
		t.Fatalf("got %q", r.out)
	}
	calls := 0
	h := &countASCII{n: &calls}
	NewParser(h).Write([]byte("hello"))
	if calls != 1 {
		t.Fatalf("PrintASCII calls = %d, want 1", calls)
	}
}

type countASCII struct {
	recorder
	n *int
}

func (c *countASCII) PrintASCII(b []byte) { *c.n++ }

func TestParserUTF8(t *testing.T) {
	runCases(t, "utf8", "invalid", "truncated utf8 before control", "c1 bytes are not controls")
}

func TestParserCSI(t *testing.T) {
	runCases(t, "sgr", "private", "subparams", "empty", "default first param", "csi intermediate", "esc restarts csi", "csi ignore")

	var sub []int
	var get int = -1
	r := &recorder{onCSI: func(p *Params) { sub = slices.Clone(p.Sub(0)); get = p.Get(0, 0) }}
	pr := NewParser(r)
	pr.Write([]byte("\x1b[4:3m"))
	if !slices.Equal(sub, []int{4, 3}) {
		t.Errorf("Sub(0) = %v, want [4 3]", sub)
	}
	pr.Write([]byte("\x1b[m"))
	if get != 0 {
		t.Errorf("Get(0,0) = %d, want 0", get)
	}
	r.onCSI = func(p *Params) { get = p.Get(0, 1) }
	pr.Write([]byte("\x1b[0H"))
	if get != 1 {
		t.Errorf("Get(0,1) on explicit 0 = %d, want 1", get)
	}
	r.onCSI = func(p *Params) { get = p.Get(5, 7) }
	pr.Write([]byte("\x1b[2H"))
	if get != 7 {
		t.Errorf("Get(5,7) out of range = %d, want 7", get)
	}
}

func TestParserParamLimits(t *testing.T) {
	var n, last int
	var subs []int
	r := &recorder{onCSI: func(p *Params) { n = p.Len(); last = p.Get(p.Len()-1, -1); subs = slices.Clone(p.Sub(0)) }}
	pr := NewParser(r)
	pr.Write([]byte("\x1b[" + strings.Repeat("1;", 40) + "9m"))
	if n != maxParams || last != 1 {
		t.Errorf("40 params: Len=%d last=%d, want %d and 1", n, last, maxParams)
	}
	pr.Write([]byte("\x1b[1:2:3:4:5:6:7:8:9:10m"))
	if !slices.Equal(subs, []int{1, 2, 3, 4, 5, 6, 7, 8}) {
		t.Errorf("Sub(0) = %v, want first 8", subs)
	}
	// Values clamp at 2^31-1 on every GOARCH, including 32-bit int.
	for _, in := range []string{"2147483647", "2147483648", "99999999999", "99999999999999999999"} {
		pr.Write([]byte("\x1b[" + in + "m"))
		if last != maxParamVal {
			t.Errorf("param %s = %d, want %d", in, last, maxParamVal)
		}
	}
}

func TestParserOSC(t *testing.T) {
	runCases(t, "osc bel", "osc st", "osc utf8", "string esc then csi")
}

func TestParserOSCLimit(t *testing.T) {
	big := "\x1b]0;" + strings.Repeat("x", maxStringLen+1) + "\x07ok"
	if got := record(big); !slices.Equal(got, []string{"A(ok)"}) {
		t.Errorf("oversized OSC: got %q", got)
	}
	big = "\x1bPq" + strings.Repeat("x", maxStringLen+1) + "\x1b\\ok"
	if got := record(big); !slices.Equal(got, []string{"A(ok)"}) {
		t.Errorf("oversized DCS: got %q", got)
	}
}

func TestParserESCAndDCS(t *testing.T) {
	runCases(t, "esc", "esc intermediate", "dcs", "dcs private", "apc ignored", "pm ignored", "del ignored in ground")
}

func TestParserC0InsideCSI(t *testing.T) {
	runCases(t, "c0 inside csi")
}

func TestParserCANAbort(t *testing.T) {
	runCases(t, "can abort", "sub abort")
}

func TestParserSplitAcrossWrites(t *testing.T) {
	for _, c := range parserCases {
		whole := record(c.in)
		for i := 1; i < len(c.in); i++ {
			if got := record(c.in[:i], c.in[i:]); !slices.Equal(got, whole) {
				t.Errorf("%s split at %d:\n got %q\nwant %q", c.name, i, got, whole)
			}
		}
	}
}

type nopHandler struct{}

func (nopHandler) Print(rune)                        {}
func (nopHandler) PrintASCII([]byte)                 {}
func (nopHandler) Execute(byte)                      {}
func (nopHandler) CSI(*Params, []byte, byte, byte)   {}
func (nopHandler) ESC([]byte, byte)                  {}
func (nopHandler) OSC([]byte)                        {}
func (nopHandler) DCS(*Params, []byte, byte, []byte) {}

// BenchmarkParserASCII parses 1 MB of printable ASCII with 2% newlines
// (one every 50 bytes).
func BenchmarkParserASCII(b *testing.B) {
	data := make([]byte, 1<<20)
	for i := range data {
		if i%50 == 49 {
			data[i] = '\n'
		} else {
			data[i] = byte(' ' + i%95)
		}
	}
	p := NewParser(nopHandler{})
	b.SetBytes(int64(len(data)))
	for b.Loop() {
		p.Write(data)
	}
}
