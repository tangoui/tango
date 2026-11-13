package monarch

import "sync"

// State 是驻留后的行尾状态 ID；0 保留为"无"（TokenizeLine 视同 Initial）。
// 同一 Lexer 上，相同的（状态栈, 嵌入语言, 嵌入语言状态）总是得到同一个 State。
// State 只在产生它的 Lexer 上有意义。
//
// 驻留表只增不减：状态名可由输入内容决定（如 `@heredoc.$1` 里的 heredoc 标记），
// 不同的状态名会让表持续增长。若这对长期运行的进程重要，可按文档重建 Registry。
type State uint32

// stackElem 是 Monarch 状态栈的一个元素。元素按（父元素, 状态名）驻留，
// 因此结构相同的栈就是同一个指针，可以直接作 map 键。
type stackElem struct {
	parent *stackElem
	state  string
	depth  int
	rules  []*rule
	ok     bool // 状态（或其点分前缀）有定义
}

// embedded 是嵌入语言的状态（对应 Monaco 的 EmbeddedLanguageData）。
type embedded struct {
	active bool
	lang   string // 解析后的语言 ID（未注册的语言也保留原名）
	lexer  *Lexer // nil：语言没有分词器，内容按 "" 输出
	state  State  // lexer 上的状态
}

// lineState 是行间传递的完整状态（对应 MonarchLineState）。
type lineState struct {
	stack *stackElem
	emb   embedded
}

type stackKey struct {
	parent *stackElem
	state  string
}

type interner struct {
	mu     sync.Mutex
	stacks map[stackKey]*stackElem
	ids    map[lineState]State
	states []lineState // State-1 → lineState
}

func (l *Lexer) stackElem(parent *stackElem, state string) *stackElem {
	in := &l.intern
	in.mu.Lock()
	defer in.mu.Unlock()
	k := stackKey{parent, state}
	if e, ok := in.stacks[k]; ok {
		return e
	}
	e := &stackElem{parent: parent, state: state, depth: 1}
	if parent != nil {
		e.depth = parent.depth + 1
	}
	e.rules, e.ok = l.g.findRules(state)
	in.stacks[k] = e
	return e
}

func (l *Lexer) internState(ls lineState) State {
	in := &l.intern
	in.mu.Lock()
	defer in.mu.Unlock()
	if id, ok := in.ids[ls]; ok {
		return id
	}
	in.states = append(in.states, ls)
	id := State(len(in.states))
	in.ids[ls] = id
	return id
}

func (l *Lexer) lookupState(st State) (lineState, bool) {
	in := &l.intern
	in.mu.Lock()
	defer in.mu.Unlock()
	if st == 0 || int(st) > len(in.states) {
		return lineState{}, false
	}
	return in.states[st-1], true
}

func (e *stackElem) popall() *stackElem {
	for e.parent != nil {
		e = e.parent
	}
	return e
}
