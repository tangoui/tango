package theme

import "testing"

// TestShadcnAnchorLinks: shadcn's anchor follows the docs' "On this page"
// list: muted-foreground links, the current and hovered link foreground.
// With shadcn's near-black primary the Ant colors (primary active link)
// left the current link indistinguishable from the others.
func TestShadcnAnchorLinks(t *testing.T) {
	for _, dark := range []bool{false, true} {
		var opts []ShadcnArg
		if dark {
			opts = append(opts, Dark())
		}
		th := Shadcn(opts...)
		a := th.Components.Anchor
		if a.LinkColor != th.TextTertiary || a.LinkActiveColor != th.Text || a.LinkHoverColor != th.Text {
			t.Errorf("dark=%v anchor: link %v active %v hover %v", dark, a.LinkColor, a.LinkActiveColor, a.LinkHoverColor)
		}
		if a.LinkColor == a.LinkActiveColor {
			t.Errorf("dark=%v: the current link has the same color as the others", dark)
		}
	}
}

// TestShadcnEmptyMedia: shadcn's Empty draws a size-10 rounded-lg bg-muted
// icon tile; the others keep their illustrations (no tile).
func TestShadcnEmptyMedia(t *testing.T) {
	th := Shadcn()
	e := th.Components.Empty
	if e.ImageHeight != 40 || e.MediaBg != th.FillSecondary || e.MediaColor != th.Text || e.MediaRadius != th.RadiusLG {
		t.Errorf("shadcn empty: %+v", e)
	}
	for _, o := range []*Theme{AntDesign(), ElementPlus()} {
		if o.Components.Empty.MediaBg.A != 0 {
			t.Errorf("%v: empty has an icon tile", o.Flavor)
		}
	}
}
