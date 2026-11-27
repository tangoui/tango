//go:build darwin && !ios

package darwin

import (
	"fmt"
	"slices"

	"github.com/ebitengine/purego/objc"
	"github.com/tangoui/tango/internal/platform"
)

const imeDocumentInputModeTag = 0x696d696d // kTSMDocumentInputModePropertyTag, 'imim'

type imeModes struct {
	bundleID string
	ids      []string
	current  string
	pending  bool // an application selection awaiting an active client
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
		m.current, m.pending = mode, true
		withPool(func() { m.applyPending(imeClient(imeHost.active)) })
	}
	return nil
}

func (*backend) IMECurrentMode() string { return imeHost.modes.current }

func (m *imeModes) applyPending(client id) {
	if m.pending && client != 0 {
		m.pending = false
		send(client, "selectInputMode:", uintptr(nsString(imeModeSourceID(m.bundleID, m.current))))
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
		if session == 0 || imeHost.active != 0 && session != imeHost.active ||
			!sendBool(value, "isKindOfClass:", uintptr(class("NSString"))) {
			return
		}
		mode := imeModeID(m.bundleID, goString(value))
		if mode == m.current || !slices.Contains(m.ids, mode) {
			return
		}
		m.current, m.pending = mode, false
		if sink, ok := imeHost.sink.(platform.InputMethodModeSink); ok {
			sink.IMEModeChanged(mode)
		}
	})
}
