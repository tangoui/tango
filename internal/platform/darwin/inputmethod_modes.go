//go:build darwin && !ios

package darwin

import (
	"fmt"
	"log"
	"slices"

	"github.com/ebitengine/purego/objc"
	"github.com/tangoui/tango/internal/platform"
)

const imeDocumentInputModeTag = 0x696d696d // kTSMDocumentInputModePropertyTag, 'imim'

type imeModes struct {
	bundleID   string
	ids        []string
	current    string
	pending    bool // an application selection awaiting an active client
	source     imeInputSource
	lastSource string
	request    uint64
}

var _ platform.InputMethodModes = (*backend)(nil)

func imeModesFromBundle(bundle id, bundleID string) imeModes {
	m := imeModes{bundleID: bundleID}
	list := send(bundle, "objectForInfoDictionaryKey:", uintptr(nsString("TangoInputModes")))
	if !sendBool(list, "isKindOfClass:", uintptr(class("NSArray"))) {
		return m
	}
	count := send(list, "count")
	for i := id(0); i < count; i++ {
		item := send(list, "objectAtIndex:", uintptr(i))
		if !sendBool(item, "isKindOfClass:", uintptr(class("NSDictionary"))) {
			continue
		}
		v := send(item, "objectForKey:", uintptr(nsString("id")))
		if sendBool(v, "isKindOfClass:", uintptr(class("NSString"))) {
			if key := goString(v); key != "" {
				m.ids = append(m.ids, key)
			}
		}
	}
	if len(m.ids) != 0 {
		m.current = m.ids[0]
	}
	return m
}

func (*backend) IMESetMode(mode string) error {
	m := &imeHost.modes
	if len(m.ids) == 0 {
		return platform.ErrUnsupported
	}
	if !slices.Contains(m.ids, mode) {
		return fmt.Errorf("ime: undeclared input mode %q", mode)
	}
	if m.current != mode {
		m.request++
		m.current, m.pending = mode, true
		withPool(func() { m.applyPending(imeClient(imeHost.active)) })
	}
	return nil
}

func (*backend) IMECurrentMode() string { return imeHost.modes.current }

func (m *imeModes) applyPending(client id) {
	if !m.pending || client == 0 {
		return
	}
	mode, request, session := m.current, m.request, imeHost.active
	before := ""
	if m.source.current != nil {
		before = m.source.current()
		m.lastSource = before
	}
	m.pending = false
	send(client, "selectInputMode:", uintptr(nsString(imeModeSourceID(m.bundleID, mode))))
	if before == "" {
		return
	}
	// Let the client finish the IMK event before using the TIS fallback.
	postMain(func() {
		if m.request == request && imeHost.active == session {
			m.selectInputSourceIfUnchanged(mode, before)
			imeObserveInputSource()
		}
	})
}

func (m *imeModes) selectInputSourceIfUnchanged(mode, before string) {
	if m.current != mode || !slices.Contains(m.ids, imeModeID(m.bundleID, before)) {
		return
	}
	want := imeModeSourceID(m.bundleID, mode)
	if before != want && m.source.current() == before {
		if err := m.source.selectMode(want); err != nil {
			log.Printf("tango: IME mode: %v", err)
		}
	}
}

func (m *imeModes) observeInputSource() string {
	if m.source.current == nil {
		return ""
	}
	sourceID := m.source.current()
	if sourceID == m.lastSource {
		return ""
	}
	m.lastSource = sourceID
	mode := imeModeID(m.bundleID, sourceID)
	if m.acceptSystemMode(mode) {
		return mode
	}
	return ""
}

func (m *imeModes) acceptSystemMode(mode string) bool {
	if mode == m.current || !slices.Contains(m.ids, mode) {
		return false
	}
	m.current, m.pending = mode, false
	return true
}

func imeObserveInputSource() {
	if mode := imeHost.modes.observeInputSource(); mode != "" {
		imeNotifyModeChanged(mode)
	}
}

func imeNotifyModeChanged(mode string) {
	if sink, ok := imeHost.sink.(platform.InputMethodModeSink); ok {
		sink.IMEModeChanged(mode)
	}
}

func imeSetValue(self id, cmd objc.SEL, value id, tag int, client id) {
	if tag != imeDocumentInputModeTag {
		self.SendSuper(cmd, value, tag, client)
		return
	}
	imeOnMain(func() {
		m := &imeHost.modes
		session := imeHost.sessions[self]
		// A new controller may report the system's mode before activation.
		// Previously deactivated controllers can still deliver stale values.
		if session == 0 || imeHost.inactive[session] ||
			imeHost.active != 0 && session != imeHost.active ||
			!sendBool(value, "isKindOfClass:", uintptr(class("NSString"))) {
			return
		}
		mode := imeModeID(m.bundleID, goString(value))
		if m.acceptSystemMode(mode) {
			imeNotifyModeChanged(mode)
		}
	})
}
