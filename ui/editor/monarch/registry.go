package monarch

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/tangoui/tango/ui/editor/langs"
)

// Registry 管理语言 ID 到 Monarch 语法的映射，按需编译 Lexer。可并发使用。
type Registry struct {
	mu      sync.Mutex
	entries map[string]*entry
	names   map[string]string // 小写别名 → 语言 ID（Monaco getLanguageIdByLanguageName）
	mimes   map[string]string // MIME → 语言 ID（Monaco getLanguageIdByMimeType）
}

type entry struct {
	grammar json.RawMessage
	once    sync.Once // 编译只做一次（结果含错误被缓存）；在 r.mu 之外执行
	lexer   *Lexer
	err     error
}

// compileGrammarFunc 是实际使用的编译函数；只有测试会替换它（模拟编译期 panic 与慢编译）。
var compileGrammarFunc = compileGrammar

// NewRegistry 返回装入全部内置语言（langs.Builtin）的注册表；语法在第一次 Lexer 时编译。
func NewRegistry() *Registry {
	r := &Registry{entries: map[string]*entry{}, names: map[string]string{}, mimes: map[string]string{}}
	for _, l := range langs.Builtin() {
		for _, a := range l.Aliases {
			if a != "" {
				r.names[strings.ToLower(a)] = l.ID
			}
		}
		mimes := l.MimeTypes
		if len(mimes) == 0 { // Monaco 给没有 MIME 的语言补 text/x-<id>
			mimes = []string{"text/x-" + l.ID}
		}
		for _, m := range mimes {
			r.mimes[m] = l.ID
		}
		if g := strings.TrimSpace(string(l.Grammar)); g != "" && g != "null" {
			r.entries[l.ID] = &entry{grammar: l.Grammar}
		}
	}
	return r
}

// Add 注册（或以同 ID 覆盖）一种语言的 Monarch 语法（JSON，正则写成 {"$regex","flags"}）。
// Add 只校验 JSON 语法；编译是惰性的，语法错误（未定义的状态、属性等）由之后的 Lexer(id) 返回。
// 覆盖后 Lexer(id) 返回新编译的 Lexer：之前为该语言产生的所有 State（包括嵌入了该语言的其他语言的
// State，它们持有旧的嵌入 Lexer）对新 Lexer 都无效，调用方须从 Initial() 重新分词。
// 旧 Lexer 在它自己的 State 上仍可继续使用。
func (r *Registry) Add(id string, grammar json.RawMessage) error {
	if id == "" {
		return fmt.Errorf("monarch: empty language id")
	}
	if _, err := parseJSON(grammar); err != nil {
		return fmt.Errorf("monarch: %s: %w", id, err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[id] = &entry{grammar: append(json.RawMessage(nil), grammar...)}
	return nil
}

// Lexer 返回语言的分词器；语言未注册或没有 Monarch 语法（如 plaintext）时返回错误。
// 第一次调用时编译语法，结果（含错误）被缓存。编译在 r.mu 之外进行：慢编译只让请求同一语言的
// 调用等待，不阻塞其他语言。编译期的 panic（不可信插件语法）转为错误。
func (r *Registry) Lexer(id string) (*Lexer, error) {
	r.mu.Lock()
	e, ok := r.entries[id]
	r.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("monarch: no tokenizer for language %q", id)
	}
	e.once.Do(func() { e.lexer, e.err = r.compile(id, e.grammar) })
	return e.lexer, e.err
}

func (r *Registry) compile(id string, src json.RawMessage) (lx *Lexer, err error) {
	defer func() {
		if p := recover(); p != nil {
			lx, err = nil, fmt.Errorf("monarch: %s: panic while compiling grammar: %v", id, p)
		}
	}()
	n, err := parseJSON(src)
	if err != nil {
		return nil, fmt.Errorf("monarch: %s: %w", id, err)
	}
	g, err := compileGrammarFunc(id, n)
	if err != nil {
		return nil, fmt.Errorf("monarch: %s: %w", id, err)
	}
	return newLexer(r, id, g), nil
}

// embeddedFor 对应 Monaco 的 computeNewStateForEmbeddedLanguage：按语言名（别名）、
// MIME、ID 的顺序解析 nextEmbedded 的值。
func (r *Registry) embeddedFor(name string) embedded {
	r.mu.Lock()
	id, ok := r.names[strings.ToLower(name)]
	if !ok {
		if id, ok = r.mimes[name]; !ok {
			id = name
		}
	}
	_, registered := r.entries[id]
	r.mu.Unlock()
	e := embedded{active: true, lang: id}
	if registered {
		if lx, err := r.Lexer(id); err == nil {
			e.lexer, e.state = lx, lx.initial
		}
	}
	return e
}
