// Package imesession routes system input method sessions on the app thread.
package imesession

import (
	"fmt"
	"unicode/utf16"

	"github.com/tangoui/tango/internal/imebridge/proto"
	"github.com/tangoui/tango/internal/platform"
)

// The session router and all sink calls belong to the application thread.
type Mux struct {
	sink     platform.InputMethodSink
	next     uint64
	byID     map[uint64]*Session
	clients  map[*Client]bool
	modes    map[string]proto.Mode
	current  string
	routeKey func(*Mux, *Session, proto.Key) bool
}

type Client struct {
	exe      string
	send     func([]proto.Message)
	sessions map[uint64]*Session
	menu     map[string]bool
}

type Session struct {
	id, remoteID      uint64
	client            *Client
	active            bool
	caret             platform.Rect
	exe               string
	seq               uint64
	batch             []proto.Message
	candidateRevision uint64
	candidateCount    int
}

func New(sink platform.InputMethodSink, modes []proto.Mode, routeKey func(*Mux, *Session, proto.Key) bool) *Mux {
	m := &Mux{
		sink: sink, byID: make(map[uint64]*Session), clients: make(map[*Client]bool),
		modes:    make(map[string]proto.Mode),
		routeKey: routeKey,
	}
	for _, mode := range modes {
		m.modes[mode.ID] = mode
	}
	if len(modes) != 0 {
		m.current = modes[0].ID
	}
	return m
}

func (m *Mux) Connect(exe string, send func([]proto.Message)) *Client {
	c := &Client{exe: exe, send: send, sessions: make(map[uint64]*Session)}
	m.clients[c] = true
	if m.current != "" {
		c.post(m.modes[m.current])
	}
	return c
}

func (m *Mux) Disconnect(c *Client) {
	delete(m.clients, c)
	for remote := range c.sessions {
		m.End(c, remote)
	}
}

func (m *Mux) Open(c *Client, remote uint64) *Session {
	if remote == 0 || c.sessions[remote] != nil || len(c.sessions) >= 64 {
		return nil
	}
	m.next++
	session := &Session{id: m.next, remoteID: remote, client: c, exe: c.exe}
	c.sessions[remote], m.byID[session.id] = session, session
	return session
}

func (m *Mux) End(c *Client, remote uint64) {
	if s := c.sessions[remote]; s != nil {
		m.Focus(s, false)
		delete(c.sessions, remote)
		delete(m.byID, s.id)
		m.sink.IMEEnd(s.id)
	}
}

func (m *Mux) Focus(s *Session, on bool) {
	if s == nil || s.active == on {
		return
	}
	s.active = on
	if on {
		m.sink.IMEActivate(s.id)
	} else {
		s.candidateCount = 0
		m.sink.IMEDeactivate(s.id)
	}
}

func (m *Mux) SetMode(id string, system bool) error {
	if len(m.modes) == 0 {
		return platform.ErrUnsupported
	}
	mode, ok := m.modes[id]
	if !ok {
		return fmt.Errorf("ime: undeclared input mode %q", id)
	}
	if id == m.current {
		return nil
	}
	m.current = id
	for client := range m.clients {
		client.post(mode)
	}
	if system {
		if sink, ok := m.sink.(platform.InputMethodModeSink); ok {
			sink.IMEModeChanged(id)
		}
	}
	return nil
}

func (c *Client) post(body any) {
	c.send([]proto.Message{{Body: body}})
}

const (
	maxEditTextUnits = 16 * 1024 // UTF-16 code units per text frame, within proto.MaxFrame
	maxReplyEdits    = 32        // Maximum edits accepted by the bridges per reply
)

func (s *Session) Post(body any) {
	if list, ok := body.(proto.Candidates); ok {
		s.candidateRevision++
		s.candidateCount = len(list.Items)
		list.Revision = s.candidateRevision
		body = list
	}
	limit := maxReplyEdits - len(s.batch)
	if limit == 0 {
		return
	}
	switch edit := body.(type) {
	case proto.Commit:
		s.candidateCount = 0
		// Keep all parts in one batch, before the key result if replying.
		remaining := edit.Text
		var batch []proto.Message
		for range limit {
			end := editTextEnd(remaining)
			edit.Text, remaining = remaining[:end], remaining[end:]
			batch = append(batch, proto.Message{Seq: s.seq, Body: edit})
			if remaining == "" {
				break
			}
		}
		s.post(batch)
		return
	case proto.Compose:
		edit.Text = edit.Text[:editTextEnd(edit.Text)]
		body = edit
	}
	s.post([]proto.Message{{Seq: s.seq, Body: body}})
}

func (s *Session) post(batch []proto.Message) {
	if s.seq != 0 {
		s.batch = append(s.batch, batch...)
		return
	}
	s.client.send(batch)
}

func editTextEnd(text string) int {
	units := 0
	for i, r := range text {
		units += utf16.RuneLen(r)
		if units > maxEditTextUnits {
			return i
		}
	}
	return len(text)
}

func (m *Mux) Dispatch(c *Client, msg proto.Message) {
	switch v := msg.Body.(type) {
	case proto.SessionOpen:
		m.Open(c, uint64(v))
	case proto.SessionClose:
		m.End(c, uint64(v))
	case proto.Reset:
		if session := c.sessions[uint64(v)]; session != nil {
			m.CommitComposition(session)
		}
	case proto.Focus:
		s := c.sessions[v.Session]
		if msg.Seq != 0 {
			m.reply(c, v.Session, msg.Seq, s, func() bool {
				m.Focus(s, v.On)
				return false
			})
		} else {
			m.Focus(s, v.On)
		}
	case proto.TestKey:
		if msg.Seq != 0 {
			m.test(c, proto.Key(v), msg.Seq)
		}
	case proto.CandidatePick:
		if msg.Seq != 0 {
			session := c.sessions[v.Session]
			m.reply(c, v.Session, msg.Seq, session, func() bool {
				if v.Revision == 0 || v.Revision != session.candidateRevision ||
					uint64(v.Index) >= uint64(session.candidateCount) {
					return false
				}
				return m.SelectCandidate(session, int(v.Index))
			})
		}
	case proto.Key:
		if session := c.sessions[v.Session]; session != nil && session.active && m.routeKey != nil {
			m.routeKey(m, session, v)
		}
	case proto.MenuOpen:
		if c.sessions[uint64(v)] != nil {
			m.SendMenu(c)
		}
	case proto.MenuPick:
		if sink, ok := m.sink.(platform.InputMethodMenus); ok && c.menu[string(v)] {
			sink.IMEMenuClicked(string(v))
			m.SendMenu(c)
		}
	case proto.ModePick:
		_ = m.SetMode(string(v), true)
	}
}

func (m *Mux) SendMenu(c *Client) {
	var menu proto.Menu
	c.menu = make(map[string]bool)
	if sink, ok := m.sink.(platform.InputMethodMenus); ok {
		for _, item := range sink.IMEMenu() {
			menu.Items = append(menu.Items, proto.MenuItem(item))
			if item.ID != "" && !item.Disabled && !item.Separator {
				c.menu[item.ID] = true
			}
		}
	}
	c.post(menu)
}

func (m *Mux) test(c *Client, key proto.Key, seq uint64) {
	session := c.sessions[key.Session]
	m.reply(c, key.Session, seq, session, func() bool {
		return session.active && m.routeKey != nil && m.routeKey(m, session, key)
	})
}

func (m *Mux) reply(c *Client, remote, seq uint64, session *Session, apply func() bool) {
	reply := proto.Message{Seq: seq, Body: proto.KeyResult{Session: remote}}
	if session == nil {
		c.send([]proto.Message{reply})
		return
	}
	session.seq = seq
	defer func() { session.seq, session.batch = 0, nil }()
	reply.Body = proto.KeyResult{Session: remote, Eaten: apply()}
	c.send(append(session.batch, reply))
}

// Key delivers an event from an in-process transport.
func (m *Mux) Key(s *Session, key platform.IMEKey) bool {
	return s.active && m.sink.IMEKey(s.id, key)
}

// Modifier delivers an event if the app subscribed to modifier changes.
func (m *Mux) Modifier(s *Session, key platform.IMEModifierKey) bool {
	sink, ok := m.sink.(platform.InputMethodModifiers)
	return s.active && ok && sink.IMEWantsModifiers() && sink.IMEModifier(s.id, key)
}

func (m *Mux) CommitComposition(s *Session) {
	m.sink.IMECommitComposition(s.id)
}

func (m *Mux) SelectCandidate(s *Session, index int) bool {
	sink, ok := m.sink.(platform.InputMethodCandidateSink)
	if !ok || s == nil || !s.active || index < 0 {
		return false
	}
	sink.IMECandidateSelected(s.id, index)
	return true
}

func (m *Mux) Session(id uint64) *Session { return m.byID[id] }
func (m *Mux) CurrentMode() string        { return m.current }

func (s *Session) ID() uint64                  { return s.id }
func (s *Session) RemoteID() uint64            { return s.remoteID }
func (s *Session) Client() *Client             { return s.client }
func (s *Session) Caret() platform.Rect        { return s.caret }
func (s *Session) SetCaret(rect platform.Rect) { s.caret = rect }
func (s *Session) ClientApp() string           { return s.exe }
func (s *Session) SetClientApp(exe string)     { s.exe = exe }
