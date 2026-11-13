// Package editor is the code editor. It follows Monaco's split between the
// text model and the editor:
//
//   - A Model is one document: its text, language, highlighter and undo
//     history. Several editors can show the same Model (a split view); a
//     ModelPool keeps one Model per URI.
//   - A State is one editor's view of a Model: its selections, viewport and
//     Options. The application holds it and passes it to the View every
//     frame. SaveViewState / RestoreViewState carry the per-editor state
//     across model switches.
//
// The Model and State parts of the package do not depend on package ui:
// they measure no text and draw nothing. The View sets the Viewport sizes it
// measured (line height, viewport and content widths) and passes caret x
// positions to RevealX; State turns keyboard-level intents (Move, Insert,
// Delete, Undo, Redo, ScrollBy) into Model edits, selection changes and
// scroll positions.
//
// Offsets are UTF-8 byte offsets into the buffer's LF-normalised text (see
// package buffer). Caret movement by Char steps over grapheme clusters; see
// grapheme.go for the implemented subset of UAX #29.
//
// Every edit, undo and redo is passed to the Model's Highlighter as edits
// (buffer.Buffer.LastChange for undo and redo), so it never has to
// re-tokenize the whole document after an undo.
package editor

import (
	"path"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/highlight"
	"github.com/tangoui/tango/ui/editor/langs"
	"github.com/tangoui/tango/ui/editor/monarch"
	"github.com/tangoui/tango/ui/editor/regex"
)

// PlainText is the language of a Model whose URI matches no language.
const PlainText = "plaintext"

var defaultRegistry = sync.OnceValue(monarch.NewRegistry)

// DefaultRegistry returns the process-wide grammar registry NewState uses
// (all built-in languages). Grammars added to it (plugins) are seen by
// Models created afterwards; existing Models must Reset their Highlighter
// with the new Lexer (see monarch.Registry.Add) or SetLanguage again.
func DefaultRegistry() *monarch.Registry { return defaultRegistry() }

// ModelChange describes one change of a Model, passed to its subscribers
// synchronously, after the change (Model.Snapshot is New).
type ModelChange struct {
	// Old and New are the snapshots before and after the change.
	Old, New buffer.Snapshot
	// Edits turn Old into New (sorted, non-overlapping, in Old's
	// offsets, as buffer.Buffer.Edit takes them). Empty for language and
	// EOL changes.
	Edits []buffer.Edit
	// Source is the view that made the edit; for undo and redo, the view
	// that ran them. Nil for changes made on the Model itself (SetEOL,
	// SetLanguage).
	Source *State
	// Undo and Redo mark undo and redo; Language a SetLanguage (Old and
	// New are then the same snapshot); EOL a SetEOL or the undo or redo of
	// one.
	Undo, Redo, Language, EOL bool
}

// Model is one document: its text and undo history (a buffer.Buffer), its
// language and configuration, and the Highlighter all the views of the
// document share. Create one with NewModel or ModelPool.Open and show it
// with NewView. Like State it is not safe for concurrent use; drive it from
// the UI goroutine (its Highlighter is itself safe for its background
// goroutine).
//
// The application owns the Highlighter's background goroutine
// (highlight.Highlighter.Start): it starts it once, when Highlighter first
// returns non-nil, and cancels its context when the Model goes away (for a
// pooled Model, in ModelPool.OnWillDispose). The Model keeps that one
// Highlighter for its whole life, across SetLanguage, so the goroutine stays
// valid. A Model that starts as plain text has a nil Highlighter until
// SetLanguage switches it to a language with a grammar: start the goroutine
// from the callback that reports that switch, ModelPool.OnLanguageChange for
// a pooled Model, or a Subscribe callback receiving a ModelChange with
// Language set otherwise (check Highlighter() != nil there, and start it only
// once).
//
// The text changes only through the views (State's Insert, Delete, Edit,
// Undo and Redo) and SetEOL: a direct Buffer().Edit, Undo or Redo bypasses
// the Highlighter (which then re-tokenizes the whole document), the
// subscribers and the mapping of the views' selections.
type Model struct {
	uri  string
	buf  *buffer.Buffer
	reg  *monarch.Registry
	pool *ModelPool // nil unless opened through a pool

	lang   string
	conf   *langs.Conf
	wordRe *regexp.Regexp         // nil: Unicode letters, digits, marks and '_'
	hl     *highlight.Highlighter // never nil; its lexer is nil for plain text
	plain  bool                   // no grammar: hl is not fed edits

	subs listeners[func(ModelChange)]

	// Indentation options (Monaco's model options tabSize and
	// insertSpaces), shared by every view.
	tabSize      int
	insertSpaces bool

	// lastSource is the view that made the last edit: an edit from
	// another view starts a new undo step.
	lastSource *State
	refs       int  // ModelPool references
	disposing  bool // the pool is disposing it (OnWillDispose running or done)

	// Changes requested from inside a subscriber run after the
	// notification; see Subscribe.
	notifying bool
	draining  bool
	queue     []queued
	log       [][]buffer.Edit // edits of the changes made while draining
}

// queued is a change requested during a notification. run gets the edits
// of the changes applied since it was requested, to map its offsets.
type queued struct {
	run  func(since [][]buffer.Edit)
	mark int // len(log) when queued
}

// NewModel returns a Model holding text, with the language chosen from the
// file name in uri (langs.ForFilename; for a URI with a scheme, such as
// "file:///src/main.go", its path) and grammars from reg. An unknown
// language is PlainText; a language without a Monarch grammar, or a nil
// reg, shows no highlighting (Highlighter returns nil).
func NewModel(uri, text string, reg *monarch.Registry) *Model {
	m := &Model{uri: uri, buf: buffer.New(text), reg: reg, hl: highlight.New(nil), tabSize: DefaultTabSize, insertSpaces: true}
	id := PlainText
	if l := langs.ForFilename(uriPath(uri)); l != nil {
		id = l.ID
	}
	m.setLanguage(id)
	return m
}

// uriPath returns the path part of uri ("file:///a/b.go" → "/a/b.go"), or
// uri itself when it has no scheme (a file name or path).
func uriPath(uri string) string {
	i := strings.Index(uri, "://")
	if i <= 0 {
		return uri
	}
	p := uri[i+3:]
	if j := strings.IndexAny(p, "?#"); j >= 0 {
		p = p[:j]
	}
	if j := strings.IndexByte(p, '/'); j >= 0 {
		return p[j:]
	}
	return path.Base(p)
}

// URI returns the Model's URI (the file name for NewState's private model).
func (m *Model) URI() string { return m.uri }

// Buffer returns the text and undo history. Read it freely (Snapshot,
// AltVersion, Anchor/Resolve, PushUndoStop); change the text only through
// the views and SetEOL (see Model).
func (m *Model) Buffer() *buffer.Buffer { return m.buf }

// Snapshot returns the current text.
func (m *Model) Snapshot() buffer.Snapshot { return m.buf.Snapshot() }

// Language returns the language id, PlainText when none.
func (m *Model) Language() string { return m.lang }

// Conf returns the language configuration, nil for plain text.
func (m *Model) Conf() *langs.Conf { return m.conf }

// Highlighter returns the Highlighter, nil while the language has no
// grammar (plain text; milestone 1's contract). The views call Highlight
// and Tokens on it; the application may Start its background goroutine (see
// Model). The Model has one Highlighter for its whole life: SetLanguage
// Resets it with the new grammar (with none for plain text, when this
// returns nil but the instance and its goroutine live on), so a language
// round trip returns the same, still started, Highlighter.
func (m *Model) Highlighter() *highlight.Highlighter {
	if m.plain {
		return nil
	}
	return m.hl
}

// SetLanguage switches to language id (a built-in language id such as
// "go"; anything else is PlainText): the configuration, the word pattern
// and the Highlighter follow, every line is re-tokenized, and subscribers
// and the pool's OnLanguageChange listeners are told. Setting the current
// language does nothing. Called from a subscriber, it runs after the
// notification (see Subscribe).
func (m *Model) SetLanguage(id string) {
	if m.notifying {
		m.enqueue(func([][]buffer.Edit) { m.SetLanguage(id) })
		return
	}
	if langs.ByID(id) == nil {
		id = PlainText
	}
	if id == m.lang {
		return
	}
	old := m.lang
	m.setLanguage(id)
	s := m.buf.Snapshot()
	m.notify(ModelChange{Old: s, New: s, Language: true})
	if m.pool != nil {
		m.pool.langChanged(m, old)
	}
}

func (m *Model) setLanguage(id string) {
	m.lang, m.conf, m.wordRe = id, nil, nil
	var lx *monarch.Lexer
	if l := langs.ByID(id); l != nil {
		m.conf = &l.Conf
		m.wordRe = compileWordPattern(l.Conf.WordPattern)
		if m.reg != nil {
			if x, err := m.reg.Lexer(l.ID); err == nil {
				lx = x
			}
		}
	}
	wasPlain := m.plain
	m.plain = lx == nil
	if wasPlain && m.plain {
		return
	}
	m.hl.Reset(lx)
	if wasPlain {
		// Edits were not passed on while it was plain text.
		m.hl.Sync(m.buf.Snapshot())
	}
}

// TabSize returns the document's tab width in columns (Monaco's model
// option tabSize; DefaultTabSize for a new Model). Every view of the Model
// uses it for tab stops, Tab, outdent and re-indenting; a View's TabSize
// modifier overrides it for drawing and caret movement for one frame.
func (m *Model) TabSize() int { return m.tabSize }

// SetTabSize sets the tab width (Monaco's updateOptions({tabSize})); n <= 0
// means DefaultTabSize. Views see it from their next command or frame.
func (m *Model) SetTabSize(n int) {
	if n <= 0 {
		n = DefaultTabSize
	}
	m.tabSize = n
}

// InsertSpaces reports whether Tab and the indentation commands indent
// with spaces rather than tab characters (Monaco's model option
// insertSpaces; true for a new Model).
func (m *Model) InsertSpaces() bool { return m.insertSpaces }

// SetInsertSpaces sets InsertSpaces (Monaco's updateOptions({insertSpaces}));
// editor.action.indentationToSpaces / ToTabs set it too.
func (m *Model) SetInsertSpaces(on bool) { m.insertSpaces = on }

// EOL returns the line ending the document is saved with, "\n" or "\r\n"
// (see buffer.Snapshot.TextWithEOL).
func (m *Model) EOL() string { return m.buf.Snapshot().EOL() }

// SetEOL sets the line ending, "\n" or "\r\n", as an undo step of its own
// (Monaco's pushEOL); anything else, or the current ending, does nothing.
// Called from a subscriber, it runs after the notification (see Subscribe).
func (m *Model) SetEOL(eol string) {
	if m.notifying {
		m.enqueue(func([][]buffer.Edit) { m.SetEOL(eol) })
		return
	}
	old := m.buf.Snapshot()
	ns, ok := m.buf.SetEOL(eol, nil)
	if !ok {
		return
	}
	if !m.plain {
		m.hl.Edit(old, ns, nil)
	}
	m.lastSource = nil
	m.notify(ModelChange{Old: old, New: ns, EOL: true})
}

// Subscribe calls fn after every change of the Model, synchronously, until
// cancel is called. Listeners run in subscription order; they may cancel
// themselves or subscribe others (seen from the next change).
//
// A listener may change the Model (edit through a view, Undo, Redo,
// SetEOL, SetLanguage): the change is queued and applied once every
// listener has seen the current change, so all of them, and every view's
// selection, see the changes in one order. Queued changes run first in,
// first out; an edit's offsets (in the text the listener saw) are mapped
// through the queued changes applied before it, and an edit that no longer
// applies is dropped. Changes requested while the queue is applied are
// queued behind it.
func (m *Model) Subscribe(fn func(ModelChange)) (cancel func()) { return m.subs.add(fn) }

func (m *Model) notify(ch ModelChange) {
	if m.draining {
		m.log = append(m.log, ch.Edits)
	}
	m.notifying = true
	m.subs.each(func(fn func(ModelChange)) { fn(ch) })
	m.notifying = false
	if !m.draining && len(m.queue) > 0 {
		m.drain()
	}
}

// enqueue queues a change requested during a notification.
func (m *Model) enqueue(run func(since [][]buffer.Edit)) {
	m.queue = append(m.queue, queued{run: run, mark: len(m.log)})
}

func (m *Model) drain() {
	m.draining = true
	for len(m.queue) > 0 {
		q := m.queue[0]
		m.queue = m.queue[1:]
		q.run(m.log[q.mark:])
	}
	m.queue, m.log, m.draining = nil, nil, false
}

// mapEdits maps edits made on an earlier text through the changes since
// (see mapOffset).
func mapEdits(edits []buffer.Edit, since [][]buffer.Edit) []buffer.Edit {
	if len(since) == 0 {
		return edits
	}
	out := make([]buffer.Edit, len(edits))
	for i, e := range edits {
		for _, c := range since {
			e.Start, e.End = mapOffset(c, e.Start), mapOffset(c, e.End)
			e.End = max(e.End, e.Start)
		}
		out[i] = e
	}
	return out
}

// edit applies edits for view src, recording before and after as the
// selections for undo. When the text changed, it updates the highlighter,
// calls done with the new snapshot (the source view updates its selection
// there, before the subscribers run) and notifies.
func (m *Model) edit(src *State, edits []buffer.Edit, before, after []Selection, done func(buffer.Snapshot)) error {
	if m.lastSource != nil && m.lastSource != src {
		m.buf.PushUndoStop() // another view's edit: a step of its own
	}
	old := m.buf.Snapshot()
	ns, err := m.buf.Edit(edits, before, after)
	if err != nil {
		return err
	}
	if ns.Version() == old.Version() {
		done(ns)
		return nil
	}
	if !m.plain {
		m.hl.Edit(old, ns, edits)
	}
	m.lastSource = src
	done(ns)
	m.notify(ModelChange{Old: old, New: ns, Edits: edits, Source: src})
	return nil
}

// history runs op (Buffer.Undo or Redo) for view src; done receives the
// new snapshot and the selections op restored.
func (m *Model) history(src *State, redo bool, done func(buffer.Snapshot, []Selection)) bool {
	op := m.buf.Undo
	if redo {
		op = m.buf.Redo
	}
	eol := m.buf.Snapshot().EOL()
	ns, sels, ok := op()
	if !ok {
		return false
	}
	old, edits := m.buf.LastChange()
	if !m.plain {
		m.hl.Edit(old, ns, edits)
	}
	m.lastSource = nil
	done(ns, sels)
	m.notify(ModelChange{Old: old, New: ns, Edits: edits, Source: src, Undo: !redo, Redo: redo, EOL: ns.EOL() != eol})
	return true
}

// mapOffset maps off, an offset in the text edits apply to, into the text
// they produce, as Monaco moves the cursors of the other views: an insert
// exactly at off pushes it right, an offset inside a deleted or replaced
// range moves to the range's start, and edits before off shift it.
func mapOffset(edits []buffer.Edit, off int) int {
	delta := 0
	for _, e := range edits {
		if e.Start == e.End && e.Text == "" {
			continue
		}
		n := len(e.Text) - strings.Count(e.Text, "\r\n") // as Apply stores it
		switch {
		case off < e.Start:
			return off + delta
		case e.Start == e.End: // an insert at or before off
		case off < e.End:
			return e.Start + delta
		}
		delta += n - (e.End - e.Start)
	}
	return off + delta
}

// compileWordPattern translates a Monaco wordPattern to Go syntax. Patterns
// RE2 cannot express (lookaround, backreferences) fall back to the default
// word definition.
func compileWordPattern(p *langs.RegexSpec) *regexp.Regexp {
	if p == nil {
		return nil
	}
	src, ok := regex.Translate(p.Source, p.Flags)
	if !ok {
		return nil
	}
	re, err := regexp.Compile(src)
	if err != nil {
		return nil
	}
	return re
}

// listeners is a list of callbacks with cancel functions; the zero value is
// empty.
type listeners[F any] struct {
	next int
	list []listener[F]
}

type listener[F any] struct {
	id int
	fn F
}

func (l *listeners[F]) add(fn F) (cancel func()) {
	l.next++
	id := l.next
	l.list = append(l.list, listener[F]{id, fn})
	return func() {
		l.list = slices.DeleteFunc(l.list, func(x listener[F]) bool { return x.id == id })
	}
}

// each calls f with every listener registered when it starts, skipping the
// ones cancelled meanwhile.
func (l *listeners[F]) each(f func(F)) {
	if len(l.list) == 0 {
		return
	}
	snap := slices.Clone(l.list)
	for _, x := range snap {
		if slices.ContainsFunc(l.list, func(y listener[F]) bool { return y.id == x.id }) {
			f(x.fn)
		}
	}
}

func (l *listeners[F]) clear() { l.list = nil }
