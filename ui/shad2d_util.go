package ui

import (
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// shadcn/ui look of the Pro and layout components (ProLayout, Sider,
// PageContainer, FooterToolbar, ProCard, StatisticCard, CheckCard,
// ProSkeleton, ProDescriptions, ProList, LightFilter, LoginForm, ProTable,
// QueryFilter) and the ConfigProvider.Defaults readers they share.
//
// shadcn has no Pro components; the shapes follow its blocks: the Sidebar
// block (sidebar surface, muted group labels, rounded-md items, icon rail
// when collapsed, user row at the bottom), the dashboard block (cards
// rounded-xl border shadow-sm, font-semibold titles, muted descriptions,
// header action at the top right), the login block (card with title,
// description, labelled fields, full-width button, "or continue with"
// separator) and the data-table example (toolbar with dashed facet
// buttons and a column toggle, rounded-md bordered table, muted selection
// count with Previous / Next outline buttons).

// shad2d reports the shadcn flavor.
func shad2d(c *Context) bool { return c.Theme.Flavor == theme.StyleShadcn }

// shad2dCardRadius is shadcn's rounded-xl (--radius + 4px) used by Card.
func shad2dCardRadius(th *theme.Theme) float32 {
	if th.Shadcn != nil {
		return th.Shadcn.Radius + 4
	}
	return th.RadiusLG + 4
}

// shad2dSidebar returns the sidebar surface, its border, the accent of a
// hovered / selected item and the muted text of group labels.
func shad2dSidebar(th *theme.Theme) (bg, border, accent, accentFg, fg, muted theme.Color) {
	if v := th.Shadcn; v != nil {
		return v.Sidebar, v.SidebarBorder, v.SidebarAccent, v.SidebarAccentForeground, v.SidebarForeground, theme.Fade(v.SidebarForeground, .7)
	}
	return th.SiderLightBg, th.Split, th.ItemHoverBg, th.ItemSelected, th.Text, th.TextTertiary
}

// shad2dPrimary returns --primary and --primary-foreground.
func shad2dPrimary(th *theme.Theme) (theme.Color, theme.Color) {
	if v := th.Shadcn; v != nil {
		return v.Primary, v.PrimaryForeground
	}
	return th.Primary.Base, th.TextOnPrimary
}

// shad2dMuted returns --muted and --muted-foreground.
func shad2dMuted(th *theme.Theme) (theme.Color, theme.Color) {
	if v := th.Shadcn; v != nil {
		return v.Muted, v.MutedForeground
	}
	return th.FillTertiary, th.TextTertiary
}

// shad2dBottomLine / shad2dTopLine / shad2dRightLine paint a 1px border on
// one edge of a box.
func shad2dBottomLine(col theme.Color) PaintFunc {
	return func(l *paint.List, r geom.Rect) { dataHLine(l, r.X, r.Bottom()-1, r.W, col) }
}

func shad2dTopLine(col theme.Color) PaintFunc {
	return func(l *paint.List, r geom.Rect) { dataHLine(l, r.X, r.Y, r.W, col) }
}

func shad2dRightLine(col theme.Color) PaintFunc {
	return func(l *paint.List, r geom.Rect) { dataVLine(l, r.Right()-1, r.Y, r.H, col) }
}

// shad2dEndLine paints a 1px border on the end edge of a box (right in
// LTR, left in RTL): a start-side sidebar's border-r.
func shad2dEndLine(c *Context, col theme.Color) PaintFunc {
	return func(l *paint.List, r geom.Rect) { dataVLine(l, c.inline(r).X(r.W-1, 1), r.Y, r.H, col) }
}

// Small line icons drawn on a 24px grid (lucide-like shapes, own paths).

func shad2dStroke(l *paint.List, p *paint.Path, s float32, col theme.Color) {
	l.StrokePath(p, paint.StrokeStyle{Width: max(1.2, 2*s), Cap: paint.CapRound}, col)
}

// shad2dIconPanel draws a "panel left" mark (sidebar toggle) in r.
func shad2dIconPanel(l *paint.List, r geom.Rect, col theme.Color) {
	s := r.W / 24
	x, y := r.X, r.Y
	shad2dStroke(l, paint.NewPath().RRect(geom.Rect{X: x + 3*s, Y: y + 3*s, W: 18 * s, H: 18 * s}, geom.Rad(2*s)), s, col)
	shad2dStroke(l, paint.NewPath().MoveTo(x+9*s, y+3*s).LineTo(x+9*s, y+21*s), s, col)
}

// shad2dIconPanelDir is shad2dIconPanel with the panel divider on the
// start side: mirrored in RTL, where the sidebar sits on the right.
func (c *Context) shad2dIconPanelDir(l *paint.List, r geom.Rect, col theme.Color) {
	if !c.IsRTL() {
		shad2dIconPanel(l, r, col)
		return
	}
	s := r.W / 24
	x, y := r.X, r.Y
	shad2dStroke(l, paint.NewPath().RRect(geom.Rect{X: x + 3*s, Y: y + 3*s, W: 18 * s, H: 18 * s}, geom.Rad(2*s)), s, col)
	shad2dStroke(l, paint.NewPath().MoveTo(x+15*s, y+3*s).LineTo(x+15*s, y+21*s), s, col)
}

// shad2dIconPlusCircle draws a circled plus (facet filter button).
func shad2dIconPlusCircle(l *paint.List, r geom.Rect, col theme.Color) {
	s := r.W / 24
	cx, cy := r.X+12*s, r.Y+12*s
	shad2dStroke(l, paint.NewPath().Circle(cx, cy, 10*s), s, col)
	shad2dStroke(l, paint.NewPath().MoveTo(cx-4*s, cy).LineTo(cx+4*s, cy).MoveTo(cx, cy-4*s).LineTo(cx, cy+4*s), s, col)
}

// shad2dIconChevrons draws the up-down chevrons of a menu trigger.
func shad2dIconChevrons(l *paint.List, r geom.Rect, col theme.Color) {
	s := r.W / 24
	x, y := r.X, r.Y
	shad2dStroke(l, paint.NewPath().MoveTo(x+7*s, y+15*s).LineTo(x+12*s, y+20*s).LineTo(x+17*s, y+15*s), s, col)
	shad2dStroke(l, paint.NewPath().MoveTo(x+7*s, y+9*s).LineTo(x+12*s, y+4*s).LineTo(x+17*s, y+9*s), s, col)
}

// shad2dIconSliders draws two horizontal sliders (column view toggle).
func shad2dIconSliders(l *paint.List, r geom.Rect, col theme.Color) {
	s := r.W / 24
	x, y := r.X, r.Y
	shad2dStroke(l, paint.NewPath().MoveTo(x+4*s, y+7*s).LineTo(x+20*s, y+7*s).MoveTo(x+4*s, y+17*s).LineTo(x+20*s, y+17*s), s, col)
	l.Path(paint.NewPath().Circle(x+15*s, y+7*s, 3*s), col)
	l.Path(paint.NewPath().Circle(x+9*s, y+17*s, 3*s), col)
}

// shad2dIconCheck draws a check mark.
func shad2dIconCheck(l *paint.List, r geom.Rect, col theme.Color) {
	s := r.W / 24
	x, y := r.X, r.Y
	shad2dStroke(l, paint.NewPath().MoveTo(x+5*s, y+12.5*s).LineTo(x+10*s, y+17*s).LineTo(x+19*s, y+7*s), s*1.2, col)
}

// shad2dIconFn adapts an icon painter to a fixed-size Custom node.
func shad2dIconNode(c *Context, size float32, draw func(*paint.List, geom.Rect, theme.Color), col theme.Color) *Box {
	return Custom(c, func(_, _ float32) geom.Size { return geom.Size{W: size, H: size} }, func(l *paint.List, r geom.Rect) {
		draw(l, r, col)
	})
}

// ---------------------------------------------------------------------------
// ConfigProvider.Defaults readers (keys follow the ProComponents / Ant
// property names; explicit modifiers always win).

// proPropSize reads a "size" default: a ControlSize or "small" /
// "middle" / "default" / "large".
func proPropSize(c *Context, comp string) (ControlSize, bool) {
	v, ok := c.ConfigProp(comp, "size")
	if !ok {
		return Middle, false
	}
	switch x := v.(type) {
	case ControlSize:
		return x, true
	case string:
		switch x {
		case "small", "sm":
			return Small, true
		case "middle", "default", "medium":
			return Middle, true
		case "large", "lg":
			return Large, true
		}
	}
	return Middle, false
}

// proPropInt reads an integer default (any Go number).
func proPropInt(c *Context, comp, key string) (int, bool) {
	f, ok := p2bNum(c, comp, key)
	return int(f), ok
}

// proPropPageSize reads "pagination": false (no pager, ok with 0), true
// (keep the default), a page size number, or a Props map holding
// "pageSize". The second result reports whether pagination is on.
func proPropPageSize(c *Context, comp string, def int) (int, bool) {
	v, ok := c.ConfigProp(comp, "pagination")
	if !ok {
		if n, ok := proPropInt(c, comp, "pageSize"); ok && n > 0 {
			return n, true
		}
		return def, def > 0
	}
	switch x := v.(type) {
	case bool:
		if !x {
			return 0, false
		}
		return max(def, 10), true
	case Props:
		if n, ok := x["pageSize"]; ok {
			if f, ok := proAnyNum(n); ok && f > 0 {
				return int(f), true
			}
		}
		return max(def, 10), true
	case map[string]any:
		return proPropPageSizeMap(x, def)
	}
	if f, ok := proAnyNum(v); ok && f > 0 {
		return int(f), true
	}
	return def, def > 0
}

func proPropPageSizeMap(m map[string]any, def int) (int, bool) {
	if f, ok := proAnyNum(m["pageSize"]); ok && f > 0 {
		return int(f), true
	}
	return max(def, 10), true
}

// proAnyNum converts any Go number.
func proAnyNum(v any) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case float32:
		return float64(x), true
	case float64:
		return x, true
	case time.Duration:
		return float64(x), true
	}
	return 0, false
}
