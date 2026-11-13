package uihost_test

import (
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/i18n"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/theme"
	"github.com/tangoui/tango/uihost"
)

// seen is what the page saw in its last build. Main thread.
type seen struct {
	th     *theme.Theme
	locale *ui.Locale
	text   string // the button label, a built-in text
	builds int
}

func (s *seen) page(c *ui.Context) {
	s.th, s.locale = c.Theme, c.Locale()
	s.builds++
	s.text = c.Locale().Cancel
	ui.Button(c, s.text)
}

// openWith opens a window with opts on the main thread and runs its first
// frame.
func openWith(t *testing.T, a *tango.App, fb *fake.Backend, opts uihost.Options, ropts ui.RunnerOptions, page func(*ui.Context)) (*ui.Runner, *tango.Surface, *fake.Canvas) {
	t.Helper()
	opts.Surface = tango.SurfaceOptions{Title: "UI", Width: 200, Height: 100}
	o := tango.Await(a, func() opened {
		r, s, err := uihost.Open(a, opts, page, ropts)
		return opened{r, s, err}
	})
	if o.err != nil {
		t.Fatal(o.err)
	}
	cv := fb.Canvas(o.s.ID())
	cv.FlushFrame()
	return o.r, o.s, cv
}

// look returns what the page saw, after running pending frames.
func look(fb *fake.Backend, cv *fake.Canvas, s *seen) seen {
	cv.FlushFrame()
	var v seen
	fb.Do(func() { v = *s })
	return v
}

// following runs FollowingSystemTheme on the main thread.
func following(a *tango.App, r *ui.Runner) bool {
	return tango.Await(a, func() bool { return uihost.FollowingSystemTheme(r) })
}

// resetLocale restores the global locales after a test.
func resetLocale(t *testing.T) {
	t.Cleanup(func() {
		i18n.SetLocale("")
		ui.SetDefaultLocale(ui.ZhCN)
	})
}

func TestFollowThemeByDefault(t *testing.T) {
	resetLocale(t)
	a, fb := faketest.Run(t, tango.Options{})
	var s seen
	r, _, cv := openWith(t, a, fb, uihost.Options{}, ui.RunnerOptions{}, s.page)
	v := look(fb, cv, &s)
	if v.th == nil || v.th.Dark || v.th.BgContainer != theme.AntDesign().BgContainer {
		t.Fatalf("light frame used %+v", v.th)
	}
	fb.SetTheme(platform.Theme{Dark: true})
	v = look(fb, cv, &s)
	dark := theme.AntDesign(theme.Dark())
	if !v.th.Dark || v.th.BgContainer != dark.BgContainer || v.th.Text != dark.Text {
		t.Fatal("frame after the switch does not use dark tokens")
	}
	if !following(a, r) {
		t.Fatal("not following")
	}
	fb.SetTheme(platform.Theme{})
	if v = look(fb, cv, &s); v.th.Dark {
		t.Fatal("did not switch back to light")
	}
}

func TestThemeFixed(t *testing.T) {
	resetLocale(t)
	a, fb := faketest.Run(t, tango.Options{})
	for _, opts := range []uihost.Options{{Theme: uihost.ThemeFixed}, {}} { // {}: explicit RunnerOptions.Theme
		fb.SetTheme(platform.Theme{})
		fixed := theme.ElementPlus()
		var s seen
		r, _, cv := openWith(t, a, fb, opts, ui.RunnerOptions{Theme: fixed}, s.page)
		fb.SetTheme(platform.Theme{Dark: true})
		if v := look(fb, cv, &s); v.th != fixed {
			t.Fatalf("%+v: theme replaced", opts)
		}
		if following(a, r) {
			t.Fatal("fixed window follows")
		}
	}
	// ThemeDark ignores the system's light appearance.
	fb.SetTheme(platform.Theme{})
	var s seen
	_, _, cv := openWith(t, a, fb, uihost.Options{Theme: uihost.ThemeDark}, ui.RunnerOptions{}, s.page)
	if v := look(fb, cv, &s); !v.th.Dark {
		t.Fatal("ThemeDark drew light")
	}
}

func TestManualThemeStopsFollowing(t *testing.T) {
	resetLocale(t)
	a, fb := faketest.Run(t, tango.Options{})

	// The app sets its own theme: the next system change leaves it alone.
	var s seen
	r, _, cv := openWith(t, a, fb, uihost.Options{}, ui.RunnerOptions{}, s.page)
	mine := theme.ElementPlus()
	fb.Do(func() { r.SetTheme(mine) })
	fb.SetTheme(platform.Theme{Dark: true})
	if v := look(fb, cv, &s); v.th != mine {
		t.Fatal("manual theme overwritten")
	}
	if following(a, r) {
		t.Fatal("still following after SetTheme")
	}

	// FollowSystemTheme(false), then true again.
	fb.SetTheme(platform.Theme{})
	var s2 seen
	r2, _, cv2 := openWith(t, a, fb, uihost.Options{}, ui.RunnerOptions{}, s2.page)
	fb.Do(func() { uihost.FollowSystemTheme(r2, false) })
	fb.SetTheme(platform.Theme{Dark: true})
	if v := look(fb, cv2, &s2); v.th.Dark {
		t.Fatal("followed after FollowSystemTheme(false)")
	}
	fb.Do(func() { uihost.FollowSystemTheme(r2, true) }) // applies the current appearance at once
	if v := look(fb, cv2, &s2); !v.th.Dark {
		t.Fatal("FollowSystemTheme(true) did not apply the dark theme")
	}
	fb.SetTheme(platform.Theme{})
	if v := look(fb, cv2, &s2); v.th.Dark {
		t.Fatal("not following again")
	}
	// The first window is still left alone.
	if v := look(fb, cv, &s); v.th != mine {
		t.Fatal("manual theme overwritten later")
	}
}

func TestAccentFlavorCustomize(t *testing.T) {
	resetLocale(t)
	a, fb := faketest.Run(t, tango.Options{})
	fb.SetTheme(platform.Theme{Accent: 0x112233ff})
	calls := 0
	var gotAccent *theme.Color
	opts := uihost.Options{
		AccentFromSystem: true,
		Flavor: func(dark bool, accent *theme.Color) *theme.Theme {
			gotAccent = accent
			o := []theme.Option{theme.WithPrimary(*accent)}
			if dark {
				o = append(o, theme.Dark())
			}
			return theme.ElementPlus(o...)
		},
		Customize: func(th *theme.Theme) { calls++; th.Name = "custom" },
	}
	var s seen
	_, _, cv := openWith(t, a, fb, opts, ui.RunnerOptions{}, s.page)
	v := look(fb, cv, &s)
	want := theme.Color{R: 0x11, G: 0x22, B: 0x33, A: 0xff}
	if gotAccent == nil || *gotAccent != want {
		t.Fatalf("accent %v", gotAccent)
	}
	if v.th.Flavor != theme.Element || v.th.Name != "custom" ||
		v.th.Primary.Base != theme.ElementPlus(theme.WithPrimary(want)).Primary.Base {
		t.Fatalf("theme %q flavor %v primary %v", v.th.Name, v.th.Flavor, v.th.Primary.Base)
	}
	fb.SetTheme(platform.Theme{Dark: true, Accent: 0x445566ff})
	v = look(fb, cv, &s)
	want2 := theme.Color{R: 0x44, G: 0x55, B: 0x66, A: 0xff}
	if !v.th.Dark || v.th.Name != "custom" || v.th.Primary.Base != theme.ElementPlus(theme.WithPrimary(want2), theme.Dark()).Primary.Base {
		t.Fatal("accent change not followed")
	}
	var n int
	fb.Do(func() { n = calls })
	if n != 2 {
		t.Fatalf("Customize called %d times", n)
	}

	// Without AccentFromSystem the default flavor keeps its own primary.
	var s2 seen
	_, _, cv2 := openWith(t, a, fb, uihost.Options{}, ui.RunnerOptions{}, s2.page)
	if v := look(fb, cv2, &s2); v.th.Primary.Base != theme.AntDesign(theme.Dark()).Primary.Base {
		t.Fatal("accent used without AccentFromSystem")
	}
	// The default flavor with AccentFromSystem.
	var s3 seen
	_, _, cv3 := openWith(t, a, fb, uihost.Options{AccentFromSystem: true}, ui.RunnerOptions{}, s3.page)
	if v := look(fb, cv3, &s3); v.th.Primary.Base != theme.AntDesign(theme.Dark(), theme.WithPrimary(want2)).Primary.Base {
		t.Fatal("default flavor ignores the accent")
	}
}

func TestThemeForMatchesWindow(t *testing.T) {
	o := uihost.Options{Theme: uihost.ThemeFixed}
	if o.ThemeFor(platform.Theme{}) != nil {
		t.Fatal("ThemeFixed generated a theme")
	}
	o = uihost.Options{Theme: uihost.ThemeLight}
	if th := o.ThemeFor(platform.Theme{Dark: true}); th.Dark {
		t.Fatal("ThemeLight drew dark")
	}
}

func TestLocaleFollowsI18n(t *testing.T) {
	resetLocale(t)
	a, fb := faketest.Run(t, tango.Options{})
	i18n.SetLocale("en")
	var s seen
	r, _, cv := openWith(t, a, fb, uihost.Options{}, ui.RunnerOptions{}, s.page)
	if v := look(fb, cv, &s); v.locale != ui.EnUS {
		t.Fatalf("locale %v", v.locale.Code)
	}
	i18n.SetLocale("ja") // from another goroutine
	waitFor(t, "Japanese texts", func() bool {
		v := look(fb, cv, &s)
		return v.locale == ui.Ja
	})
	if v := look(fb, cv, &s); v.text != ui.Ja.Cancel || v.text == ui.EnUS.Cancel {
		t.Fatalf("component text %q", v.text)
	}

	// The app sets the default locale itself: uihost stops following.
	fb.Do(func() { ui.SetDefaultLocale(ui.ZhTW); r.Invalidate() })
	i18n.SetLocale("en")
	time.Sleep(20 * time.Millisecond)
	fb.Do(func() {}) // the posted handler
	if v := look(fb, cv, &s); v.locale != ui.ZhTW {
		t.Fatalf("manual locale overwritten by %v", v.locale.Code)
	}
}

func TestLocaleFixed(t *testing.T) {
	resetLocale(t)
	a, fb := faketest.Run(t, tango.Options{})
	i18n.SetLocale("en")
	fb.Do(func() { ui.SetDefaultLocale(ui.ZhTW) })
	var s seen
	r, _, cv := openWith(t, a, fb, uihost.Options{Locale: uihost.LocaleFixed}, ui.RunnerOptions{}, s.page)
	i18n.SetLocale("ja")
	time.Sleep(20 * time.Millisecond)
	fb.Do(r.Invalidate)
	if v := look(fb, cv, &s); v.locale != ui.ZhTW {
		t.Fatalf("LocaleFixed window switched to %v", v.locale.Code)
	}
}

func TestSharedSubscription(t *testing.T) {
	resetLocale(t)
	a, fb := faketest.Run(t, tango.Options{})
	fb.SetTheme(platform.Theme{})
	calls := 0
	opts := uihost.Options{Customize: func(*theme.Theme) { calls++ }}
	var s1, s2 seen
	_, w1, cv1 := openWith(t, a, fb, opts, ui.RunnerOptions{}, s1.page)
	_, w2, cv2 := openWith(t, a, fb, opts, ui.RunnerOptions{}, s2.page)
	if n, sub := uihost.FollowStats(a); n != 2 || !sub {
		t.Fatalf("stats %d %v", n, sub)
	}
	fb.SetTheme(platform.Theme{Dark: true})
	if !look(fb, cv1, &s1).th.Dark || !look(fb, cv2, &s2).th.Dark {
		t.Fatal("not both dark")
	}
	closeAndWait(t, a, w1)
	if n, sub := uihost.FollowStats(a); n != 1 || !sub {
		t.Fatalf("after one close: %d %v", n, sub)
	}
	closeAndWait(t, a, w2)
	if n, sub := uihost.FollowStats(a); n != 0 || sub {
		t.Fatalf("after all closed: %d %v", n, sub)
	}
	fb.SetTheme(platform.Theme{}) // nobody listens: nothing happens

	// A new window subscribes again, once: one Customize per change.
	var s3 seen
	r3, _, cv3 := openWith(t, a, fb, opts, ui.RunnerOptions{}, s3.page)
	if n, sub := uihost.FollowStats(a); n != 1 || !sub {
		t.Fatalf("reopened: %d %v", n, sub)
	}
	var before int
	fb.Do(func() { before = calls })
	fb.SetTheme(platform.Theme{Dark: true})
	fb.SetTheme(platform.Theme{})
	var after int
	fb.Do(func() { after = calls })
	if after-before != 2 || !following(a, r3) || look(fb, cv3, &s3).th.Dark {
		t.Fatalf("Customize ran %d times for 2 changes", after-before)
	}
}

func closeAndWait(t *testing.T, a *tango.App, s *tango.Surface) {
	t.Helper()
	closed := make(chan struct{})
	tango.Await(a, func() struct{} { s.OnClosed(func() { close(closed) }); return struct{}{} })
	s.Close()
	select {
	case <-closed:
	case <-time.After(faketest.Timeout):
		t.Fatal("surface not closed")
	}
	tango.Await(a, func() struct{} { return struct{}{} })
}

func TestAttachFollows(t *testing.T) {
	resetLocale(t)
	a, fb := faketest.Run(t, tango.Options{})
	fb.SetTheme(platform.Theme{})
	s := webSurface(t, a, 400, 300)
	var sn seen
	r, at, cv := attach(t, a, fb, s, uihost.Layer{Above: true}, sn.page)
	fb.SetTheme(platform.Theme{Dark: true})
	cv.FlushFrame()
	var dark bool
	fb.Do(func() { dark = sn.th.Dark })
	if !dark || !following(a, r) {
		t.Fatal("attached canvas did not follow")
	}
	if n, _ := uihost.FollowStats(a); n != 1 {
		t.Fatalf("stats %d", n)
	}
	fb.Do(at.Detach)
	if n, sub := uihost.FollowStats(a); n != 0 || sub {
		t.Fatalf("after Detach: %d %v", n, sub)
	}

	// AttachWith takes the window options.
	o := tango.Await(a, func() attached {
		r, at, err := uihost.AttachWith(s, uihost.Layer{}, uihost.Options{Theme: uihost.ThemeLight}, sn.page, ui.RunnerOptions{})
		return attached{r, at, err}
	})
	if o.err != nil {
		t.Fatal(o.err)
	}
	if tango.Await(a, func() bool { return o.r.Context().Theme.Dark }) {
		t.Fatal("ThemeLight attached canvas is dark")
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(faketest.Timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
