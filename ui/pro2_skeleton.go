package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
)

// ---------------------------------------------------------------------------
// ProSkeleton (Ant Design Pro ProSkeleton): page-level loading layouts.
//
//	ui.ProSkeleton(c, ui.SkeletonList).Active(true).Show()
//	ui.ProSkeleton(c, ui.SkeletonResult).Show()
//	ui.ProSkeleton(c, ui.SkeletonDescriptions).Show()

// ProSkeleton page types.
const (
	SkeletonList         = "list"
	SkeletonResult       = "result"
	SkeletonDescriptions = "descriptions"
)

// ProSkeletonW builds a page skeleton.
type ProSkeletonW struct {
	_ [0]func()

	c          *Context
	kind       string
	active     bool
	pageHeader bool
	statistic  int
	list       int
	listSet    bool
	toolbar    bool
	opts       []layout.Option
}

// ProSkeleton starts a page skeleton of kind (SkeletonList, SkeletonResult
// or SkeletonDescriptions; "" = list). Defaults: active, with page header,
// 2 statistics, a toolbar and 5 list rows (4 table rows on the
// descriptions page).
//
// Pro v3 picks the statistic count from the first breakpoint key that is
// true; antd's screens put xs/sm first, so in a browser that is always 2
// (xs and sm both map to 2). Tango renders that observed default, not the
// md/xl entries of the lookup table.
func ProSkeleton(c *Context, kind string) *ProSkeletonW {
	if kind == "" {
		kind = SkeletonList
	}
	// ConfigProvider.Defaults("ProSkeleton", ...) keys: "active",
	// "pageHeader", "toolbar" (bool), "statistic", "list" (number).
	s := &ProSkeletonW{c: c, kind: kind, active: true, pageHeader: true, statistic: 2, list: 5, toolbar: true}
	s.active = Prop(c, "ProSkeleton", "active", s.active)
	s.pageHeader = Prop(c, "ProSkeleton", "pageHeader", s.pageHeader)
	s.toolbar = Prop(c, "ProSkeleton", "toolbar", s.toolbar)
	if n, ok := proPropInt(c, "ProSkeleton", "statistic"); ok {
		s.statistic = n
	}
	if n, ok := proPropInt(c, "ProSkeleton", "list"); ok {
		s.list, s.listSet = n, true
	}
	return s
}

// Active turns the shimmer on or off.
func (s *ProSkeletonW) Active(on bool) *ProSkeletonW { s.active = on; return s }

// PageHeader shows or hides the page header placeholder.
func (s *ProSkeletonW) PageHeader(on bool) *ProSkeletonW { s.pageHeader = on; return s }

// Statistic sets the number of statistic placeholders (list type; 0 hides
// them; default 2).
func (s *ProSkeletonW) Statistic(n int) *ProSkeletonW { s.statistic = n; return s }

// List sets the number of list rows (list type, default 5) or table rows
// (descriptions type, default 4); 0 hides the list, or the divider and
// table. The list toolbar is controlled by Toolbar alone.
func (s *ProSkeletonW) List(n int) *ProSkeletonW { s.list, s.listSet = n, true; return s }

// Toolbar shows or hides the list toolbar placeholder.
func (s *ProSkeletonW) Toolbar(on bool) *ProSkeletonW { s.toolbar = on; return s }

// Layout applies layout options to the skeleton.
func (s *ProSkeletonW) Layout(o ...layout.Option) *ProSkeletonW {
	s.opts = append(s.opts, o...)
	return s
}

// Show builds the skeleton.
func (s *ProSkeletonW) Show() *Box {
	c := s.c
	opts := append([]layout.Option{layout.Gap(24), layout.MinW(0)}, s.opts...)
	b := pro2Box(c, "pro2-skeleton", s.kind, layout.Column, opts)
	b.Semantics(a11y.RoleStatus, c.Locale().Loading).A11yState(a11y.Busy)
	b.Body(func() {
		switch s.kind {
		case SkeletonResult:
			if s.pageHeader {
				s.header()
			}
			s.result()
		case SkeletonDescriptions:
			if s.pageHeader {
				s.header()
			}
			s.descriptions()
		default:
			if s.pageHeader {
				s.header()
			}
			if s.statistic > 0 {
				s.statistics()
			}
			if s.list > 0 || s.toolbar {
				s.listCard()
			}
		}
	})
	return b
}

// card is a white surface holding skeleton content.
func (s *ProSkeletonW) card(fn func(), opts ...layout.Option) *Box {
	c, th := s.c, s.c.Theme
	rad := th.RadiusLG
	if pro2El(c) {
		rad = th.Radius
	}
	if pro2Shad(c) {
		rad = shad2dCardRadius(th)
	}
	pad := float32(24)
	if pro2El(c) {
		pad = 20
	}
	o := append([]layout.Option{layout.Pad(pad)}, opts...)
	b := Column(c, fn, o...).Bg(th.BgContainer).Radius(rad)
	if pro2El(c) || pro2Shad(c) {
		b.Border(1, th.BorderSecondary)
	}
	if pro2Shad(c) {
		b.Shadow(th.CardShadow)
	}
	return b
}

// bar adds a block of fixed width (0 stretches) and height.
func (s *ProSkeletonW) bar(w, h float32, opts ...layout.Option) *skelW {
	b := skelBlock(s.c, s.active, w, h, false, false, opts...)
	if w == 0 {
		b.n.ln.MinW = 0
	}
	return b
}

// header: breadcrumb and title placeholders.
func (s *ProSkeletonW) header() {
	c := s.c
	bars := func() {
		s.bar(185, 14)
		s.bar(240, 24)
	}
	if pro2Shad(c) {
		// On the page surface (bordered card) so the accent bars keep
		// their contrast whatever the surrounding fill is, light or dark.
		s.card(bars, layout.Gap(16))
		return
	}
	Column(c, bars, layout.Gap(16))
}

// statistics: one card with n columns split by vertical dividers.
func (s *ProSkeletonW) statistics() {
	c, th := s.c, s.c.Theme
	n := s.statistic
	split := th.Split
	s.card(func() {
		Row(c, func() {
			for i := 0; i < n; i++ {
				col := Column(c, func() {
					s.bar(100, 14)
					s.bar(0, 32).widthPct = 0.6
				}, layout.Grow(1), layout.Basis(0), layout.MinW(0), layout.Gap(12),
					layout.Padding(geom.Insets{Left: map[bool]float32{true: 0, false: 24}[i == 0]}))
				if i > 0 {
					col.Paint(func(l *paint.List, r geom.Rect) { dataVLine(l, dataStartX(c, r), r.Y, r.H, split) })
				}
			}
		})
	})
}

// listCard: toolbar and rows.
func (s *ProSkeletonW) listCard() {
	c, th := s.c, s.c.Theme
	split := th.Split
	s.card(func() {
		if s.toolbar {
			Row(c, func() {
				s.bar(100, 24)
				Spacer(c)
				SkeletonButton(c, Middle, "default", s.active)
				SkeletonButton(c, Middle, "default", s.active)
			}, layout.Middle(), layout.Gap(16), layout.Margin(geom.Insets{Bottom: 8}))
		}
		for i := 0; i < s.list; i++ {
			row := Row(c, func() {
				Column(c, func() {
					s.bar(0, 16).widthPct = 0.3
					s.bar(0, 14).widthPct = 0.7
				}, layout.Grow(1), layout.MinW(0), layout.Gap(10))
				s.bar(48, 14, layout.Self(layout.AlignCenter))
			}, layout.PadVH(16, 0), layout.Gap(24))
			if i > 0 || s.toolbar {
				row.Paint(func(l *paint.List, r geom.Rect) { dataHLine(l, r.X, r.Y, r.W, split) })
			}
		}
	})
}

// result: icon, title, sub title and actions centred.
func (s *ProSkeletonW) result() {
	c := s.c
	s.card(func() {
		Column(c, func() {
			skelBlock(c, s.active, 72, 72, false, true)
			s.bar(280, 24, layout.Margin(geom.Insets{Top: 24}))
			s.bar(400, 14, layout.Margin(geom.Insets{Top: 16}))
			Row(c, func() {
				SkeletonButton(c, Middle, "default", s.active)
				SkeletonButton(c, Middle, "default", s.active)
			}, layout.Gap(16), layout.Margin(geom.Insets{Top: 32}))
		}, layout.AlignItems(layout.AlignCenter), layout.PadVH(48, 0))
	})
}

// descCols is the descriptions-page column count. Pro v3 takes the first
// true breakpoint key (xs below 576px, otherwise sm), so in a browser it
// is 1 on xs and 2 elsewhere (Descriptions MediaQueryKeyEnum xs 1, sm 2).
func (s *ProSkeletonW) descCols() int {
	if ScreenBreakpoint(s.c) == XS {
		return 1
	}
	return 2
}

// descriptions: a descriptions card and, unless List(0), a table card
// with List rows (default 4).
func (s *ProSkeletonW) descriptions() {
	c, th := s.c, s.c.Theme
	split := th.Split
	cols := s.descCols()
	s.card(func() {
		s.bar(160, 20, layout.Margin(geom.Insets{Bottom: 24}))
		for r := 0; r < 3; r++ {
			Row(c, func() {
				for col := 0; col < cols; col++ {
					Row(c, func() {
						s.bar(64, 14)
						s.bar(0, 14, layout.Grow(1)).widthPct = 0.7
					}, layout.Grow(1), layout.Basis(0), layout.MinW(0), layout.Gap(12), layout.Middle())
				}
			}, layout.Gap(24), layout.Margin(geom.Insets{Bottom: 16}))
		}
	})
	rows := 4 // TableSkeleton size default
	if s.listSet {
		rows = s.list
	}
	if rows <= 0 {
		return
	}
	// DescriptionsPageSkeleton renders Line only when the table is shown.
	Column(c, nil, layout.H(1), layout.Margin(geom.Insets{Left: 24, Right: 24})).Bg(split)
	s.card(func() {
		s.bar(160, 20, layout.Margin(geom.Insets{Bottom: 24}))
		for r := 0; r <= rows; r++ { // header row + rows
			row := Row(c, func() {
				h := float32(14)
				if r == 0 {
					h = 16
				}
				// TableItemSkeleton: cols flex-1 cells and one flex-3 cell.
				for col := 0; col < cols; col++ {
					s.bar(0, h, layout.Grow(1), layout.Basis(0)).widthPct = 0.6
				}
				s.bar(0, h, layout.Grow(3), layout.Basis(0)).widthPct = 0.6
			}, layout.Gap(24), layout.PadVH(16, 16))
			if r > 0 {
				row.Paint(func(l *paint.List, rc geom.Rect) { dataHLine(l, rc.X, rc.Y, rc.W, split) })
			} else {
				row.Bg(th.FillAlter)
			}
		}
	})
}
