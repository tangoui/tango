package icon_test

import (
	"image/color"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/icon/antd"
	"github.com/tangoui/tango/ui/icon/element"
	"github.com/tangoui/tango/ui/icon/lucide"
	"github.com/tangoui/tango/ui/paint"
)

// TestSetsDraw renders every icon of the three sets and checks it covers
// a plausible share of its square: catches path data the parser misreads,
// broken view boxes and even-odd conversions that filled or emptied an icon.
func TestSetsDraw(t *testing.T) {
	sets := map[string][]*icon.Icon{"ep:": element.All(), "ant:": antd.All(), "lucide:": lucide.All()}
	want := map[string]int{"ep:": 293, "ant:": 848, "lucide:": 2052}
	r := paint.NewCPU()
	for prefix, all := range sets {
		if len(all) != want[prefix] {
			t.Errorf("%s: %d icons, want %d", prefix, len(all), want[prefix])
		}
		for _, ic := range all {
			if !strings.HasPrefix(ic.Name, prefix) {
				t.Errorf("%s: name without prefix %s", ic.Name, prefix)
			}
			var l paint.List
			icon.Draw(&l, ic, geom.R(0, 0, 32, 32), color.NRGBA{A: 255}, 0)
			if err := r.Render(&l, geom.Size{W: 32, H: 32}, 1); err != nil {
				t.Fatal(err)
			}
			img := r.Image()
			cov := 0
			for i := 3; i < len(img.Pix); i += 4 {
				cov += int(img.Pix[i])
			}
			share := float64(cov) / 255 / (32 * 32)
			if share < 0.003 || share > 0.95 {
				t.Errorf("%s covers %.1f%% of its square", ic.Name, share*100)
			}
		}
	}
}

func TestSetsRegister(t *testing.T) {
	if icon.ByName("lucide:search") != nil {
		t.Fatal("sets must not register themselves")
	}
	element.Register()
	antd.Register()
	lucide.Register()
	for name, want := range map[string]*icon.Icon{
		"ep:arrow-left":            element.ArrowLeft,
		"ant:search-outlined":      antd.SearchOutlined,
		"ant:check-circle-twotone": antd.CheckCircleTwoTone,
		"lucide:chevron-down":      lucide.ChevronDown,
		"lucide:filter":            lucide.Funnel, // alias
		"search":                   icon.Search,   // built-in still there
	} {
		if got := icon.ByName(name); got != want {
			t.Errorf("ByName(%q) = %v, want %v", name, got, want)
		}
	}
	if !lucide.ChevronLeft.IsDirectional() || lucide.ChevronDown.IsDirectional() || !antd.ArrowRightOutlined.IsDirectional() {
		t.Error("directional marks")
	}
}

// TestTwoTone checks secondary parts take DrawOpts.Secondary.
func TestTwoTone(t *testing.T) {
	var l paint.List
	main, sec := color.NRGBA{R: 255, A: 255}, color.NRGBA{B: 255, A: 255}
	icon.DrawWith(&l, antd.CheckCircleTwoTone, geom.R(0, 0, 32, 32), main, icon.DrawOpts{Secondary: sec})
	seen := map[color.NRGBA]bool{}
	for _, op := range l.Ops {
		if f, ok := op.(paint.FillPath); ok {
			seen[f.Color] = true
		}
	}
	if !seen[main] || !seen[sec] || len(seen) != 2 {
		t.Fatalf("colors %v", seen)
	}
}
