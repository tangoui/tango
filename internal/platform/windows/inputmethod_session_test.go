package windows

import (
	"slices"
	"strings"
	"testing"

	"github.com/tangoui/tango/internal/imebridge/proto"
	"github.com/tangoui/tango/internal/platform"
)

type imeSink struct {
	keys   []uint64
	ended  []uint64
	mods   []platform.IMEModifierKey
	modes  []string
	clicks []string
	menu   []platform.IMEMenuItem
	onKey  func(uint64)
}

func (s *imeSink) IMEKey(id uint64, _ platform.IMEKey) bool {
	s.keys = append(s.keys, id)
	if s.onKey != nil {
		s.onKey(id)
	}
	return true
}
func (*imeSink) IMEActivate(uint64)          {}
func (*imeSink) IMEDeactivate(uint64)        {}
func (*imeSink) IMECommitComposition(uint64) {}
func (s *imeSink) IMEEnd(id uint64)          { s.ended = append(s.ended, id) }
func (*imeSink) IMEWantsModifiers() bool     { return true }
func (s *imeSink) IMEModifier(_ uint64, m platform.IMEModifierKey) bool {
	s.mods = append(s.mods, m)
	return m.Down
}
func (s *imeSink) IMEModeChanged(id string)        { s.modes = append(s.modes, id) }
func (s *imeSink) IMEMenu() []platform.IMEMenuItem { return s.menu }
func (s *imeSink) IMEMenuClicked(id string)        { s.clicks = append(s.clicks, id) }

func TestSessionMux(t *testing.T) {
	sink := &imeSink{}
	m := newTIPSessions(sink, nil)
	var outA, outB []proto.Message
	a := m.connect("a.exe", func(batch []proto.Message) { outA = append(outA, batch...) })
	b := m.connect("b.exe", func(batch []proto.Message) { outB = append(outB, batch...) })
	sink.onKey = func(id uint64) { m.byID[id].post(proto.Commit{Session: m.byID[id].remoteID, Text: "你"}) }
	for _, c := range []*tipClient{a, b} {
		m.dispatch(c, proto.Message{Body: proto.SessionOpen(1)})
		m.dispatch(c, proto.Message{Body: proto.Focus{Session: 1, On: true}})
		m.dispatch(c, proto.Message{Seq: 5, Body: proto.TestKey{Session: 1, VK: 'N', Flags: proto.KeyDown}})
	}
	if len(sink.keys) != 2 || sink.keys[0] == sink.keys[1] ||
		m.byID[sink.keys[0]].client != a || m.byID[sink.keys[1]].client != b {
		t.Fatalf("client routing: %v", sink.keys)
	}
	for _, out := range [][]proto.Message{outA, outB} {
		if len(out) != 2 || out[0].Seq != 5 || out[1].Seq != 5 ||
			out[0].Body != (proto.Commit{Session: 1, Text: "你"}) ||
			out[1].Body != (proto.KeyResult{Session: 1, Eaten: true}) {
			t.Fatalf("key edits/results = %v", out)
		}
	}
	m.dispatch(a, proto.Message{Body: proto.SessionClose(1)})
	m.disconnect(b)
	if !slices.Equal(sink.ended, sink.keys) || len(m.byID) != 0 {
		t.Fatalf("ended %v, keys %v, live %v", sink.ended, sink.keys, m.byID)
	}
}

func TestModifierUpDown(t *testing.T) {
	sink := &imeSink{}
	m := newTIPSessions(sink, nil)
	var out []proto.Message
	c := m.connect("host.exe", func(batch []proto.Message) { out = append(out, batch...) })
	m.dispatch(c, proto.Message{Body: proto.SessionOpen(11)})
	m.dispatch(c, proto.Message{Body: proto.Focus{Session: 11, On: true}})
	m.dispatch(c, proto.Message{Seq: 1, Body: proto.TestKey{Session: 11, VK: 0x10, Scan: 0x36, Flags: proto.KeyDown, Mods: proto.ModShift}})
	m.dispatch(c, proto.Message{Seq: 2, Body: proto.TestKey{Session: 11, VK: 0x10, Scan: 0x36, Flags: proto.KeyUp}})
	want := []platform.IMEModifierKey{{Code: "ShiftRight", Down: true, Shift: true}, {Code: "ShiftRight"}}
	if !slices.Equal(sink.mods, want) || len(out) != 2 ||
		out[0].Body != (proto.KeyResult{Session: 11, Eaten: true}) ||
		out[1].Body != (proto.KeyResult{Session: 11, Eaten: false}) {
		t.Fatalf("modifiers = %v, replies = %v", sink.mods, out)
	}
}

func TestModeBroadcast(t *testing.T) {
	sink := &imeSink{}
	m := newTIPSessions(sink, []proto.Mode{{ID: "zh", Short: "中"}, {ID: "en", Short: "英"}})
	var a, b []proto.Message
	m.connect("a.exe", func(batch []proto.Message) { a = append(a, batch...) })
	c := m.connect("b.exe", func(batch []proto.Message) { b = append(b, batch...) })
	if err := m.setMode("en", false); err != nil {
		t.Fatal(err)
	}
	m.dispatch(c, proto.Message{Body: proto.ModePick("zh")})
	m.dispatch(c, proto.Message{Body: proto.ModePick("zh")})
	if len(a) != 3 || len(b) != 3 || a[1].Body != (proto.Mode{ID: "en", Short: "英"}) ||
		b[1].Body != a[1].Body || !slices.Equal(sink.modes, []string{"zh"}) {
		t.Fatalf("broadcasts %v, %v; callbacks %v", a, b, sink.modes)
	}
}

func TestMenuRoundTrip(t *testing.T) {
	sink := &imeSink{menu: []platform.IMEMenuItem{{ID: "about", Title: "About", Checked: true},
		{ID: "inactive", Title: "Inactive", Disabled: true}}}
	m := newTIPSessions(sink, nil)
	var out []proto.Message
	c := m.connect("a.exe", func(batch []proto.Message) { out = append(out, batch...) })
	m.dispatch(c, proto.Message{Body: proto.SessionOpen(2)})
	m.dispatch(c, proto.Message{Body: proto.MenuOpen(2)})
	m.dispatch(c, proto.Message{Body: proto.MenuPick("inactive")})
	m.dispatch(c, proto.Message{Body: proto.MenuPick("about")})
	if len(out) != 2 || !slices.Equal(out[0].Body.(proto.Menu).Items, []proto.MenuItem{
		{ID: "about", Title: "About", Checked: true},
		{ID: "inactive", Title: "Inactive", Disabled: true},
	}) || !slices.Equal(sink.clicks, []string{"about"}) {
		t.Fatalf("menu %v, clicks %v", out, sink.clicks)
	}
}

func TestPipeSecurityDescriptor(t *testing.T) {
	const sid = "S-1-5-21-1-2-3-1001"
	got := tipPipeSecurityDescriptor(sid)
	for _, want := range []string{sid, ";;;AC)", "S:(ML;;NW;;;LW)"} {
		if !strings.Contains(got, want) {
			t.Errorf("SDDL %q missing %q", got, want)
		}
	}
}

func TestCandidateBoundsWindows(t *testing.T) {
	physical := proto.Rect{X: -300, Y: 153, Width: 3, Height: 30}
	want := platform.Rect{X: -200, Y: 102, Width: 2, Height: 20}
	if got := tipLogicalCaret(physical, 144); got != want {
		t.Fatalf("caret = %+v, want %+v", got, want)
	}
}
