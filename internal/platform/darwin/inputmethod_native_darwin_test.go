//go:build darwin && !ios

package darwin

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"testing"

	"github.com/ebitengine/purego/objc"
	"github.com/tangoui/tango/internal/platform"
)

const imeControllerProbe = "TANGO_TEST_IME_NATIVE_CONTROLLER"

func init() {
	if os.Getenv(imeControllerProbe) != "1" {
		return
	}
	runtime.LockOSThread()
	previous := MainThreadTestHook
	MainThreadTestHook = func() (int, bool) {
		if previous != nil {
			if code, handled := previous(); handled {
				return code, true
			}
		}
		runNativeIMEController()
		return 0, true
	}
}

func TestIMENativeController(t *testing.T) {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), imeControllerProbe+"=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("native IME controller: %v\n%s", err, out)
	}
}

type imeControllerProbeSink struct {
	platform.InputMethodSink
	clicked []string
	modes   []string
	keys    []platform.IMEKey
	mods    []platform.IMEModifierKey
}

func (s *imeControllerProbeSink) IMEKey(_ uint64, k platform.IMEKey) bool {
	s.keys = append(s.keys, k)
	return false
}

func (*imeControllerProbeSink) IMEEnd(uint64) {}

func (s *imeControllerProbeSink) IMEModifier(_ uint64, m platform.IMEModifierKey) bool {
	s.mods = append(s.mods, m)
	return m.Code == "ShiftLeft" && !m.Down
}

func (*imeControllerProbeSink) IMEMenu() []platform.IMEMenuItem {
	return []platform.IMEMenuItem{
		{ID: "checked", Title: "Checked", Checked: true, Disabled: true},
		{Separator: true},
		{ID: "about", Title: "About"},
	}
}

func (s *imeControllerProbeSink) IMEMenuClicked(id string) { s.clicked = append(s.clicked, id) }
func (s *imeControllerProbeSink) IMEModeChanged(id string) { s.modes = append(s.modes, id) }

func runNativeIMEController() {
	const bundle = "com.example.inputmethod.probe"
	withPool(func() {
		if err := loadIME(); err != nil {
			panic(err)
		}
		send(class("NSApplication"), "sharedApplication")
		sink := &imeControllerProbeSink{}
		imeHost.sink = sink
		host := &backend{}
		controller := send(class(imeControllerClass), "alloc")
		defer release(controller)
		imeHost.sessions = map[id]uint64{controller: 1}
		imeHost.controllers = map[uint64]id{1: controller}
		imeHost.modes = imeModes{bundleID: bundle, ids: []string{"zh", "en"}, current: "zh"}
		menu := send(controller, "menu")
		itemAt := func(i uintptr) id { return send(menu, "itemAtIndex:", i) }
		if count := send(menu, "numberOfItems"); count != 3 {
			panic(fmt.Sprintf("menu contains %d items", count))
		}
		if !sendBool(itemAt(1), "isSeparatorItem") ||
			sendBool(itemAt(0), "isEnabled") || send(itemAt(0), "state") != 1 ||
			goString(send(itemAt(2), "title")) != "About" ||
			send(itemAt(2), "action") != id(sel("tangoMenuItem:")) ||
			send(itemAt(2), "target") != controller {
			panic("native menu item state")
		}
		dict := send(class("NSDictionary"), "dictionaryWithObject:forKey:", uintptr(itemAt(2)), uintptr(imeRuntime.menuItemKey))
		send(controller, "doCommandBySelector:commandDictionary:", uintptr(sel("tangoMenuItem:")), uintptr(dict))
		if !slices.Equal(sink.clicked, []string{"about"}) {
			panic(fmt.Sprintf("menu clicks = %v", sink.clicked))
		}
		if got := send(controller, "recognizedEvents:", 0); got != imeRecognizedEventsMask {
			panic(fmt.Sprintf("recognized events = %#x", got))
		}
		makeEvent := func(kind, flags uintptr, code uint16, repeat bool) id {
			// Exercise the controller with event values in process.
			imeProbeEvent = struct {
				kind, flags uintptr
				code        uint16
				repeat      bool
			}{kind, flags, code, repeat}
			return alloc("TangoIMEProbeEvent")
		}
		classDef("TangoIMEProbeEvent", "NSObject", nil, []objc.MethodDef{
			method("type", func(id, objc.SEL) uint { return uint(imeProbeEvent.kind) }),
			method("modifierFlags", func(id, objc.SEL) uint { return uint(imeProbeEvent.flags) }),
			method("keyCode", func(id, objc.SEL) uint16 { return imeProbeEvent.code }),
			method("isARepeat", func(id, objc.SEL) bool { return imeProbeEvent.repeat }),
			method("characters", func(id, objc.SEL) id { return nsString("A") }),
			method("charactersIgnoringModifiers", func(id, objc.SEL) id { return nsString("a") }),
		})
		for _, tc := range []struct {
			typeID, flags uintptr
			code          uint16
			repeat, eat   bool
		}{
			{12, 1 << 20, 0x37, false, false},
			{10, 1<<20 | 1<<16, 0, true, false},
			{12, 0, 0x37, false, false},
			{12, 1 << 17, 0x38, false, false},
			{12, 0, 0x38, false, true},
		} {
			event := makeEvent(tc.typeID, tc.flags, tc.code, tc.repeat)
			handled := sendBool(controller, "handleEvent:client:", uintptr(event), 0)
			release(event)
			if handled != tc.eat {
				panic(fmt.Sprintf("event %+v was handled = %t", tc, handled))
			}
		}
		if len(sink.mods) != 4 || len(sink.keys) != 1 || !sink.keys[0].Repeat || !sink.keys[0].CapsLock || !sink.keys[0].Meta {
			panic(fmt.Sprintf("modifiers = %v, keys = %v", sink.mods, sink.keys))
		}
		if err := host.IMESetMode("unknown"); err == nil {
			panic("unknown mode accepted")
		}
		const modeTag = 0x696d696d
		systemMode := func(id string) {
			send(controller, "setValue:forTag:client:", uintptr(nsString(bundle+"."+id)), modeTag, 0)
		}
		systemMode("en")
		systemMode("en")
		if got := host.IMECurrentMode(); got != "en" || !slices.Equal(sink.modes, []string{"en"}) {
			panic(fmt.Sprintf("mode = %q, callbacks = %v", got, sink.modes))
		}
		if err := host.IMESetMode("zh"); err != nil {
			panic(err)
		}
		systemMode("zh")
		if got := host.IMECurrentMode(); got != "zh" || !slices.Equal(sink.modes, []string{"en"}) {
			panic(fmt.Sprintf("mode = %q, callbacks = %v", got, sink.modes))
		}
	})
}

var imeProbeEvent struct {
	kind, flags uintptr
	code        uint16
	repeat      bool
}
