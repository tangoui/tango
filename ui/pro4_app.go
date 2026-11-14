package ui

import (
	"time"

	"github.com/tangoui/tango/ui/layout"
)

// ---------------------------------------------------------------------------
// App (Ant Design v6 App / App.useApp)
//
//	ui.App(c).Message(ui.AppMessageConfig{Duration: 2 * time.Second, MaxCount: 3}).
//		Notification(ui.AppNotificationConfig{Placement: ui.NotifyBottomRight}).
//		Body(func() {
//			app := ui.UseApp(c)
//			if ui.Button(c, "Save").Clicked() {
//				app.Message("success", "Saved")
//			}
//		})
//
// In Ant, App provides the context-aware message / notification / modal
// instances (App.useApp) and resets the base text styles of its subtree.
// Tango's Message, Notification and Confirm are already plain calls that
// work anywhere and read ConfigProvider.Defaults, so App is the unified
// configuration container: its Message / Notification settings become the
// "Message" / "Notification" defaults of everything built inside, and
// UseApp returns handles that apply them (including MaxCount, which the
// plain calls do not know). Text already takes the theme's color and font
// everywhere, so there is no style reset to do; App only adds a column
// that fills its parent.

// AppMessageConfig configures the messages opened inside an App (Ant
// App message prop).
type AppMessageConfig struct {
	_ [0]func()

	Duration time.Duration // default display time (0: MessageDuration)
	MaxCount int           // most messages on screen; the oldest close first (0: no limit)
}

// AppNotificationConfig configures the notifications opened inside an App
// (Ant App notification prop).
type AppNotificationConfig struct {
	_ [0]func()

	Placement    NotifyPlacement
	PlacementSet bool          // Placement given (NotifyTopEnd is the zero value)
	Duration     time.Duration // 0: the default
	MaxCount     int           // most cards on screen (0: no limit)
	ShowProgress bool
}

// AppW builds an App.
type AppW struct {
	_ [0]func()

	c    *Context
	msg  AppMessageConfig
	note AppNotificationConfig
	opts []layout.Option
}

type pro4AppStack struct{ frames []AppAPI }

var pro4AppKey = hashID("pro4-app-stack")

// App starts an application root; chain options, then Body.
//
// ConfigProvider.Defaults("App", ...) keys: "messageMaxCount",
// "notificationMaxCount" (number).
func App(c *Context) *AppW {
	a := &AppW{c: c}
	if n, ok := proPropInt(c, "App", "messageMaxCount"); ok {
		a.msg.MaxCount = n
	}
	if n, ok := proPropInt(c, "App", "notificationMaxCount"); ok {
		a.note.MaxCount = n
	}
	return a
}

// Message sets the message configuration.
func (a *AppW) Message(cfg AppMessageConfig) *AppW { a.msg = cfg; return a }

// Notification sets the notification configuration.
func (a *AppW) Notification(cfg AppNotificationConfig) *AppW { a.note = cfg; return a }

// Layout applies layout options to the App column.
func (a *AppW) Layout(o ...layout.Option) *AppW { a.opts = append(a.opts, o...); return a }

// Body builds fn inside the App.
func (a *AppW) Body(fn func()) *Box {
	c := a.c
	cp := ConfigProvider(c)
	if a.msg.Duration > 0 {
		cp.Defaults("Message", Props{"duration": a.msg.Duration})
	}
	np := Props{}
	if a.note.PlacementSet {
		np["placement"] = a.note.Placement
	}
	if a.note.Duration > 0 {
		np["duration"] = a.note.Duration
	}
	if a.note.ShowProgress {
		np["showProgress"] = true
	}
	if len(np) > 0 {
		cp.Defaults("Notification", np)
	}
	var box *Box
	cp.Body(func() {
		st := State[pro4AppStack](c, pro4AppKey, nil)
		st.frames = append(st.frames, AppAPI{c: c, msg: a.msg, note: a.note, ok: true})
		depth := len(st.frames)
		box = Column(c, fn, append([]layout.Option{layout.Grow(1), layout.MinW(0)}, a.opts...)...)
		st = State[pro4AppStack](c, pro4AppKey, nil)
		if len(st.frames) >= depth {
			st.frames = st.frames[:depth-1]
		}
	})
	return box
}

// AppAPI opens messages, notifications and confirm boxes with the
// configuration of the enclosing App (Ant App.useApp()).
type AppAPI struct {
	_ [0]func()

	c    *Context
	msg  AppMessageConfig
	note AppNotificationConfig
	ok   bool
}

// UseApp returns the API of the innermost App being built (outside an
// App it opens plain messages with the defaults).
func UseApp(c *Context) AppAPI {
	st := State[pro4AppStack](c, pro4AppKey, nil)
	if n := len(st.frames); n > 0 {
		api := st.frames[n-1]
		api.c = c
		return api
	}
	return AppAPI{c: c}
}

// InApp reports whether the API comes from an enclosing App.
func (a AppAPI) InApp() bool { return a.ok }

// Message opens a message (see Message), closing the oldest ones beyond
// MaxCount.
func (a AppAPI) Message(status, content string) *MessageRef {
	r := Message(a.c, status, content)
	if a.msg.Duration > 0 {
		r.Duration(a.msg.Duration)
	}
	if n := a.msg.MaxCount; n > 0 {
		t := a.c.toasts()
		if len(t.msgs) > n {
			t.msgs = append([]*toast(nil), t.msgs[len(t.msgs)-n:]...)
		}
	}
	return r
}

// Notification opens a notification (see Notification), closing the
// oldest cards beyond MaxCount.
func (a AppAPI) Notification(status, title string) *NotificationRef {
	r := Notification(a.c, status, title)
	if a.note.PlacementSet {
		r.Placement(a.note.Placement)
	}
	if a.note.Duration > 0 {
		r.Duration(a.note.Duration)
	}
	if a.note.ShowProgress {
		r.ShowProgress()
	}
	if n := a.note.MaxCount; n > 0 {
		ns := a.c.navNotes()
		if len(ns.list) > n {
			ns.list = append([]*navNote(nil), ns.list[len(ns.list)-n:]...)
		}
	}
	return r
}

// Confirm opens a confirm box (see Confirm; Ant modal.confirm).
func (a AppAPI) Confirm(o ConfirmOptions) *ConfirmRef { return Confirm(a.c, o) }

// Info opens an alert box (see MessageBox; Ant modal.info).
func (a AppAPI) Info(o ConfirmOptions) *ConfirmRef { return MessageBox(a.c, o) }
