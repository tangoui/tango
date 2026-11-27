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
	err      error
	mark     func(id, objc.SEL, id, nsRange, nsRange)
	insert   func(id, objc.SEL, id, nsRange)
	lineRect func(id, objc.SEL, int, *nsRect) id
}{}

// IMK state is owned by the main thread. The retained server lives until
// the process exits. The maps track controllers without retaining them.
var imeHost struct {
	server      id
	sink        platform.InputMethodSink
	nextSession uint64
	sessions    map[id]uint64
	controllers map[uint64]id
}

var _ platform.InputMethods = (*backend)(nil)

func loadIME() error {
	imeRuntime.Do(func() {
		var l linker
		l.open(frameworksDir + "InputMethodKit.framework/InputMethodKit")
		if imeRuntime.err = errors.Join(l.failures...); imeRuntime.err != nil {
			return
		}
		_, imeRuntime.err = objc.RegisterClass(imeControllerClass, objc.GetClass("IMKInputController"), nil, nil, []objc.MethodDef{
			method("initWithServer:delegate:client:", imeInitController),
			method("handleEvent:client:", imeHandleEvent),
			method("activateServer:", imeActivate),
			method("deactivateServer:", imeDeactivate),
			method("commitComposition:", imeCommitComposition),
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
		server := send(class("IMKServer"), "alloc")
		imeHost.server = send(server, "initWithName:bundleIdentifier:", uintptr(nsString(connection)), uintptr(bundleID))
		if imeHost.server == 0 {
			imeHost.sink = nil
			imeHost.sessions = nil
			imeHost.controllers = nil
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

func imeHandleEvent(self id, _ objc.SEL, event, _ id) bool {
	const nsEventTypeKeyDown = 10
	if event == 0 || send(event, "type") != nsEventTypeKeyDown {
		return false
	}
	var handled bool
	imeOnMain(func() {
		if session := imeHost.sessions[self]; session != 0 {
			code := uint16(send(event, "keyCode"))
			chars := goString(send(event, "characters"))
			unmodified := goString(send(event, "charactersIgnoringModifiers"))
			flags := uint(send(event, "modifierFlags"))
			handled = imeHost.sink.IMEKey(session, imeKey(code, chars, unmodified, flags))
		}
	})
	return handled
}

func imeActivate(self id, cmd objc.SEL, client id) {
	self.SendSuper(cmd, client)
	imeOnMain(func() {
		if session := imeHost.sessions[self]; session != 0 {
			imeHost.sink.IMEActivate(session)
		}
	})
}

func imeDeactivate(self id, cmd objc.SEL, client id) {
	imeOnMain(func() {
		if session := imeHost.sessions[self]; session != 0 {
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
