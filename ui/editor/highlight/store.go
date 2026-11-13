package highlight

import (
	"bytes"
	"slices"
	"sort"
	"time"

	"github.com/tangoui/tango/ui/editor/monarch"
)

// store 是每行的高亮状态。
//
//   - states[i] 是第 i 行的行尾状态（下一行的起始状态）；0 表示未知（TokenizeLine 视同 Initial）。
//     每行总有一个，1M 行约 4 MB。
//   - dirty[i] == 1 表示第 i 行须重新分词（文本变了，或起始状态可能变了）。不变式：干净的行满足
//     states[i] == tokenize(text[i], states[i-1]) 的行尾状态；脏行的 states[i] 只是"旧值"，
//     重新分词时用来比较、决定能否在此停止。
//   - lo 是首个脏行的下界：lo 之前的行都干净。
//   - wins 是 token 窗口，每个视图（ViewKey）一个，最多 maxWindows 个，满了按最近最少使用淘汰。
//     窗口 w 覆盖 [w.lo, w.lo+len(w.slots)) 行；所有窗口外只留状态，token 需要时由起始状态重算。
//     窗口可以重叠：同一行在几个窗口里各有一个槽，apply 同时写入（复制）所有槽，保持一致。
//   - deg 是降级（Degraded）的行，按行号排序：等待重试的行及其以下的行不算最终；accepted 的记录
//     （重试已用完、结果已接受）不再等待，只用来让 Tokens 重算再次降级时不重新开始重试。
//     编辑落在该行（splice 删除记录）或该行重新分词不再降级时记录被移除。
type store struct {
	states []monarch.State
	dirty  []byte
	lo     int

	wins  []window
	clock uint64 // LRU 计数

	deg []degraded

	// 多区域编辑时的双缓冲，避免每次分配。
	spareStates []monarch.State
	spareDirty  []byte
}

// slot 是窗口中一行的 token。ok 时 toks 是本行"当前文本"从 start 状态分词的结果：
// 行干净时它就是最终 token；行脏时是临时结果（起始状态是猜的）。
type slot struct {
	toks  []monarch.Token
	start monarch.State
	ok    bool
}

// maxWindows 是 token 窗口（视图）的上限。
const maxWindows = 4

// window 是一个视图的 token 窗口：覆盖 [lo, lo+len(slots)) 行，围绕 focus（该视图最近请求的行）。
type window struct {
	key   ViewKey
	lo    int
	slots []slot
	focus int
	used  uint64 // LRU：最近使用时的 store.clock
}

func (w *window) contains(i int) bool { return i >= w.lo && i < w.lo+len(w.slots) }

// degraded 记录一行降级的次数与下次重试的时间。
type degraded struct {
	line     int
	attempts int
	due      time.Time
	accepted bool // 重试已用完，结果已接受：不再等待重试
}

// region 是一次 Edit 中受影响的一段行：旧行 [sl, el] 被替换为从 nsl 起的 el-sl+1+delta 行。
type region struct {
	sl, el, delta, nsl int
}

func (r region) m() int { return r.el - r.sl + 1 + r.delta }

func (s *store) n() int { return len(s.states) }

// reset 把行数设为 n，所有行标为脏、状态清零（状态可能来自别的文本或别的 Lexer），丢弃 token 与重试记录。
func (s *store) reset(n int) {
	s.states = slices.Grow(s.states[:0], n)[:n]
	clear(s.states)
	s.dirty = slices.Grow(s.dirty[:0], n)[:n]
	for i := range s.dirty {
		s.dirty[i] = 1
	}
	s.lo = 0
	for k := range s.wins {
		clear(s.wins[k].slots)
	}
	s.clampWindow()
	s.deg = s.deg[:0]
}

// nextDirty 返回首个脏行，没有则返回 n()。
func (s *store) nextDirty() int {
	if s.lo >= len(s.dirty) {
		s.lo = len(s.dirty)
		return s.lo
	}
	k := bytes.IndexByte(s.dirty[s.lo:], 1)
	if k < 0 {
		s.lo = len(s.dirty)
	} else {
		s.lo += k
	}
	return s.lo
}

func (s *store) markDirty(i int) {
	s.dirty[i] = 1
	if i < s.lo {
		s.lo = i
	}
}

// minPending 返回首个等待重试的降级行，没有则返回 n()。
func (s *store) minPending() int {
	for _, d := range s.deg {
		if !d.accepted {
			return d.line
		}
	}
	return s.n()
}

// acceptedDegraded 报告第 i 行的降级结果是否已在重试用完后被接受。
func (s *store) acceptedDegraded(i int) bool {
	k := sort.Search(len(s.deg), func(k int) bool { return s.deg[k].line >= i })
	return k < len(s.deg) && s.deg[k].line == i && s.deg[k].accepted
}

// final 报告第 i 行的 token 是否是最终结果：它及其之上的行都干净，且没有等待重试的降级行。
func (s *store) final(i int) bool {
	return i < s.nextDirty() && i < s.minPending()
}

// slot 返回第 i 行的窗口槽（第一个覆盖它的窗口），不在任何窗口内返回 nil。
func (s *store) slot(i int) *slot {
	for k := range s.wins {
		if w := &s.wins[k]; w.contains(i) {
			return &w.slots[i-w.lo]
		}
	}
	return nil
}

// slotFor 返回 key 的窗口中第 i 行的槽；不在其中时退回别的窗口（slot）。
func (s *store) slotFor(key ViewKey, i int) *slot {
	if w := s.window(key); w != nil && w.contains(i) {
		return &w.slots[i-w.lo]
	}
	return s.slot(i)
}

// window 返回 key 的窗口，没有则返回 nil。
func (s *store) window(key ViewKey) *window {
	for k := range s.wins {
		if s.wins[k].key == key {
			return &s.wins[k]
		}
	}
	return nil
}

// ensureWindow 返回 key 的窗口（标为最近使用），没有则新建；窗口已满时淘汰最近最少使用的一个。
func (s *store) ensureWindow(key ViewKey) *window {
	w := s.window(key)
	if w == nil {
		if len(s.wins) < maxWindows {
			s.wins = append(s.wins, window{key: key})
			w = &s.wins[len(s.wins)-1]
		} else {
			w = &s.wins[0]
			for k := range s.wins {
				if s.wins[k].used < w.used {
					w = &s.wins[k]
				}
			}
			*w = window{key: key}
		}
	}
	s.touch(w)
	return w
}

// dropWindow 删除 key 的窗口（视图已关闭）；没有则什么也不做。
func (s *store) dropWindow(key ViewKey) {
	for k := range s.wins {
		if s.wins[k].key == key {
			s.wins = slices.Delete(s.wins, k, k+1)
			return
		}
	}
}

// touch marks w as the most recently used window.
func (s *store) touch(w *window) {
	s.clock++
	w.used = s.clock
}

// setWindow 把窗口 w 移到 [lo, hi)（裁剪到 [0, n)），保留重叠部分的 token。
func (w *window) setWindow(lo, hi, n int) {
	lo, hi = max(lo, 0), min(hi, n)
	if hi < lo {
		hi = lo
	}
	if lo == w.lo && hi-lo == len(w.slots) {
		return
	}
	nw := make([]slot, hi-lo)
	for i := max(lo, w.lo); i < min(hi, w.lo+len(w.slots)); i++ {
		nw[i-lo] = w.slots[i-w.lo]
	}
	w.lo, w.slots = lo, nw
}

func (s *store) clampWindow() {
	n := s.n()
	for k := range s.wins {
		w := &s.wins[k]
		if w.lo+len(w.slots) > n {
			w.setWindow(w.lo, n, n)
		}
		w.focus = max(0, min(w.focus, n-1))
	}
}

// focus 让 key 的窗口覆盖 f 附近 ±keep 行。窗口已覆盖 f 附近 ±keep/2 时不动（避免滚动时每帧搬移）。
func (s *store) focus(key ViewKey, f, keep int) {
	w := s.ensureWindow(key)
	w.focus = f
	n := s.n()
	if max(f-keep/2, 0) >= w.lo && min(f+keep/2+1, n) <= w.lo+len(w.slots) {
		return
	}
	w.setWindow(f-keep, f+keep+1, n)
}

// ranges 把各窗口的 [lo, hi) 追加到 dst。
func (s *store) ranges(dst [][2]int) [][2]int {
	for k := range s.wins {
		w := &s.wins[k]
		dst = append(dst, [2]int{w.lo, w.lo + len(w.slots)})
	}
	return dst
}

// noteResult 更新第 i 行的降级重试记录：不降级则移除；降级且未用完重试次数则（重新）排队；
// 用完则标为已接受（保留记录，不再等待重试），以便窗口移动后 Tokens 重算再次降级时不重新开始重试。
func (s *store) noteResult(i int, isDegraded bool, now time.Time, maxRetries int, delay time.Duration) {
	k := sort.Search(len(s.deg), func(k int) bool { return s.deg[k].line >= i })
	found := k < len(s.deg) && s.deg[k].line == i
	if !isDegraded {
		if found {
			s.deg = slices.Delete(s.deg, k, k+1)
		}
		return
	}
	attempts := 1
	if found {
		attempts = s.deg[k].attempts + 1
	}
	d := degraded{line: i, attempts: attempts, due: now.Add(delay)}
	if attempts > maxRetries { // 重试用完：接受降级结果
		d.accepted = true
	}
	if found {
		s.deg[k] = d
	} else {
		s.deg = slices.Insert(s.deg, k, d)
	}
}

// promoteDue 把 limit 之前到期的降级行标为脏（重试），返回其余等待中最早的到期时间（没有则零值）。
func (s *store) promoteDue(limit int, now time.Time) (next time.Time) {
	for _, d := range s.deg {
		switch {
		case d.accepted:
		case d.line < limit && !d.due.After(now):
			s.markDirty(d.line)
		case next.IsZero() || d.due.Before(next):
			next = d.due
		}
	}
	return next
}

// splice 按 regs（按行号升序、互不重叠）替换行：每段旧行 [sl, el] 变为 m() 行，全部标脏；
// 段内前 m()-1 行状态未知，最后一行保留旧第 el 行的行尾状态——它是段后第一行的起始状态，
// 重新分词到这里时若结果相同即可停止。段外的行干净与否、状态与 token 都不变，只平移行号。
func (s *store) splice(regs []region, keep int) {
	total, shifts := 0, false
	for _, r := range regs {
		total += r.delta
		shifts = shifts || r.delta != 0
	}
	oldN := s.n()

	// 窗口：各自按自己的焦点处理，见 spliceWindow。
	for k := range s.wins {
		s.wins[k].splice(regs, keep, shifts, oldN+total)
	}

	// 降级重试记录：段内的删除（行已标脏），段外的平移。
	{
		out := s.deg[:0]
		for _, d := range s.deg {
			if inRegion(regs, d.line) {
				continue
			}
			d.line = mapLine(regs, d.line)
			out = append(out, d)
		}
		s.deg = out
	}

	if !shifts {
		// 每段行数都不变：原地改写各段。
		for _, r := range regs {
			last := s.states[r.el]
			for i := r.sl; i <= r.el; i++ {
				s.states[i] = 0
				s.dirty[i] = 1
			}
			s.states[r.el] = last
		}
	} else if len(regs) == 1 {
		// 单段：原地 Replace（只搬移段后的部分，末尾编辑几乎不搬移）。
		r := regs[0]
		m := r.m()
		last := s.states[r.el]
		repS := make([]monarch.State, m)
		repS[m-1] = last
		repD := bytes.Repeat([]byte{1}, m)
		s.states = slices.Replace(s.states, r.sl, r.el+1, repS...)
		s.dirty = slices.Replace(s.dirty, r.sl, r.el+1, repD...)
	} else {
		// 多段：一遍拷贝到备用缓冲再交换。
		ns := s.spareStates[:0]
		nd := s.spareDirty[:0]
		prev := 0
		for _, r := range regs {
			ns = append(ns, s.states[prev:r.sl]...)
			nd = append(nd, s.dirty[prev:r.sl]...)
			for range r.m() - 1 {
				ns = append(ns, 0)
				nd = append(nd, 1)
			}
			ns = append(ns, s.states[r.el])
			nd = append(nd, 1)
			prev = r.el + 1
		}
		ns = append(ns, s.states[prev:]...)
		nd = append(nd, s.dirty[prev:]...)
		s.spareStates, s.states = s.states, ns
		s.spareDirty, s.dirty = s.dirty, nd
	}
	s.lo = min(s.lo, regs[0].sl, s.n()) // 第一段之前的行不受影响
}

// splice 平移窗口 w：段外的槽平移，段内的槽丢弃（文本变了）；新窗口限制在新焦点 ±keep 内。
// 所有段都在窗口之后（常见：在远处输入）时窗口不变。
// 行数都不变时（窗口内打字）行号不平移：原地作废段内的槽（保留 token 缓冲），不重建窗口。
// newN 是编辑后的行数。
func (w *window) splice(regs []region, keep int, shifts bool, newN int) {
	focusLine := w.focus
	newFocus := mapLine(regs, focusLine)
	w.focus = newFocus
	if !shifts {
		for _, r := range regs {
			for i := max(r.sl, w.lo); i <= min(r.el, w.lo+len(w.slots)-1); i++ {
				sl := &w.slots[i-w.lo]
				*sl = slot{toks: sl.toks[:0]}
			}
		}
		return
	}
	if !(regs[0].sl < w.lo+len(w.slots) || regs[0].sl <= focusLine) {
		return
	}
	nlo := max(mapLine(regs, w.lo), newFocus-keep)
	nhi := min(mapLine(regs, w.lo+len(w.slots)-1)+1, newFocus+keep+1, newN)
	if len(w.slots) == 0 || nhi < nlo {
		nlo, nhi = 0, 0
	}
	nw := make([]slot, nhi-nlo)
	ri := 0
	for j := w.lo; j < w.lo+len(w.slots); j++ {
		for ri < len(regs) && regs[ri].el < j {
			ri++
		}
		if ri < len(regs) && regs[ri].sl <= j {
			continue // 段内
		}
		shift := 0
		if ri > 0 {
			p := regs[ri-1]
			shift = p.nsl + p.m() - (p.el + 1)
		}
		if nj := j + shift; nj >= nlo && nj < nhi {
			nw[nj-nlo] = w.slots[j-w.lo]
		}
	}
	w.lo, w.slots = nlo, nw
}

// inRegion 报告旧行 j 是否在某段内。
func inRegion(regs []region, j int) bool {
	k := sort.Search(len(regs), func(k int) bool { return regs[k].el >= j })
	return k < len(regs) && regs[k].sl <= j
}

// mapLine 把旧行号映射到新行号；段内的行映射到该段的第一行。
func mapLine(regs []region, j int) int {
	k := sort.Search(len(regs), func(k int) bool { return regs[k].el >= j })
	if k < len(regs) && regs[k].sl <= j {
		return regs[k].nsl
	}
	if k == 0 {
		return j
	}
	p := regs[k-1]
	return j + p.nsl + p.m() - (p.el + 1)
}
