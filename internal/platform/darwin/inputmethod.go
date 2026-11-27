//go:build darwin && !ios

package darwin

import (
	"errors"
	"fmt"
	"math"
	"runtime"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/tangoui/tango/internal/platform"
)

const imeControllerClass = "TangoInputController"

var imeRuntime = struct {
	sync.Once
	err         error
	mark        func(id, objc.SEL, id, nsRange, nsRange)
	insert      func(id, objc.SEL, id, nsRange)
	lineRect    func(id, objc.SEL, int, *nsRect) id
	menuItemKey id
}{}

// IMK state is owned by the main thread. The retained server lives until
// the process exits. The maps track controllers without retaining them.
var imeHost struct {
	server      id
	sink        platform.InputMethodSink
	nextSession uint64
	sessions    map[id]uint64
	controllers map[uint64]id
	inactive    map[uint64]bool // sessions deactivated since their last activation
	active      uint64
	modes       imeModes
}

var _ platform.InputMethods = (*backend)(nil)

func loadIME() error {
	imeRuntime.Do(func() {
		var l linker
		imk := l.open(frameworksDir + "InputMethodKit.framework/InputMethodKit")
		if imk != 0 {
			imeRuntime.menuItemKey = id(constPtr(imk, "kIMKCommandMenuItemName"))
		}
		if imeRuntime.err = errors.Join(l.failures...); imeRuntime.err != nil {
			return
		}
		_, imeRuntime.err = objc.RegisterClass(imeControllerClass, objc.GetClass("IMKInputController"), nil, nil, []objc.MethodDef{
			method("initWithServer:delegate:client:", imeInitController),
			method("handleEvent:client:", imeHandleEvent),
			method("recognizedEvents:", imeRecognizedEvents),
			method("mouseDownOnCharacterIndex:coordinate:withModifier:continueTracking:client:", imeMouseDown),
			method("activateServer:", imeActivate),
			method("deactivateServer:", imeDeactivate),
			method("commitComposition:", imeCommitComposition),
			method("setValue:forTag:client:", imeSetValue),
			method("menu", imeMenu),
			method("tangoMenuItem:", imeMenuItem),
			method("dealloc", imeDealloc),
		})
		if imeRuntime.err != nil {
			return
		}
		if runtime.GOARCH == "amd64" {
			imeRuntime.mark = imeMarkAMD64
		} else {
			purego.RegisterFunc(&imeRuntime.mark, msgSendAddr)
		}
		purego.RegisterFunc(&imeRuntime.insert, msgSendAddr)
		purego.RegisterFunc(&imeRuntime.lineRect, msgSendAddr)
	})
	return imeRuntime.err
}

// On x86-64, self, the selector, the string, and the selection consume five
// of the six integer argument registers. The entire replacement NSRange
// goes on the stack. Reserve the unused sixth register so purego cannot
// split that range across the last register and the stack.
func imeMarkAMD64(client id, op objc.SEL, text id, selection, replacement nsRange) {
	purego.SyscallN(msgSendAddr, uintptr(client), uintptr(op), uintptr(text),
		uintptr(selection.Location), uintptr(selection.Length), 0,
		uintptr(replacement.Location), uintptr(replacement.Length))
}

// imeConnectionName resolves the server name inside an autorelease pool.
func imeConnectionName(connection string, bundle id) string {
	if connection != "" {
		return connection
	}
	dict := send(bundle, "infoDictionary")
	value := send(dict, "objectForKey:", uintptr(nsString("InputMethodConnectionName")))
	if sendBool(value, "isKindOfClass:", uintptr(class("NSString"))) {
		if name := goString(value); name != "" {
			return name
		}
	}
	return goString(send(bundle, "bundleIdentifier")) + "_Connection"
}

func (*backend) ServeInputMethod(connection string, sink platform.InputMethodSink) error {
	if imeHost.sink != nil {
		return errors.New("darwin: input method already serving")
	}
	var err error
	withPool(func() {
		bundle := send(class("NSBundle"), "mainBundle")
		bundleID := send(bundle, "bundleIdentifier")
		if bundleID == 0 {
			err = errors.New("darwin: input method must run from an app bundle")
			return
		}
		if loadErr := loadIME(); loadErr != nil {
			err = fmt.Errorf("darwin: load input method: %w", loadErr)
			return
		}
		imeHost.sink = sink
		imeHost.sessions = make(map[id]uint64)
		imeHost.controllers = make(map[uint64]id)
		imeHost.inactive = make(map[uint64]bool)
		imeHost.modes = imeModesFromBundle(bundle, goString(bundleID))
		server := send(class("IMKServer"), "alloc")
		connection = imeConnectionName(connection, bundle)
		imeHost.server = send(server, "initWithName:bundleIdentifier:", uintptr(nsString(connection)), uintptr(bundleID))
		if imeHost.server == 0 {
			imeHost.sink = nil
			imeHost.sessions = nil
			imeHost.controllers = nil
			imeHost.inactive = nil
			imeHost.modes = imeModes{}
			err = fmt.Errorf("darwin: IMKServer initialization failed for %q", connection)
		}
	})
	return err
}

// InputMethodKit normally invokes the controller on the main thread. Wait
// for the main thread if it calls us elsewhere so events stay ordered, the
// objects stay live, and handleEvent can return the client's decision.
func imeOnMain(fn func()) {
	if isMainThread() {
		withPool(fn)
		return
	}
	done := make(chan struct{})
	postMain(func() {
		defer close(done)
		fn()
	})
	<-done
}

func imeInitController(self id, cmd objc.SEL, server, delegate, client id) id {
	controller := self.SendSuper(cmd, server, delegate, client)
	if controller != 0 {
		imeOnMain(func() {
			imeHost.nextSession++
			imeHost.sessions[controller] = imeHost.nextSession
			imeHost.controllers[imeHost.nextSession] = controller
		})
	}
	return controller
}

func imeRecognizedEvents(id, objc.SEL, id) uint {
	mask := uint(1 << nsEventTypeKeyDown)
	imeOnMain(func() {
		if sink, ok := imeHost.sink.(platform.InputMethodModifiers); ok && sink.IMEWantsModifiers() {
			mask = imeRecognizedEventsMask
		}
	})
	return mask
}

// IMK only provides its default click-to-commit behavior for a KeyDown-only
// event mask. When flags are requested, handle the equivalent mouse callback.
func imeMouseDown(self id, cmd objc.SEL, index uint, point nsPoint, flags uint, tracking *bool, client id) bool {
	var modifiers bool
	imeOnMain(func() {
		sink, ok := imeHost.sink.(platform.InputMethodModifiers)
		modifiers = ok && sink.IMEWantsModifiers()
		if !modifiers {
			return
		}
		session := imeHost.sessions[self]
		if session == 0 || !sendBool(client, "respondsToSelector:", uintptr(sel("markedRange"))) {
			return
		}
		marked := objc.Send[nsRange](client, sel("markedRange"))
		if marked.Location != nsNotFound && marked.Length != nsNotFound && marked.Length != 0 &&
			(index < marked.Location || index-marked.Location >= marked.Length) {
			imeHost.sink.IMECommitComposition(session)
		}
	})
	if !modifiers {
		return objc.SendSuper[bool](self, cmd, index, point, flags, tracking, client)
	}
	return false
}

func imeHandleEvent(self id, _ objc.SEL, event, _ id) bool {
	if event == 0 {
		return false
	}
	kind := send(event, "type")
	if kind != nsEventTypeKeyDown && kind != nsEventTypeFlagsChanged {
		return false
	}
	var handled bool
	imeOnMain(func() {
		session := imeHost.sessions[self]
		if session == 0 {
			return
		}
		code := uint16(send(event, "keyCode"))
		flags := uint(send(event, "modifierFlags"))
		if kind == nsEventTypeFlagsChanged {
			if sink, ok := imeHost.sink.(platform.InputMethodModifiers); ok {
				if mod, ok := imeModifierFromFlags(code, flags); ok {
					handled = sink.IMEModifier(session, mod)
				}
			}
			return
		}
		chars := goString(send(event, "characters"))
		unmodified := goString(send(event, "charactersIgnoringModifiers"))
		handled = imeHost.sink.IMEKey(session, imeKey(code, chars, unmodified, flags, sendBool(event, "isARepeat")))
	})
	return handled
}

func imeActivate(self id, cmd objc.SEL, client id) {
	self.SendSuper(cmd, client)
	imeOnMain(func() {
		if session := imeHost.sessions[self]; session != 0 {
			delete(imeHost.inactive, session)
			imeHost.active = session
			imeHost.modes.applyPending(imeClient(session))
			imeHost.sink.IMEActivate(session)
		}
	})
}

func imeDeactivate(self id, cmd objc.SEL, client id) {
	imeOnMain(func() {
		if session := imeHost.sessions[self]; session != 0 {
			imeHost.inactive[session] = true
			if imeHost.active == session {
				imeHost.active = 0
			}
			imeHost.sink.IMEDeactivate(session)
		}
	})
	self.SendSuper(cmd, client)
}

func imeCommitComposition(self id, _ objc.SEL, _ id) {
	imeOnMain(func() {
		if session := imeHost.sessions[self]; session != 0 {
			imeHost.sink.IMECommitComposition(session)
		}
	})
}

func imeDealloc(self id, cmd objc.SEL) {
	imeOnMain(func() {
		if session := imeHost.sessions[self]; session != 0 {
			delete(imeHost.sessions, self)
			delete(imeHost.controllers, session)
			delete(imeHost.inactive, session)
			if imeHost.active == session {
				imeHost.active = 0
			}
			imeHost.sink.IMEEnd(session)
		}
	})
	self.SendSuper(cmd)
}

func imeClient(session uint64) id {
	return sendIfLive(imeHost.controllers[session], "client")
}

func (*backend) IMESetComposition(session uint64, text string, cursor int) {
	withPool(func() {
		if client := imeClient(session); client != 0 {
			selection := nsRange{Location: uint(utf16Offset(text, cursor))}
			imeRuntime.mark(client, sel("setMarkedText:selectionRange:replacementRange:"),
				nsString(text), selection, nsRange{Location: nsNotFound})
		}
	})
}

func (*backend) IMECommit(session uint64, text string) {
	withPool(func() {
		if client := imeClient(session); client != 0 {
			imeRuntime.insert(client, sel("insertText:replacementRange:"), nsString(text), nsRange{Location: nsNotFound})
		}
	})
}

func (*backend) IMECaret(session uint64) (rect platform.Rect) {
	withPool(func() {
		if client := imeClient(session); client != 0 {
			var line nsRect
			imeRuntime.lineRect(client, sel("attributesForCharacterIndex:lineHeightRectangle:"), 0, &line)
			if line.Size.Height > 0 {
				x, y := topLeft(line.Origin.X, line.Origin.Y, line.Size.Height, primaryHeight())
				rect = platform.Rect{X: x, Y: y, Width: int(math.Round(line.Size.Width)), Height: int(math.Round(line.Size.Height))}
			}
		}
	})
	return rect
}

func (*backend) IMEClientApp(session uint64) (bundleID string) {
	withPool(func() {
		bundleID = goString(sendIfLive(imeClient(session), "bundleIdentifier"))
	})
	return bundleID
}
