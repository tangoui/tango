// Package regex 编译 Monaco Monarch 语法中的 JavaScript 正则。
//
// 能与 JS 语义等价翻译到 RE2 的正则用 Go 标准库 regexp（线性时间）；其余
// （前瞻、后顾、反向引用等）退回 github.com/dlclark/regexp2 的 ECMAScript 模式，
// 并给每次匹配加超时（MatchTimeout）。MatchStart/MatchStartRunes 把超时当作不匹配；
// TryMatchStart 返回 ErrTimeout，供调用方区分"不匹配"与"放弃匹配"（monarch 据此降级整行）。
//
// 已知局限（与浏览器 JS 的差异）：
//   - i 标志且无 u 时，JS 的大小写规范化不让 ſ(U+017F) 匹配 s、K(U+212A) 匹配 k；
//     Go (?i) 的简单折叠会匹配。i+u 下 JS 的 \w/\b 含 ſ 与 K，RE2 不含。
//   - 无 u 标志时 JS 的 . 与 [^x] 每次匹配一个 UTF-16 代码单元；本包两种引擎都按完整
//     码点（rune）匹配，增补平面字符（如 emoji）整体被一个 . 吃掉。孤立代理永不匹配。
//   - 不支持 \p{XID_Start} 等 RE2 与 regexp2 都没有的 Unicode 属性。
//   - 回溯引擎超时依赖进程级 regexp2.SetTimeoutCheckPeriod（见 init）。
//
// Experimental: this package's API may change in any 0.x release (see
// docs/release.md, "API 稳定性").
package regex

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/dlclark/regexp2"
)

// Engine 标识一个 Regexp 实际使用的引擎。
type Engine int

const (
	Std       Engine = iota // Go regexp（RE2）
	Backtrack               // regexp2，ECMAScript 模式，带超时
)

func (e Engine) String() string {
	switch e {
	case Std:
		return "Std"
	case Backtrack:
		return "Backtrack"
	}
	return fmt.Sprintf("Engine(%d)", int(e))
}

// Regexp 是编译后的 JS 正则。
type Regexp interface {
	// MatchStart 只接受从 s 开头开始的匹配（Monarch 对"行的剩余部分"做 ^(?:…) 匹配）。
	// 返回字节偏移对，长度 2*(1+NumGroups())，未参与的组为 -1；不匹配返回 nil。
	MatchStart(s string) []int
	// MatchStartRunes 同 MatchStart，调用方另给出 rs（必须等于 []rune(s)，通常是整行 []rune 的子切片）。
	// 回溯引擎借此免去每次把 s 转成 []rune 的 O(len(s)) 开销——在长行上逐位置匹配时那是平方级的；
	// Std 引擎忽略 rs。
	MatchStartRunes(s string, rs []rune) []int
	// TryMatchStart 同 MatchStartRunes（rs 可为 nil，此时按需转换），但回溯引擎超时时返回
	// (nil, ErrTimeout) 而不是当作不匹配。Std 引擎（线性时间）从不返回错误。
	TryMatchStart(s string, rs []rune) ([]int, error)
	Engine() Engine
	Source() string
	NumGroups() int
}

// MatchTimeout 是回溯引擎单次匹配的时限。正常输入上的匹配远低于此值，只有病态回溯会触及；
// monarch 在一行内遇到第一次超时就降级整行（见 monarch.LineInfo），因此单行最坏多花约这么久。
// 每次引擎匹配保留完整 MatchTimeout；第一次超时后从头确认一次，两次都超时才报告 ErrTimeout。
// 因而一次调度/GC 停顿不会被误报为病态回溯。确认会使真实病态匹配花约 2*MatchTimeout，
// 另加后台时钟检查粒度；这不是严格的墙钟上限（调度暂停仍会延长总耗时）。
// 后台时钟滞后导致的提前超时不计入确认次数（见 TryMatchStart）。
// -race 构建中时限放大 10 倍（race.go）。
const MatchTimeout = 100 * time.Millisecond

// matchTimeout 是实际使用的时限：MatchTimeout 乘以 raceFactor（见 race.go）。
const matchTimeout = MatchTimeout * raceFactor

// ErrTimeout 是 TryMatchStart 在回溯引擎超过 MatchTimeout 时返回的错误。
var ErrTimeout = errors.New("regex: match timeout")

var nStd, nBacktrack, nTimeouts atomic.Int64

func init() {
	// regexp2 的超时由后台时钟检查，默认周期 100ms，实际超时会是 MatchTimeout+100~200ms。
	// 调到 5ms 使实际超时接近 MatchTimeout；该时钟只在有超时匹配进行时运行（最后一次截止后约 1s 停止）。
	// 注意：这是全局副作用，会影响同一进程中所有使用 regexp2 超时的代码（只影响检查粒度，不改变语义）。
	// 这是 regexp2 的进程级设置，只在 init 里写一次，避免与时钟 goroutine 竞争。
	regexp2.SetTimeoutCheckPeriod(5 * time.Millisecond)
}

// Stats 返回全局计数：编译成功的 Std / Backtrack 正则数，以及回溯匹配超时次数。
func Stats() (stdCount, backtrackCount, timeouts int64) {
	return nStd.Load(), nBacktrack.Load(), nTimeouts.Load()
}

// Compile 编译 JS 正则源码。flags 支持 JS 的 i、s、u、m；g、y 被接受并忽略
// （MatchStart 总是返回锚定在开头的单次匹配，二者都是空操作）；其他标志（d、v 等）或重复标志报错。
func Compile(source, flags string) (Regexp, error) {
	fl, err := parseFlags(flags)
	if err != nil {
		return nil, err
	}
	out, groups, err := translate(source, fl, toRE2)
	switch {
	case err == nil:
		if re, cerr := regexp.Compile(anchorStd(out)); cerr == nil {
			nStd.Add(1)
			return &stdRegexp{re: re, src: source, ngroups: len(groups)}, nil
		}
		// RE2 拒绝（如重复次数超过 1000、组名含 $）：交给 regexp2。
	case err != errNeedBacktrack:
		return nil, err
	}

	out, groups, err = translate(source, fl, toBacktrack)
	if err != nil {
		return nil, err
	}
	opts := regexp2.RegexOptions(regexp2.ECMAScript)
	if fl.i {
		opts |= regexp2.IgnoreCase
	}
	re, err := regexp2.Compile(anchorBacktrack(out), opts)
	if err != nil {
		return nil, fmt.Errorf("regex: compile %q: %w", source, err)
	}
	re.MatchTimeout = matchTimeout
	// regexp2 先给未命名组编号、再给命名组编号；换算成 JS 的出现顺序。
	nums := make([]int, len(groups))
	unnamed := 0
	for i, name := range groups {
		if name == "" {
			unnamed++
			nums[i] = unnamed
		} else {
			nums[i] = re.GroupNumberFromName(name)
		}
	}
	nBacktrack.Add(1)
	return &backtrackRegexp{re: re, src: source, nums: nums}, nil
}

type stdRegexp struct {
	re      *regexp.Regexp
	src     string
	ngroups int
}

func (r *stdRegexp) MatchStart(s string) []int { return r.re.FindStringSubmatchIndex(s) }
func (r *stdRegexp) MatchStartRunes(s string, _ []rune) []int {
	return r.re.FindStringSubmatchIndex(s)
}
func (r *stdRegexp) TryMatchStart(s string, _ []rune) ([]int, error) {
	return r.re.FindStringSubmatchIndex(s), nil
}
func (r *stdRegexp) Engine() Engine { return Std }
func (r *stdRegexp) Source() string { return r.src }
func (r *stdRegexp) NumGroups() int { return r.ngroups }

type backtrackRegexp struct {
	re   *regexp2.Regexp
	src  string
	nums []int // JS 组 i+1 对应的 regexp2 组号
}

func (r *backtrackRegexp) Engine() Engine { return Backtrack }
func (r *backtrackRegexp) Source() string { return r.src }
func (r *backtrackRegexp) NumGroups() int { return len(r.nums) }

func (r *backtrackRegexp) MatchStart(s string) []int {
	return r.MatchStartRunes(s, []rune(s))
}

func (r *backtrackRegexp) MatchStartRunes(s string, rs []rune) []int {
	m, _ := r.TryMatchStart(s, rs)
	return m
}

func (r *backtrackRegexp) TryMatchStart(s string, rs []rune) ([]int, error) {
	return r.tryMatchStart(s, rs, r.re.FindRunesMatch, time.Now)
}

// Dependencies are call-local: deterministic pause tests never mutate a shared
// Regexp, the regexp2 process clock, or a global hook used by other goroutines.
func (r *backtrackRegexp) tryMatchStart(s string, rs []rune, find func([]rune) (*regexp2.Match, error), now func() time.Time) ([]int, error) {
	if rs == nil {
		rs = []rune(s)
	}
	start := now()
	confirmed := false
	var m *regexp2.Match
	for {
		var err error
		m, err = find(rs)
		if err == nil {
			break
		}
		// FindRunesMatch 只会因超时出错；仍按消息区分，以免把其他错误记成超时。
		if !strings.Contains(err.Error(), "timeout") {
			return nil, fmt.Errorf("regex: match %q: %w", r.src, err)
		}
		// A stale regexp2 clock can produce an early timeout. Give each phase
		// its own elapsed-time allowance, then confirm on a fresh deadline.
		// A scheduler pause in the first phase does not consume the second.
		if now().Sub(start) >= matchTimeout {
			if confirmed {
				nTimeouts.Add(1)
				return nil, ErrTimeout
			}
			confirmed = true
			start = now()
		}
	}
	if m == nil {
		return nil, nil
	}
	// regexp2 给出 rune 下标（与 []rune(s) 一致，非法字节各算一个 rune），换算为字节偏移。
	res := make([]int, 2*(1+len(r.nums)))
	res[0], res[1] = m.Index, m.Index+m.Length
	maxRune := res[1]
	for i, n := range r.nums {
		g := m.GroupByNumber(n)
		if g == nil || len(g.Captures) == 0 {
			res[2*i+2], res[2*i+3] = -1, -1
			continue
		}
		res[2*i+2], res[2*i+3] = g.Index, g.Index+g.Length
		maxRune = max(maxRune, res[2*i+3]) // 前瞻里的组可以越过整体匹配的结尾
	}
	byteOff := make([]int, 0, maxRune+1)
	for b := 0; len(byteOff) <= maxRune; {
		byteOff = append(byteOff, b)
		_, size := utf8.DecodeRuneInString(s[b:])
		if size == 0 {
			break
		}
		b += size
	}
	for i, v := range res {
		if v >= 0 {
			res[i] = byteOff[v]
		}
	}
	return res, nil
}
