package editor

import "github.com/tangoui/tango/ui/editor/monarch"

// ModelPool keeps one Model per URI, reference counted, so the editors
// showing the same document share its Model (Monaco's model service). Like
// Model it is not safe for concurrent use.
type ModelPool struct {
	reg    *monarch.Registry
	models map[string]*Model

	create, dispose listeners[func(*Model)]
	lang            listeners[func(*Model, string)]
}

// NewModelPool returns an empty pool whose Models take grammars from reg
// (see NewModel).
func NewModelPool(reg *monarch.Registry) *ModelPool {
	return &ModelPool{reg: reg, models: map[string]*Model{}}
}

// Open returns the Model for uri, adding one reference. The first Open
// creates it from text (see NewModel) and tells the OnCreate listeners; a
// later Open returns the same Model and ignores text. An Open of a URI
// whose Model is being disposed (from an OnWillDispose listener) creates a
// fresh Model from text, which replaces the dying one in the pool.
func (p *ModelPool) Open(uri, text string) *Model {
	if m := p.models[uri]; m != nil && !m.disposing {
		m.refs++
		return m
	}
	m := NewModel(uri, text, p.reg)
	m.pool, m.refs = p, 1
	p.models[uri] = m
	p.create.each(func(fn func(*Model)) { fn(m) })
	return m
}

// Get returns the open Model for uri, or nil. It adds no reference.
func (p *ModelPool) Get(uri string) *Model { return p.models[uri] }

// Release drops one reference to m (as returned by Open). When the last one
// goes, the OnWillDispose listeners are told (Get still returns m while
// they run), then m leaves the pool and its subscribers are dropped
// (States showing it stay valid to Close); a later Open of its URI creates
// a new Model. Releasing a Model the pool does not hold, or one being
// disposed, does nothing.
//
// The application stops the Highlighter's background goroutine it started
// for m (see Model) in an OnWillDispose listener.
func (p *ModelPool) Release(m *Model) {
	if m == nil || m.disposing || p.models[m.uri] != m {
		return
	}
	m.refs--
	if m.refs > 0 {
		return
	}
	m.disposing = true
	p.dispose.each(func(fn func(*Model)) { fn(m) })
	if p.models[m.uri] == m { // not replaced by an Open from a listener
		delete(p.models, m.uri)
	}
	m.subs.clear()
	m.pool = nil
}

// OnCreate calls fn with every Model the pool creates (Monaco's
// onDidCreateModel).
func (p *ModelPool) OnCreate(fn func(*Model)) (cancel func()) { return p.create.add(fn) }

// OnWillDispose calls fn with every Model about to leave the pool, while it
// is still usable (Monaco's onWillDisposeModel). This is where the
// application cancels the Model's highlighting goroutine.
func (p *ModelPool) OnWillDispose(fn func(*Model)) (cancel func()) { return p.dispose.add(fn) }

// OnLanguageChange calls fn after a pooled Model's SetLanguage, with the
// previous language id (Monaco's onDidChangeModelLanguage).
func (p *ModelPool) OnLanguageChange(fn func(m *Model, old string)) (cancel func()) {
	return p.lang.add(fn)
}

func (p *ModelPool) langChanged(m *Model, old string) {
	p.lang.each(func(fn func(*Model, string)) { fn(m, old) })
}
