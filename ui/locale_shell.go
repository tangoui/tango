package ui

import "time"

// Texts of the workbench shell components (StatusBar, TitleBar,
// NotificationCenter, HotkeyInput). The packs are in locale_packs.go.

// StatusBarLocale holds the StatusBar texts.
type StatusBarLocale struct {
	_ [0]func()

	Label string // accessible name of the bar ("状态栏")
}

// TitleBarLocale holds the window button names of TitleBar and
// WindowControls (Close is Locale.Close, the full-screen pair
// Locale.Fullscreen / Locale.ExitFullscreen).
type TitleBarLocale struct {
	_ [0]func()

	Minimize, Maximize, Restore string
	WindowControls              string // accessible name of the button group
}

// NotificationCenterLocale holds the NotificationCenter texts. The trigger
// and panel are named by Locale.A11y.Notifications.
type NotificationCenterLocale struct {
	_ [0]func()

	MarkAllRead, ClearAll, Dismiss         string
	Empty                                  string // empty panel ("暂无通知")
	UnreadFmt                              string // unread count, arg: n ("%d 条未读")
	JustNow                                string // under a minute ago
	MinutesAgoFmt, HoursAgoFmt, DaysAgoFmt string // arg: n ("%d 分钟前")
}

// HotkeyInputLocale holds the HotkeyInput texts.
type HotkeyInputLocale struct {
	_ [0]func()

	Placeholder string // empty, not recording ("点击录制快捷键")
	Recording   string // waiting for a key ("请按下组合键，Esc 取消")
	ClearLabel  string // clear button ("清除快捷键")
}

// Unread formats the unread count of a NotificationCenter ("3 条未读").
func (l *NotificationCenterLocale) Unread(n int) string { return locFmt(l.UnreadFmt, n) }

// notifyAgo formats the time of a notification relative to now: JustNow under a
// minute, minutes, hours, then days up to a week; older times as the full
// date of the parent Locale (A11y.DateFmt).
func (l *Locale) notifyAgo(t, now time.Time) string {
	n := &l.NotificationCenter
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return n.JustNow
	case d < time.Hour:
		return locFmt(n.MinutesAgoFmt, int(d/time.Minute))
	case d < 24*time.Hour:
		return locFmt(n.HoursAgoFmt, int(d/time.Hour))
	case d < 7*24*time.Hour:
		return locFmt(n.DaysAgoFmt, int(d/(24*time.Hour)))
	}
	y, m, day := t.Date()
	return locFmt(l.A11y.DateFmt, y, int(m), day, l.Months[m-1])
}
