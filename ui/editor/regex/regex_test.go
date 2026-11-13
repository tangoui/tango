package regex

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func mustCompile(t *testing.T, src, flags string) Regexp {
	t.Helper()
	re, err := Compile(src, flags)
	if err != nil {
		t.Fatalf("Compile(%q, %q): %v", src, flags, err)
	}
	return re
}

func TestMatchStart(t *testing.T) {
	re := mustCompile(t, "[a-z]+", "")
	if got := re.MatchStart("abc1"); !reflect.DeepEqual(got, []int{0, 3}) {
		t.Errorf("abc1: got %v", got)
	}
	if got := re.MatchStart("1abc"); got != nil {
		t.Errorf("1abc: got %v, want nil (must match at start)", got)
	}
	if got := mustCompile(t, `\bfoo`, "").MatchStart("foo"); !reflect.DeepEqual(got, []int{0, 3}) {
		t.Errorf(`\bfoo: got %v`, got)
	}
	if re.Source() != "[a-z]+" || re.NumGroups() != 0 {
		t.Errorf("Source/NumGroups = %q/%d", re.Source(), re.NumGroups())
	}
}

func TestEngines(t *testing.T) {
	if e := mustCompile(t, `\d+`, "").Engine(); e != Std {
		t.Errorf(`\d+ engine = %v, want Std`, e)
	}
	re := mustCompile(t, "-(?!>)", "")
	if re.Engine() != Backtrack {
		t.Fatalf("-(?!>) engine = %v, want Backtrack", re.Engine())
	}
	if got := re.MatchStart("->"); got != nil {
		t.Errorf("->: got %v, want nil", got)
	}
	if got := re.MatchStart("-a"); !reflect.DeepEqual(got, []int{0, 1}) {
		t.Errorf("-a: got %v, want [0 1]", got)
	}
}

// 两种引擎都返回字节偏移，未参与的组为 -1，组号按 JS 出现顺序。
func TestGroupsBothEngines(t *testing.T) {
	cases := []struct {
		src, in string
		want    []int
		engine  Engine
	}{
		{`(é)(x)?(?<n>中)`, "é中!", []int{0, 5, 0, 2, -1, -1, 2, 5}, Std},
		{`(é)(x)?(?<n>中)(?=!)`, "é中!", []int{0, 5, 0, 2, -1, -1, 2, 5}, Backtrack},
		{`(?<a>é)(b)\2`, "ébb", []int{0, 4, 0, 2, 2, 3}, Backtrack},
	}
	for _, c := range cases {
		re := mustCompile(t, c.src, "")
		if re.Engine() != c.engine {
			t.Errorf("%q engine = %v, want %v", c.src, re.Engine(), c.engine)
		}
		if re.NumGroups() != len(c.want)/2-1 {
			t.Errorf("%q NumGroups = %d", c.src, re.NumGroups())
		}
		if got := re.MatchStart(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q on %q = %v, want %v", c.src, c.in, got, c.want)
		}
	}
}

func TestUnicodeSpace(t *testing.T) {
	for _, src := range []string{`\s`, `\s(?=x)`, `[\s]`, `[\s](?=x)`} {
		re := mustCompile(t, src, "")
		if got := re.MatchStart("\u3000x"); !reflect.DeepEqual(got, []int{0, 3}) {
			t.Errorf("%q (%v) on ideographic space: got %v", src, re.Engine(), got)
		}
	}
	// U+0085 不在 JS 空白集合里（Go 的 \s 也不认它，但 Unicode 意义上的空白认）
	for _, src := range []string{`\s`, `\s(?=x)`} {
		if got := mustCompile(t, src, "").MatchStart("\u0085x"); got != nil {
			t.Errorf("%q matched U+0085: %v", src, got)
		}
	}
}

// . 在两种引擎上都按 JS 排除行终止符，$ 只匹配输入末尾。
func TestDotAndDollarBothEngines(t *testing.T) {
	for _, la := range []string{"", "(?=)"} {
		dot := mustCompile(t, "."+la, "")
		if got := dot.MatchStart("\u2028"); got != nil {
			t.Errorf("dot%s matched U+2028", la)
		}
		if got := mustCompile(t, "."+la, "s").MatchStart("\u2028"); !reflect.DeepEqual(got, []int{0, 3}) {
			t.Errorf("dot%s with s: got %v", la, got)
		}
		if got := mustCompile(t, "a$"+la, "").MatchStart("a\n"); got != nil {
			t.Errorf("a$%s matched before trailing newline: %v", la, got)
		}
		if got := mustCompile(t, "abc"+la, "i").MatchStart("ABC"); !reflect.DeepEqual(got, []int{0, 3}) {
			t.Errorf("abc%s with i: got %v", la, got)
		}
	}
}

func TestCompileErrors(t *testing.T) {
	for _, c := range [][2]string{{"a", "d"}, {"a", "v"}, {"a", "gg"}, {"[a", ""}, {"(a", ""}, {"(?=a", ""}} {
		if _, err := Compile(c[0], c[1]); err == nil {
			t.Errorf("Compile(%q, %q) succeeded, want error", c[0], c[1])
		}
	}
}

func TestBacktrackTimeout(t *testing.T) {
	re := mustCompile(t, "(a+)+(?=b)", "")
	if re.Engine() != Backtrack {
		t.Fatalf("engine = %v, want Backtrack", re.Engine())
	}
	_, _, before := Stats()
	start := time.Now()
	got := re.MatchStart(strings.Repeat("a", 50) + "c")
	elapsed := time.Since(start)
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
	// MatchTimeout 加上 regexp2 时钟的检查粒度（5ms）与余量
	assertBudget(t, "timed-out MatchStart", elapsed, MatchTimeout+100*time.Millisecond)
	if _, _, after := Stats(); after <= before {
		t.Errorf("timeouts %d → %d, want increase", before, after)
	}
}

func TestStatsCountsEngines(t *testing.T) {
	s0, b0, _ := Stats()
	mustCompile(t, "x", "")
	mustCompile(t, "x(?=y)", "")
	s1, b1, _ := Stats()
	if s1 != s0+1 || b1 != b0+1 {
		t.Errorf("Stats delta std=%d backtrack=%d, want 1/1", s1-s0, b1-b0)
	}
}

// g、y 对"从开头匹配一次"的 MatchStart 没有意义，接受并忽略。
func TestGlobalStickyFlagsIgnored(t *testing.T) {
	for _, flags := range []string{"g", "gy", "y"} {
		re := mustCompile(t, `\w+`, flags)
		if got := re.MatchStart("ab c"); !reflect.DeepEqual(got, []int{0, 2}) {
			t.Errorf("flags %q: got %v, want [0 2]", flags, got)
		}
	}
}
