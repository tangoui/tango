package windows

import (
	"github.com/tangoui/tango/internal/imebridge/proto"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/imesession"
)

func newTIPSessions(sink platform.InputMethodSink, modes []proto.Mode) *imesession.Mux {
	return imesession.New(sink, modes, func(m *imesession.Mux, s *imesession.Session, key proto.Key) bool {
		return tipRouteKey(m, s, key, func(r proto.Rect) platform.Rect { return tipLogicalCaret(r, baseDPI) })
	})
}

func tipRouteKey(m *imesession.Mux, s *imesession.Session, key proto.Key, caret func(proto.Rect) platform.Rect) bool {
	if key.Caret.Height > 0 {
		s.SetCaret(caret(key.Caret))
	}
	direction := key.Flags & (proto.KeyDown | proto.KeyUp)
	if direction != proto.KeyDown && direction != proto.KeyUp {
		return false
	}
	if mod, ok := tipModifierKey(key); ok {
		return key.Flags&proto.KeyRepeat == 0 && m.Modifier(s, mod)
	}
	return direction == proto.KeyDown && m.Key(s, tipKeyEvent(key))
}

// The low mandatory label permits local AppContainer access. The server still
// checks the peer's login session before dispatch.
func tipPipeSecurityDescriptor(sid string) string {
	return "D:P(A;;GA;;;" + sid + ")(A;;0x120083;;;AC)S:(ML;;NW;;;LW)"
}

func tipLogicalCaret(r proto.Rect, dpi int) platform.Rect {
	x, y := pxToDIP(int(r.X), dpi), pxToDIP(int(r.Y), dpi)
	return platform.Rect{
		X: x, Y: y,
		Width:  max(1, pxToDIP(int(r.X)+int(r.Width), dpi)-x),
		Height: max(1, pxToDIP(int(r.Y)+int(r.Height), dpi)-y),
	}
}
