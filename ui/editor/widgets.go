package editor

import (
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

func (w *ViewW) contentAnchorClip() geom.Rect {
	r := w.vs.text
	top := max(r.Y, w.vs.stickyRect.Bottom())
	return geom.R(r.X, top, r.W, max(0, r.Bottom()-top))
}

// HoverOffset reports the nearest UTF-8 byte offset underneath the pointer
// when it is over the view's visible text area. Gutter, sticky headers and
// overlapping widgets are excluded. Use MountContent to show content at
// that offset, retaining the offset while the pointer is inside the popup.
func (w *ViewW) HoverOffset() (int, bool) {
	p, inside := w.c.Pointer()
	if !inside || !w.ia.Hovered || w.vs.buf != w.st.Model.Buffer() || !w.contentAnchorClip().Contains(p) {
		return 0, false
	}
	row := w.pointRow(p)
	vl := w.st.lines()
	if row < 0 || row >= vl.Count() {
		return 0, false
	}
	line, within := vl.find(row)
	if within < 0 || within >= vl.rowsOf(line) {
		return 0, false
	}
	return w.OffsetAtPoint(p)
}

// MountContent builds an overlay at a document offset, for example a rich
// hover or a quick-fix affordance. It returns nil when OffsetAnchor is
// unavailable. The body and layout options control size and styling, and
// the caller controls what frames display it. Use an anchored placement
// such as ui.BottomStart, which flips above the row near the window edge.
func (w *ViewW) MountContent(offset int, place ui.Placement, body func(), opts ...layout.Option) *ui.Box {
	anchor, ok := w.OffsetAnchor(offset)
	if !ok || body == nil {
		return nil
	}
	return w.mountContent(anchor, place, body, opts...)
}

func (w *ViewW) mountContent(anchor geom.Rect, place ui.Placement, body func(), opts ...layout.Option) *ui.Box {
	return ui.Overlay(w.c, ui.OverlayOptions{Anchor: w.box.ID(), AnchorRect: anchor, Place: place, Offset: 2}, body, opts...)
}

// SuggestOptions configures a caret-anchored completion list.
type SuggestOptions struct {
	Labels   []string // one per candidate, in display order
	Selected *int     // maintained by the caller and keyboard navigation; -1 for none
	Accept   func(int)
	Dismiss  func()
	Details  func(int) // optional body for the selected candidate
	Name     string    // accessible name of the candidate list
	// Width, DetailsWidth and MaxHeight are logical pixels. Zero uses
	// 240, 300 and 240 respectively, bounded by the window size.
	Width, DetailsWidth, MaxHeight float32
}

// Suggest mounts a scrolling completion list at the caret, with an optional
// adjacent details pane. Call it during build before Changed or any anchor
// query. It processes pending input with CompletionKeys plus Page Up/Down.
// The caller owns the list lifetime and should clear it in Accept and Dismiss.
// When the caret is outside the viewport, it returns nil and leaves the keys
// to the editor. Read-only views do not display suggestions.
func (w *ViewW) Suggest(o SuggestOptions) *ui.Box {
	if len(o.Labels) == 0 || o.Selected == nil || w.opts.ReadOnly {
		return nil
	}
	if _, ok := w.offsetAnchor(w.st.Primary().Head); !ok {
		return nil
	}
	c, th := w.c, w.c.Theme
	rowH := max(1, th.ControlHeightSM)
	height := o.MaxHeight
	if height <= 0 {
		height = 240
	}
	height = min(height, max(1, c.Size().H-4))
	width := o.Width
	if width <= 0 {
		width = 240
	}
	detailsWidth := o.DetailsWidth
	if detailsWidth <= 0 {
		detailsWidth = 300
	}
	// Reserve room for both panes on narrow windows and keep the list's
	// width stable as the selection changes.
	available := max(1, c.Size().W-4)
	if o.Details != nil {
		detailsWidth = min(detailsWidth, available/2)
		available -= detailsWidth
	}
	width = min(width, available)
	var confirm func(int)
	if o.Accept != nil {
		confirm = func(i int) {
			c.SetFocus(w.box.ID())
			o.Accept(i)
			c.Invalidate()
		}
	}
	w.CompletionKeys(len(o.Labels), o.Selected, confirm, o.Dismiss)
	w.completion.page = max(1, int(height/rowH))
	anchor, ok := w.CompletionAnchor()
	if !ok || w.completion.count == 0 || w.completion.version != w.st.Snapshot().Version() {
		return nil
	}
	selected := *o.Selected
	valid := selected >= 0 && selected < len(o.Labels)
	listHeight := min(height, float32(len(o.Labels))*rowH)
	return w.mountContent(anchor, ui.BottomStart, func() {
		ui.Row(c, func() {
			list := ui.Scroll(c, func() {
				for i, label := range o.Labels {
					row := ui.Div(c, layout.H(rowH), layout.JustifyContent(layout.JustifyCenter), layout.PadVH(0, th.PaddingSM)).
						Semantics(a11y.RoleOption, label).A11yKeyboard()
					if i == selected {
						row.A11yState(a11y.Selected).Bg(th.Primary.Bg)
					}
					if confirm != nil {
						ia := row.Interact(input.CursorPointer)
						if ia.PressedNow {
							c.SetFocus(w.box.ID())
						}
						if ia.Hovered && i != selected {
							row.Bg(th.ItemHoverBg)
						}
						if ia.Clicked {
							*o.Selected = i
							version := w.st.Snapshot().Version()
							if w.onChange != nil {
								defer w.observeDocumentChanges()()
							}
							confirm(i)
							w.changed = w.changed || version != w.st.Snapshot().Version()
						}
					}
					row.Body(func() { ui.Text(c, label).Ellipsis() })
				}
			}, layout.Size(width, listHeight))
			list.Semantics(a11y.RoleListbox, o.Name)
			type selection struct {
				index       int
				height, row float32
			}
			last := ui.State[selection](c, list.ID(), nil)
			current := selection{selected, listHeight, rowH}
			_, wasOpen := c.RectOf(list.ID())
			if valid && (!wasOpen || *last != current) {
				_, y := list.Offset()
				top := float64(selected) * float64(rowH)
				list.ScrollTo(0, min(max(y, top+float64(rowH-listHeight)), top))
			}
			*last = current
			if o.Details != nil && valid {
				ui.Key(c, selected, func() {
					ui.Scroll(c, func() {
						ui.Column(c, func() { o.Details(selected) }, layout.Pad(th.PaddingSM))
					}, layout.W(detailsWidth), layout.MaxH(height))
				})
			}
		})
	}).Bg(th.BgElevated).Border(1, th.BorderSecondary).Radius(th.RadiusSM).Shadow(th.ShadowSecondary)
}
