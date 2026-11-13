package ui

// Component token tests of group G2 (Slider).

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// TestG2SliderTokenScoped: a Slider token override inside a nested
// ConfigProvider repaints only the slider in that scope.
func TestG2SliderTokenScoped(t *testing.T) {
	mark := theme.Hex("#12ab34")
	handle := theme.Hex("#ab1234")
	a, b := 30.0, 30.0
	var outer, inner *SliderW
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			outer = Slider(c, &a, 0, 100).Layout(layout.W(300))
			ConfigProvider(c).Components(theme.Components{Slider: theme.SliderToken{TrackBg: mark, HandleColor: handle, RailSize: 10}}).Body(func() {
				inner = Slider(c, &b, 0, 100).Layout(layout.W(300))
			})
		}, layout.Gap(20), layout.Pad(20))
	}, DriverOptions{Width: 400, Height: 300, Theme: theme.AntDesign()})
	d.Settle()
	ot, it := p2bRect(t, d, outer.track), p2bRect(t, d, inner.track)
	in := func(r, box geom.Rect) bool { return r.Y >= box.Y-12 && r.Bottom() <= box.Bottom()+12 }
	var markIn, markOut, handleIn, railH bool
	for _, op := range d.r.List().Ops {
		switch o := op.(type) {
		case paint.FillRect:
			if o.Color == mark {
				markIn = markIn || in(o.Rect, it)
				markOut = markOut || in(o.Rect, ot)
			}
			if in(o.Rect, it) && o.Rect.H == 10 {
				railH = true
			}
		case paint.FillPath:
			if o.Color == handle {
				handleIn = true
			}
		}
	}
	if !markIn || !handleIn || !railH {
		t.Fatalf("override not painted in scope: track=%v handle=%v rail=%v", markIn, handleIn, railH)
	}
	if markOut {
		t.Fatal("override leaked to the outer slider")
	}
}

// TestG2SliderTokenDefaults: the derived Slider token equals the values
// Slider painted before it read tokens, in all six themes.
func TestG2SliderTokenDefaults(t *testing.T) {
	ths := map[string]*theme.Theme{
		"ant": theme.AntDesign(), "ant-dark": theme.AntDesign(theme.Dark()),
		"element": theme.ElementPlus(), "element-dark": theme.ElementPlus(theme.Dark()),
		"shadcn": theme.Shadcn(), "shadcn-dark": theme.Shadcn(theme.Dark()),
	}
	for name, th := range ths {
		k := th.Components.Slider
		var rail, railHov, track, trackHov, ring, ringAct, bg, halo theme.Color
		var hs, hsHov, lw, lwHov, dot float32
		switch th.Flavor {
		case theme.Element:
			rail, railHov, track, trackHov = th.Split, th.Split, th.Primary.Base, th.Primary.Base
			ring, ringAct, bg = th.Primary.Base, th.Primary.Base, th.BgContainer
			hs, hsHov, lw, lwHov, dot = 20, 24, 2, 2, 6
		case theme.StyleShadcn:
			rail, railHov, track, trackHov = th.FillSecondary, th.FillSecondary, th.Primary.Base, th.Primary.Base
			ring, ringAct, bg = th.Primary.Base, th.Primary.Base, theme.Hex("#ffffff")
			halo = th.ControlOutline
			if halo.A == 0 {
				halo = theme.Fade(th.FocusOutline, .5)
			}
			hs, hsHov, lw, lwHov, dot = 16, 16, 1, 1, 8
		default:
			rail, railHov, track, trackHov = th.FillTertiary, th.FillSecondary, th.Primary.Border, th.Primary.BorderHover
			ring, ringAct, bg = th.Primary.Border, th.Primary.Base, th.BgContainer
			halo = theme.Fade(th.Primary.Base, .2)
			hs, hsHov, lw, lwHov, dot = 10, 12, 2, 4, 8
		}
		if k.RailSize != th.SliderRailHeight || k.RailBg != rail || k.RailHoverBg != railHov || k.TrackBg != track ||
			k.TrackHoverBg != trackHov || k.HandleColor != ring || k.HandleActiveColor != ringAct || k.HandleBg != bg ||
			k.HandleSize != hs || k.HandleSizeHover != hsHov || k.HandleLineWidth != lw || k.HandleLineWidthHover != lwHov ||
			k.DotSize != dot || k.TrackBgDisabled != th.TextDisabled || k.HandleColorDisabled != th.TextDisabled ||
			k.DotBorderColor != th.BorderSecondary || (th.Flavor != theme.Element && k.HandleActiveOutlineColor != halo) {
			t.Errorf("%s: slider token %+v", name, k)
		}
	}
}
