package windows

import (
	"fmt"

	"github.com/tangoui/tango/internal/imebridge/proto"
	"github.com/tangoui/tango/internal/platform"
)

// The session router and all sink calls belong to the application thread.
type tipSessions struct {
	sink         platform.InputMethodSink
	next         uint64
	byID         map[uint64]*tipSession
	clients      map[*tipClient]bool
	modes        map[string]proto.Mode
	current      string
	convertCaret func(proto.Rect) platform.Rect
}

type tipClient struct {
	exe      string
	send     func([]proto.Message)
	sessions map[uint64]*tipSession
	menu     map[string]bool
}

type tipSession struct {
	id, remoteID uint64
	client       *tipClient
	active       bool
	caret        platform.Rect
	seq          uint64
	batch        []proto.Message
}

func newTIPSessions(sink platform.InputMethodSink, modes []proto.Mode) *tipSessions {
	m := &tipSessions{
		sink: sink, byID: make(map[uint64]*tipSession), clients: make(map[*tipClient]bool),
		modes: make(map[string]proto.Mode),
		convertCaret: func(r proto.Rect) platform.Rect {
			return platform.Rect{X: int(r.X), Y: int(r.Y), Width: int(r.Width), Height: int(r.Height)}
		},
	}
	for _, mode := range modes {
		m.modes[mode.ID] = mode
	}
	if len(modes) != 0 {
		m.current = modes[0].ID
	}
	return m
}

func (m *tipSessions) connect(exe string, send func([]proto.Message)) *tipClient {
	c := &tipClient{exe: exe, send: send, sessions: make(map[uint64]*tipSession)}
	m.clients[c] = true
	if m.current != "" {
		c.post(m.modes[m.current])
	}
	return c
}

func (m *tipSessions) disconnect(c *tipClient) {
	delete(m.clients, c)
	for remote := range c.sessions {
		m.end(c, remote)
	}
}

func (m *tipSessions) open(c *tipClient, remote uint64) {
	if remote == 0 || c.sessions[remote] != nil || len(c.sessions) >= 64 {
		return
	}
	m.next++
	session := &tipSession{id: m.next, remoteID: remote, client: c}
	c.sessions[remote], m.byID[session.id] = session, session
}

func (m *tipSessions) end(c *tipClient, remote uint64) {
	if s := c.sessions[remote]; s != nil {
		m.focus(s, false)
		delete(c.sessions, remote)
		delete(m.byID, s.id)
		m.sink.IMEEnd(s.id)
	}
}

func (m *tipSessions) focus(s *tipSession, on bool) {
	if s == nil || s.active == on {
		return
	}
	s.active = on
	if on {
		m.sink.IMEActivate(s.id)
	} else {
		m.sink.IMEDeactivate(s.id)
	}
}

func (m *tipSessions) setMode(id string, system bool) error {
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

func (c *tipClient) post(body any) {
	c.send([]proto.Message{{Body: body}})
}

func (s *tipSession) post(body any) {
	if s.seq != 0 {
		if len(s.batch) < 32 {
			s.batch = append(s.batch, proto.Message{Seq: s.seq, Body: body})
		}
		return
	}
	s.client.post(body)
}

func (m *tipSessions) dispatch(c *tipClient, msg proto.Message) {
	switch v := msg.Body.(type) {
	case proto.SessionOpen:
		m.open(c, uint64(v))
	case proto.SessionClose:
		m.end(c, uint64(v))
	case proto.Focus:
		m.focus(c.sessions[v.Session], v.On)
	case proto.TestKey:
		if msg.Seq != 0 {
			m.test(c, proto.Key(v), msg.Seq)
		}
	case proto.Key:
		if session := c.sessions[v.Session]; session != nil && session.active {
			m.key(session, v)
		}
	case proto.MenuOpen:
		if c.sessions[uint64(v)] != nil {
			m.sendMenu(c)
		}
	case proto.MenuPick:
		if sink, ok := m.sink.(platform.InputMethodMenus); ok && c.menu[string(v)] {
			sink.IMEMenuClicked(string(v))
			m.sendMenu(c)
		}
	case proto.ModePick:
		_ = m.setMode(string(v), true)
	}
}

func (m *tipSessions) sendMenu(c *tipClient) {
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

func (m *tipSessions) test(c *tipClient, key proto.Key, seq uint64) {
	s := c.sessions[key.Session]
	reply := proto.Message{Seq: seq, Body: proto.KeyResult{Session: key.Session}}
	if s == nil || !s.active {
		c.send([]proto.Message{reply})
		return
	}
	s.seq = seq
	defer func() { s.seq, s.batch = 0, nil }()
	reply.Body = proto.KeyResult{Session: key.Session, Eaten: m.key(s, key)}
	c.send(append(s.batch, reply))
}

func (m *tipSessions) key(s *tipSession, key proto.Key) bool {
	if key.Caret.Height > 0 {
		s.caret = m.convertCaret(key.Caret)
	}
	direction := key.Flags & (proto.KeyDown | proto.KeyUp)
	if direction != proto.KeyDown && direction != proto.KeyUp {
		return false
	}
	if mod, ok := tipModifierKey(key); ok {
		sink, exists := m.sink.(platform.InputMethodModifiers)
		return exists && sink.IMEWantsModifiers() && key.Flags&proto.KeyRepeat == 0 && sink.IMEModifier(s.id, mod)
	}
	return direction == proto.KeyDown && m.sink.IMEKey(s.id, tipKeyEvent(key))
}

// The low mandatory label permits local AppContainer access. The server still
// checks the peer's login session before dispatch.
func tipPipeSecurityDescriptor(sid string) string {
	return "D:P(A;;GA;;;" + sid + ")(A;;0x100083;;;AC)S:(ML;;NW;;;LW)"
}
