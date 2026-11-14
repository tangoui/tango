package ui

// CascaderPanel: the cascading column menus of Cascader shown inline
// (Ant Cascader.Panel, Element el-cascader-panel). It shares the model of
// CascaderW (columns, single / multiple selection, checked strategy, lazy
// loading); only the trigger and the popup are gone.

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

type casPanelState struct {
	init   bool
	synced string // casKey of the single value the expanded path follows
}

// CascaderPanelW is an inline cascader; chain options, read Changed.
type CascaderPanelW struct {
	_ [0]func()

	c        *Context
	cas      *CascaderW
	box      *Box
	ps       *casPanelState
	z        *lazy
	disabled bool
	border   bool
}

// CascaderPanel adds the column menus of a Cascader inline, bound to *val
// (the selected value path). Clicking a leaf selects it (the menus stay);
// the panel is one Tab stop whose arrow keys move through the columns
// (Right / Enter opens the next column, Left goes back; mirrored in RTL),
// Enter / Space on a leaf selects it.
//
//	ui.CascaderPanel(c, &path, options).ChangeOnSelect()
func CascaderPanel(c *Context, val *[]string, opts []CascaderOption) *CascaderPanelW {
	if val == nil {
		val = new([]string) // Multiple-only panels need no single value
	}
	w := &CascaderW{c: c, val: val, opts: opts, strategy: ShowParent}
	w.th = c.ThemeFor(theme.CompCascader)
	w.tk = &w.th.Components.Cascader
	p := &CascaderPanelW{c: c, cas: w, border: true}
	p.box = c.box("cascader-panel", layout.Row, nil)
	p.box.n.ln.NoStretch = true
	w.st = State[casState](c, hashID(p.box.n.id, "cas"), nil)
	w.st.pop.open = true // the menus are always shown
	p.ps = State[casPanelState](c, hashID(p.box.n.id, "panel"), nil)
	w.focusID = p.box.n.id
	p.z = c.lazy(p.box.n, p.build)
	return p
}

// A11yLabel names the panel and its first column (default: the generic
// "please select").
func (p *CascaderPanelW) A11yLabel(s string) *CascaderPanelW { p.box.n.setName(s); return p }

// ChangeOnSelect selects parents too (clicking a parent selects it and
// opens its column).
func (p *CascaderPanelW) ChangeOnSelect() *CascaderPanelW { p.cas.changeOnSelect = true; return p }

// ExpandOnHover opens a parent's column on hover.
func (p *CascaderPanelW) ExpandOnHover() *CascaderPanelW { p.cas.hoverExpand = true; return p }

// Multiple switches to checkboxes bound to *vals (the checked leaf paths;
// see ShowCheckedStrategy and StrategyValue).
func (p *CascaderPanelW) Multiple(vals *[][]string) *CascaderPanelW { p.cas.multi = vals; return p }

// LoadData loads children lazily (see CascaderW.LoadData).
func (p *CascaderPanelW) LoadData(fn func(path []string, done func([]CascaderOption))) *CascaderPanelW {
	p.cas.loadData = fn
	return p
}

// ShowCheckedStrategy sets the checked strategy of multiple mode (see
// CascaderW.ShowCheckedStrategy); CheckedPaths returns its paths.
func (p *CascaderPanelW) ShowCheckedStrategy(s CheckedStrategy) *CascaderPanelW {
	p.cas.strategy = s
	return p
}

// StrategyValue makes the bound value of multiple mode follow the checked
// strategy (see CascaderW.StrategyValue).
func (p *CascaderPanelW) StrategyValue(on bool) *CascaderPanelW { p.cas.stratVal = on; return p }

// Expand sets the expanded path (by default it follows the value).
func (p *CascaderPanelW) Expand(path ...string) *CascaderPanelW {
	p.cas.Expand(path...)
	p.ps.init = true
	if p.cas.val != nil {
		p.ps.synced = casKey(*p.cas.val)
	}
	return p
}

// Height sets the column height (default: the Cascader token's
// DropdownHeight).
func (p *CascaderPanelW) Height(px float32) *CascaderPanelW { p.cas.colH = px; return p }

// Border toggles the panel frame (default on).
func (p *CascaderPanelW) Border(on bool) *CascaderPanelW { p.border = on; return p }

// Disabled turns the panel off.
func (p *CascaderPanelW) Disabled(on bool) *CascaderPanelW {
	if on {
		p.disabled = true
	}
	return p
}

// Changed reports a new selection this frame.
func (p *CascaderPanelW) Changed() bool { p.z.run(); return p.cas.changed }

// CheckedPaths returns the checked paths of multiple mode by the checked
// strategy.
func (p *CascaderPanelW) CheckedPaths() [][]string {
	p.z.run()
	_, paths := p.cas.chips()
	return paths
}

// ID returns the panel's ID.
func (p *CascaderPanelW) ID() ID { return p.box.n.id }

func (p *CascaderPanelW) build() {
	w := p.cas
	var user *[][]string
	if w.stratVal && w.multi != nil {
		// Work on the checked leaf paths; write the strategy paths back.
		user = w.multi
		leaves := w.casExpandPaths(*user)
		w.multi = &leaves
	}
	p.buildPanel()
	if user != nil {
		if w.changed {
			_, paths := w.chips()
			*user = paths
		}
		w.multi = user
	}
}

func (p *CascaderPanelW) buildPanel() {
	c, w, st := p.c, p.cas, p.cas.st
	th := w.th
	b := p.box
	n := b.n
	w.pollLoads()
	st.pop.open = true
	// The expanded path follows the single value (first build and when the
	// application changes it).
	if w.multi == nil && w.val != nil {
		if k := casKey(*w.val); !p.ps.init || k != p.ps.synced {
			st.path = append([]string(nil), *w.val...)
			st.kcol = max(0, len(st.path)-1)
			p.ps.synced = k
		}
	}
	p.ps.init = true
	off := p.disabled || n.disabled
	name := popupName(c, n, "")
	n.ownKeys = true
	var ia Interaction
	if off {
		n.disabled = true
		ia = b.Interact(input.CursorNotAllowed)
	} else {
		ia = b.Focusable(input.CursorDefault)
	}
	n.setRole(a11y.RoleGroup).setName(name)
	if w.multi != nil {
		n.stateIf(true, a11y.Multiselectable)
	}
	if !off && ia.Focused {
		for _, k := range c.KeysFor(n.id) {
			if !k.Down {
				continue
			}
			switch k.Key {
			case input.KeyUp, input.KeyDown, input.KeyLeft, input.KeyRight, input.KeyEnter, input.KeySpace:
				if len(st.path) == 0 && (k.Key == input.KeyLeft || k.Key == input.KeyRight) {
					continue
				}
				w.key(k.Key)
			}
		}
	}
	w.loadExpanded()
	c.body(n, func() {
		Disabled(c, off, func() { w.columnsView(name) })
	})
	if w.multi == nil && w.val != nil {
		p.ps.synced = casKey(*w.val) // picks made by the keys or the columns
	}
	if c.a11yOn {
		act := w.activeID
		n.fill(semFunc(func(s *a11y.Node) { s.ActiveDescendant = a11y.NodeID(act) }))
	}
	if p.border {
		b.Bg(th.BgContainer).Radius(th.RadiusLG).Border(1, th.BorderSecondary).Clip()
	}
	if off {
		b.Opacity(0.55)
	}
	if ia.FocusVisible {
		rad := th.RadiusLG
		b.PaintOver(func(l *paint.List, r geom.Rect) { drawFocusRing(l, th, r, geom.Rad(rad)) })
	}
}
