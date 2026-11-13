//go:build android

package android

import (
	"strconv"

	"github.com/tangoui/tango/internal/platform"
)

// Round-4 capabilities (mobile_ext.go) over Mobile.call, implemented in
// org.tango.host.Device: store page, settings, screen, status bar,
// keyboard, background jobs and the widget. Purchases are not implemented:
// Google Play Billing needs a library the host does not bundle, so the
// backend does not offer platform.Purchases and sys/purchase reports
// ErrUnsupported (docs/mobile.md describes how to wire Billing in).

var (
	_ platform.StoreReview      = (*Backend)(nil)
	_ platform.SettingsOpener   = (*Backend)(nil)
	_ platform.ScreenControl    = (*Backend)(nil)
	_ platform.StatusBarControl = (*Backend)(nil)
	_ platform.Keyboard         = (*Backend)(nil)
	_ platform.BackgroundTasks  = (*Backend)(nil)
	_ platform.Widgets          = (*Backend)(nil)
)

func call0(op string, arg any) error {
	_, err := mobileCall(0, op, arg)
	return err
}

func (b *Backend) RequestReview() error   { return call0("review", nil) }
func (b *Backend) OpenAppSettings() error { return call0("openSettings", nil) }

func (b *Backend) SetKeepScreenOn(on bool) error {
	return call0("keepScreenOn", map[string]bool{"on": on})
}

func (b *Backend) Brightness() (float64, error) {
	s, err := mobileCall(0, "brightness", nil)
	if err != nil {
		return 0, err
	}
	return parseFloat(s), nil
}

func (b *Backend) SetBrightness(v float64) error {
	return call0("setBrightness", map[string]float64{"value": v})
}

func (b *Backend) SetStatusBar(sb platform.StatusBar) error {
	return call0("statusBar", map[string]any{"style": int(sb.Style), "hidden": sb.Hidden, "color": sb.Color})
}

func (b *Backend) KeyboardHeight() float64 {
	s, err := mobileCall(0, "keyboardHeight", nil)
	if err != nil {
		return 0
	}
	return parseFloat(s)
}

func (b *Backend) HideKeyboard() error { return call0("hideKeyboard", nil) }

func (b *Backend) ScheduleBackground(r platform.BackgroundRequest) error {
	return call0("scheduleBackground", map[string]any{"kind": int(r.Kind), "earliestMs": r.Earliest.Milliseconds(),
		"network": r.RequiresNetwork, "charging": r.RequiresCharging})
}

func (b *Backend) CancelBackground(k platform.BackgroundKind) error {
	return call0("cancelBackground", map[string]int{"kind": int(k)})
}

func (b *Backend) TakeBackgroundTasks() []platform.BackgroundTask {
	s, err := mobileCall(0, "takeBackground", nil)
	if err != nil {
		return nil
	}
	return decodeBackground(s)
}

func (b *Backend) FinishBackgroundTask(id int64, success bool) {
	mobileCall(0, "finishBackground", map[string]any{"id": id, "success": success})
}

func (b *Backend) UpdateWidget(c platform.WidgetContent) error {
	return call0("updateWidget", map[string]string{"title": c.Title, "text": c.Text})
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
