package regex

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// MatchStartRunes 与 MatchStart 结果完全一致（两种引擎、非 ASCII、非法 UTF-8、前瞻里越界的组）。
func TestMatchStartRunesEquivalent(t *testing.T) {
	pats := []struct{ src, flags string }{
		{`[a-z]+`, ""},
		{`(\w+)(?=\s*=)`, ""}, // Backtrack
		{`中(.)(?=(.+))`, "u"}, // Backtrack，组越过整体匹配结尾
		{`(a|b)\1`, ""},       // 反向引用
		{`.`, ""},
	}
	inputs := []string{"", "abc = 1", "中文😀字x", "aa", "ab", "\xff\xfeabc", "x = 中"}
	for _, p := range pats {
		re := mustCompile(t, p.src, p.flags)
		for _, in := range inputs {
			for i := 0; i <= len(in); i++ {
				s := in[i:]
				want := re.MatchStart(s)
				if got := re.MatchStartRunes(s, []rune(s)); !reflect.DeepEqual(got, want) {
					t.Errorf("%q on %q: MatchStartRunes=%v MatchStart=%v", p.src, s, got, want)
				}
			}
		}
	}
}

// 在长行上逐位置匹配时，复用 []rune 的子切片避免每次 O(行长) 的转换。
func TestMatchStartRunesLongLine(t *testing.T) {
	re := mustCompile(t, `x(?=y)`, "")
	if re.Engine() != Backtrack {
		t.Fatal("want Backtrack")
	}
	s := strings.Repeat("中", 20000)
	rs := []rune(s)
	start := time.Now()
	for i := range rs {
		if re.MatchStartRunes(s[3*i:], rs[i:]) != nil {
			t.Fatal("unexpected match")
		}
	}
	assertBudget(t, "20000 positions", time.Since(start), time.Second)
}
