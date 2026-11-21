//go:build darwin && !ios

package darwin

import (
	"math"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/tangoui/tango/internal/platform"
)

func (e *embedded) EvalWebJSON(expression string, done func([]byte, error)) {
	withPool(func() {
		block := objc.NewBlock(func(_ objc.Block, result, err id) {
			if err != 0 {
				done(nil, nsError(err, "script evaluation failed"))
				return
			}
			done([]byte(goString(result)), nil)
		})
		defer block.Release()
		send(e.s.web, "evaluateJavaScript:completionHandler:", uintptr(nsString(platform.WebJSONExpression(expression))), uintptr(block))
	})
}

var webInputNative struct {
	once       sync.Once
	mouse      func(id, objc.SEL, uint, nsPoint, uint, float64, int, id, int, int, float32) id
	insertText func(id, objc.SEL, id, nsRange)
	scroll     func(uintptr, uint32, uint32, int32, int32, int32) uintptr
	locate     func(uintptr, nsPoint)
}

func bindWebInputNative() {
	f := &webInputNative
	purego.RegisterFunc(&f.mouse, msgSendAddr)
	purego.RegisterFunc(&f.insertText, msgSendAddr)
	cg := mustDlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics")
	if fn, err := purego.Dlsym(cg, "CGEventCreateScrollWheelEvent2"); err == nil {
		purego.RegisterFunc(&f.scroll, fn)
		purego.RegisterLibFunc(&f.locate, cg, "CGEventSetLocation")
	}
}

func (e *embedded) DispatchWebInput(in platform.WebInput, done func(error)) {
	f := &webInputNative
	f.once.Do(bindWebInputNative)
	var err error
	withPool(func() {
		web := e.s.web
		win := send(web, "window")
		if win == 0 {
			err = platform.ErrUnsupported
			return
		}
		p := nsPoint{X: in.X, Y: in.Y}
		if !sendBool(web, "isFlipped") {
			p.Y = msgRect(web, sel("bounds")).Size.Height - p.Y
		}
		p = msgConvertPoint(web, sel("convertPoint:toView:"), p, 0)
		switch in.Kind {
		case platform.WebClick:
			e.FocusWeb()
			for i, selector := range []string{"mouseDown:", "mouseUp:"} {
				ev := f.mouse(class("NSEvent"), sel("mouseEventWithType:location:modifierFlags:timestamp:windowNumber:context:eventNumber:clickCount:pressure:"),
					uint(i+1), p, 0, 0, sendInt(win, "windowNumber"), 0, 0, 1, 1)
				if ev == 0 {
					err = platform.ErrUnsupported
					return
				}
				send(web, selector, uintptr(ev))
			}
		case platform.WebType:
			e.FocusWeb()
			target := send(win, "firstResponder")
			if !respondsTo(target, "insertText:replacementRange:") {
				err = platform.ErrUnsupported
				return
			}
			f.insertText(target, sel("insertText:replacementRange:"), nsString(in.Text), nsRange{nsNotFound, 0})
		case platform.WebScroll:
			if f.scroll == nil {
				err = platform.ErrUnsupported
				return
			}
			ev := f.scroll(0, 0, 2, -int32(math.Round(in.DeltaY)), -int32(math.Round(in.DeltaX)), 0)
			if ev == 0 {
				err = platform.ErrUnsupported
				return
			}
			defer cfRelease(ev)
			f.locate(ev, p)
			send(web, "scrollWheel:", uintptr(send(class("NSEvent"), "eventWithCGEvent:", ev)))
		}
	})
	done(err)
}
