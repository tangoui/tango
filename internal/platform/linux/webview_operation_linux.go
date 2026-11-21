//go:build linux && !android

package linux

import (
	"errors"
	"sync"

	"github.com/tangoui/tango/internal/platform"
)

var webOperations struct {
	once                  sync.Once
	evalFinish, runFinish func(ptr, ptr, *ptr) ptr
	runUnref              func(ptr)

	inputOK                    bool
	widgetWindow               func(ptr) ptr
	widgetEvent                func(ptr, ptr) bool
	eventNew                   func(int32) ptr
	eventFree                  func(ptr)
	eventDevice                func(ptr, ptr)
	windowOrigin               func(ptr, *int32, *int32) int32
	windowDisplay, displaySeat func(ptr) ptr
	seatPointer, seatKeyboard  func(ptr) ptr
	unicodeKey                 func(uint32) uint32
	strdup                     func(string) ptr
}

func bindWebOperations() {
	var bd binder
	f := &webOperations
	bd.opt(&f.evalFinish, "webkit_web_view_evaluate_javascript_finish", libWebKit)
	bd.opt(&f.runFinish, "webkit_web_view_run_javascript_finish", libWebKit)
	bd.opt(&f.runUnref, "webkit_javascript_result_unref", libWebKit)
	g, t := libGDK, libGTK
	f.inputOK = bd.opt(&f.widgetWindow, "gtk_widget_get_window", t) &&
		bd.opt(&f.widgetEvent, "gtk_widget_event", t) &&
		bd.opt(&f.eventNew, "gdk_event_new", g, t) &&
		bd.opt(&f.eventFree, "gdk_event_free", g, t) &&
		bd.opt(&f.eventDevice, "gdk_event_set_device", g, t) &&
		bd.opt(&f.windowOrigin, "gdk_window_get_origin", g, t) &&
		bd.opt(&f.windowDisplay, "gdk_window_get_display", g, t) &&
		bd.opt(&f.displaySeat, "gdk_display_get_default_seat", g, t) &&
		bd.opt(&f.seatPointer, "gdk_seat_get_pointer", g, t) &&
		bd.opt(&f.seatKeyboard, "gdk_seat_get_keyboard", g, t) &&
		bd.opt(&f.unicodeKey, "gdk_unicode_to_keyval", g, t) &&
		bd.opt(&f.strdup, "g_strdup", libGLib)
}

func (e *embeddedWeb) EvalWebJSON(expression string, done func([]byte, error)) {
	f := &webOperations
	f.once.Do(bindWebOperations)
	web := e.w.web
	js := platform.WebJSONExpression(expression)
	consume := func(value, gerr ptr) {
		if err := finishErr(value != 0, gerr, "script evaluation"); err != nil {
			done(nil, err)
			return
		}
		done([]byte(takeStr(jscValueToString(value))), nil)
	}
	switch {
	case webkitWebViewEvaluateJavascript != nil && f.evalFinish != nil:
		webkitWebViewEvaluateJavascript(web, js, len(js), nil, nil, 0, cbArg3, asyncData(func(res ptr) {
			var gerr ptr
			value := f.evalFinish(web, res, &gerr)
			if value != 0 {
				defer gObjectUnref(value)
			}
			consume(value, gerr)
		}))
	case webkitWebViewRunJavascript != nil && f.runFinish != nil && f.runUnref != nil:
		webkitWebViewRunJavascript(web, js, 0, cbArg3, asyncData(func(res ptr) {
			var gerr ptr
			result := f.runFinish(web, res, &gerr)
			var value ptr
			if result != 0 {
				defer f.runUnref(result)
				value = webkitJavascriptResultGetJSValue(result)
			}
			consume(value, gerr)
		}))
	default:
		done(nil, platform.ErrUnsupported)
	}
}

// These structures mirror GTK 3's GdkEvent variants on the supported LP64
// hosts. eventNew/eventFree own any window or string set on the event.
type webGDKEvent struct {
	typ    int32
	window ptr
	send   int8
	time   uint32
}

type webGDKButton struct {
	webGDKEvent
	x, y          float64
	axes          ptr
	state, button uint32
	device        ptr
	rootX, rootY  float64
}

type webGDKScroll struct {
	webGDKEvent
	x, y           float64
	state          uint32
	direction      int32
	device         ptr
	rootX, rootY   float64
	deltaX, deltaY float64
	stop           uint32
}

type webGDKKey struct {
	webGDKEvent
	state, keyval uint32
	length        int32
	text          ptr
	keycode       uint16
	group         uint8
	modifier      uint8
}

func (e *embeddedWeb) DispatchWebInput(in platform.WebInput, done func(error)) {
	f := &webOperations
	f.once.Do(bindWebOperations)
	if !f.inputOK {
		done(platform.ErrUnsupported)
		return
	}
	w := e.w.web
	win := f.widgetWindow(w)
	if win == 0 {
		done(platform.ErrUnsupported)
		return
	}
	seat := f.displaySeat(f.windowDisplay(win))
	if seat == 0 {
		done(platform.ErrUnsupported)
		return
	}
	device := f.seatPointer(seat)
	if in.Kind == platform.WebType {
		device = f.seatKeyboard(seat)
	}
	if device == 0 {
		done(platform.ErrUnsupported)
		return
	}
	var screenX, screenY int32
	f.windowOrigin(win, &screenX, &screenY)
	rootX, rootY := float64(screenX)+in.X, float64(screenY)+in.Y
	sendEvent := func(kind int32, configure func(ptr)) error {
		event := f.eventNew(kind)
		if event == 0 {
			return errors.New("tango/linux: could not create WebView input event")
		}
		defer f.eventFree(event)
		base := (*webGDKEvent)(toPtr(event))
		base.window, base.send = wv.objectRef(win), 1
		configure(event)
		f.eventDevice(event, device)
		f.widgetEvent(w, event)
		return nil
	}
	switch in.Kind {
	case platform.WebClick:
		e.FocusWeb()
		for _, kind := range []int32{4, 7} { // GDK_BUTTON_PRESS, GDK_BUTTON_RELEASE
			err := sendEvent(kind, func(event ptr) {
				b := (*webGDKButton)(toPtr(event))
				b.x, b.y, b.rootX, b.rootY, b.button = in.X, in.Y, rootX, rootY, 1
				if kind == 7 {
					b.state = 1 << 8 // GDK_BUTTON1_MASK: pre-release button state
				}
			})
			if err != nil {
				done(err)
				return
			}
		}
	case platform.WebScroll:
		done(sendEvent(31, func(event ptr) { // GDK_SCROLL
			s := (*webGDKScroll)(toPtr(event))
			s.x, s.y, s.rootX, s.rootY = in.X, in.Y, rootX, rootY
			s.direction = 4 // GDK_SCROLL_SMOOTH
			// WebKitGTK converts smooth scroll ticks into CSS pixels.
			s.deltaX, s.deltaY = in.DeltaX/40, in.DeltaY/40
		}))
		return
	case platform.WebType:
		e.FocusWeb()
		for _, ch := range in.Text {
			for _, kind := range []int32{8, 9} { // GDK_KEY_PRESS, GDK_KEY_RELEASE
				err := sendEvent(kind, func(event ptr) {
					key := (*webGDKKey)(toPtr(event))
					text := string(ch)
					key.keyval, key.length, key.text = f.unicodeKey(uint32(ch)), int32(len(text)), f.strdup(text)
				})
				if err != nil {
					done(err)
					return
				}
			}
		}
	}
	done(nil)
}
