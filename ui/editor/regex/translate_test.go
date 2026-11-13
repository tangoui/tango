package regex

import "testing"

// JS 空白集合（\s）在 RE2 字符类内部的展开，及其补集（\S）。
const (
	wsSet  = `\t\n\v\f\r \x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}`
	nwsSet = `\x{0}-\x{8}\x{e}-\x{1f}\x{21}-\x{9f}\x{a1}-\x{167f}\x{1681}-\x{1fff}\x{200b}-\x{2027}\x{202a}-\x{202e}\x{2030}-\x{205e}\x{2060}-\x{2fff}\x{3001}-\x{fefe}\x{ff00}-\x{10ffff}`
)

func TestTranslate(t *testing.T) {
	tests := []struct {
		src, flags string
		want       string
		ok         bool
	}{
		// 与 RE2 相同的语法原样保留
		{`\d+`, "", `\d+`, true},
		{`[a-z\d_]+`, "", `[a-z\d_]+`, true},
		{`a{2,3}b{2}c{1,}?`, "", `a{2,3}b{2}c{1,}?`, true},
		{`(?:ab|cd)*`, "", `(?:ab|cd)*`, true},
		{`\bfoo\B`, "", `\bfoo\B`, true},
		{`\n\r\t\v\f`, "", `\n\r\t\v\f`, true},
		{`^a$`, "", `^a$`, true},
		// \s / \S → JS 空白集合
		{`\s+`, "", `[` + wsSet + `]+`, true},
		{`\S`, "", `[^` + wsSet + `]`, true},
		{`[\s,]`, "", `[` + wsSet + `,]`, true},
		{`[^\S]`, "", `[^` + nwsSet + `]`, true},
		{`[^\s]`, "", `[^` + wsSet + `]`, true},
		// [^] 与空类
		{`[^]`, "", `[` + wsSet + nwsSet + `]`, true},
		{`a[]`, "", `a[^\x{0}-\x{10ffff}]`, true},
		// \uXXXX、\u{…}、\xHH
		{"\\u00e9", "", `\x{e9}`, true},
		{"[\\u4e00-\\u9fa5]", "", `[\x{4e00}-\x{9fa5}]`, true},
		{`\u{1F600}`, "u", `\x{1f600}`, true},
		{`\u{1F600}`, "", `u\{1F600\}`, true}, // 无 u 标志时 \u 是字母 u
		{"\\uD83D\\uDE00", "", `\x{1f600}`, true},
		{`\x41\xZ`, "", `\x{41}xZ`, true},
		// 命名组
		{`(?<name>a)`, "", `(?<name>a)`, true},
		// 标点转义保留、字母恒等转义 → 字母本身
		{`\/\*\.`, "", `\/\*\.`, true},
		{`\a\e\-`, "", `ae\-`, true},
		{`]}`, "", `\]\}`, true},
		{`a{`, "", `a\{`, true},
		{`x{,2}`, "", `x\{,2\}`, true},
		// \cX
		{`\cJ`, "", `\x{a}`, true},
		{`\cj`, "", `\x{a}`, true},
		{`\c1`, "", `\\c1`, true},
		{`[\c_]`, "", `[\x{1f}]`, true},
		// 八进制与 \0
		{`\0`, "", `\x{0}`, true},
		{`\101`, "", `\x{41}`, true},
		{`\12`, "", `\x{a}`, true}, // 没有捕获组 → 旧式八进制
		{`\8`, "", `8`, true},
		{`[\101\8]`, "", `[\x{41}8]`, true},
		// [\b] 是退格
		{`[\b]`, "", `[\x{8}]`, true},
		// 字符类中 - 与集合相邻时是字面量
		{`[\w-]`, "", `[\w\-]`, true},
		{`[\w-z]`, "", `[\w\-z]`, true},
		{`[a-\d]`, "", `[a\-\d]`, true},
		{`[--/]`, "", `[\--/]`, true},
		{`[[]`, "", `[\[]`, true},
		{`[a^]`, "", `[a\^]`, true},
		// . 的 JS 语义
		{`.`, "", `[^\n\r\x{2028}\x{2029}]`, true},
		{`.`, "s", `(?s:.)`, true},
		// 标志
		{`abc`, "i", `(?i)abc`, true},
		{`\p{L}`, "u", `\p{L}`, true},
		{`\p{Script=Greek}`, "u", `\p{Greek}`, true},
		{`\p{L}`, "", `p\{L\}`, true},
		{`\k`, "", `k`, true},
		{`abc`, "m", `abc`, true},
		// 语义无法等价 → ok=false
		{`a(?=b)`, "", "", false},
		{`-(?!>)`, "", "", false},
		{`(?<!x)y`, "", "", false},
		{`(?<=x)y`, "", "", false},
		{`(a)\1`, "", "", false},
		{`(?<n>a)\k<n>`, "", "", false},
		{`^a`, "m", "", false},     // JS 多行 ^ 还认 \r、U+2028/2029
		{`a{1001}`, "", "", false}, // 超出 RE2 重复上限
		{`abc`, "d", "", false},    // 不支持的标志
		{`abc`, "ii", "", false},
		{`[abc`, "", "", false},
		{`[z-a]`, "", "", false},
	}
	for _, tc := range tests {
		got, ok := Translate(tc.src, tc.flags)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("Translate(%q, %q) = %q, %v; want %q, %v", tc.src, tc.flags, got, ok, tc.want, tc.ok)
		}
	}
}
