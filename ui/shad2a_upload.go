package ui

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// shadcn/ui has no Upload; these follow its language: bordered rows with
// a muted file icon and ghost icon actions, dashed rounded-lg drop areas
// that turn bg-muted/50 under the pointer, muted-foreground hints and the
// ring color while a file is dragged over.

const shadUpRowH = 40 // file row: py-2 px-3 text-sm, border, rounded-md

// shadUpItem paints a text-list file row (shadcn).
func (w *UploadW) shadUpItem(l *paint.List, r geom.Rect, f UploadFile, ia, del, retry Interaction, canRetry bool) {
	c, th, tk := w.c, w.th, w.tk
	dis := w.isDisabled()
	rad := geom.Rad(tk.ItemRadius)
	border := tk.ItemBorderColor
	col, icCol := tk.ItemColor, tk.ItemIconColor
	if f.Status == "error" {
		border = theme.Fade(tk.ErrorColor, .5)
		col, icCol = tk.ErrorColor, tk.ErrorColor
	}
	if (ia.Hovered || del.Hovered || retry.Hovered) && !dis {
		l.Fill(r, rad, tk.ItemHoverBg) // hover:bg-muted/50
	}
	l.Stroke(r, rad, 1, border)
	// Laid out in LTR terms and mirrored in RTL (mx / mr): icon, name and
	// size from the inline start, ghost actions at the inline end.
	rtl := c.IsRTL()
	mx := func(x, w float32) float32 { return dtMirror(rtl, r, x, w) }
	mr := func(rr geom.Rect) geom.Rect { return dtMirrorRect(rtl, rr, r) }
	is := float32(16)
	x := r.X + 12
	iy := r.Y + (r.H-is)/2
	if f.Status == "uploading" {
		drawIconRot(l, mr(geom.Rect{X: x, Y: iy, W: is, H: is}), tk.ItemIconColor, spinAngle(c))
	} else {
		drawIcon(l, icon.File, mx(x, is), iy, is, icCol)
	}
	x += is + 8
	right := r.Right() - 6 - 26
	if canRetry {
		right -= 26
	}
	name := f.Name
	nameW := right - x - 8
	tr := r
	if f.Status == "uploading" {
		tr.H -= 6 // room for the progress bar
	}
	var tw float32
	if rtl {
		tw = min(entTextW(c, name, th.FontSize, text.Regular), max(nameW, 0))
		entText(c, l, name, mx(x, tw), tr, th.FontSize, text.Regular, col, nameW)
	} else {
		tw = entText(c, l, name, x, tr, th.FontSize, text.Regular, col, nameW)
	}
	if f.Size > 0 && f.Status != "error" {
		sz := formatBytes(f.Size)
		if sw := entTextW(c, sz, 12, text.Regular); x+tw+8+sw < right-8 {
			entText(c, l, sz, mx(x+tw+8, sw), tr, 12, text.Regular, th.TextTertiary, 0)
		}
	}
	if f.Status == "uploading" {
		// Progress: h-1 rounded-full bg-primary/20 under the name.
		bar := geom.Rect{X: x, Y: r.Bottom() - 9, W: right - x - 8, H: 3}
		l.Fill(mr(bar), geom.Rad(bar.H/2), tk.ProgressTrailColor)
		pct := float32(max(0, min(100, f.Percent)) / 100)
		l.Fill(mr(geom.Rect{X: bar.X, Y: bar.Y, W: bar.W * pct, H: bar.H}), geom.Rad(bar.H/2), tk.ProgressColor)
	}
	if dis {
		return
	}
	ghost := func(rr geom.Rect, ic *icon.Icon, hov bool, fg theme.Color) {
		b := geom.Rect{X: rr.X + (rr.W-24)/2, Y: rr.Y + (rr.H-24)/2, W: 24, H: 24}
		if hov {
			l.Fill(b, geom.Rad(th.RadiusSM), th.ItemHoverBg) // ghost: hover:bg-accent
		}
		drawIcon(l, ic, b.X+5, b.Y+5, 14, fg)
	}
	dr := geom.Rect{X: r.Right() - 6 - 26, Y: r.Y, W: 26, H: r.H}
	ghost(mr(dr), icon.Close, del.Hovered, tk.ActionsColor)
	if canRetry {
		ghost(mr(geom.Rect{X: dr.X - 26, Y: r.Y, W: 26, H: r.H}), icon.Reload, retry.Hovered, tk.ErrorColor)
	}
}

// shadUpTrigger paints the picture-card "+" trigger (shadcn).
func (w *UploadW) shadUpTrigger(l *paint.List, r geom.Rect, ia Interaction, dis bool) {
	c, th, tk := w.c, w.th, w.tk
	rad := geom.Rad(tk.CardRadius)
	if dis {
		l.Opacity(0.5)
		defer l.Pop()
	}
	border := tk.TriggerBorderColor
	if ia.Hovered && !dis {
		if tk.TriggerHoverBg.A > 0 {
			l.Fill(r, rad, tk.TriggerHoverBg)
		}
		border = tk.TriggerHoverBorderColor
	}
	l.StrokeDashed(r, rad, 1, border, []float32{4, 3})
	col := tk.TriggerColor
	drawIcon(l, icon.Plus, r.X+(r.W-16)/2, r.Y+r.H/2-22, 16, col)
	tw := entTextW(c, w.loc.Upload.Trigger, th.FontSize, text.Medium)
	entText(c, l, w.loc.Upload.Trigger, r.X+(r.W-tw)/2, geom.Rect{Y: r.Y + r.H/2 + 2, H: 20}, th.FontSize, text.Medium, col, 0)
	if ia.FocusVisible {
		entFocusRing(c, l, r, rad.TL)
	}
}

// shadUpDragger paints the drop area (shadcn).
func (w *UploadW) shadUpDragger(l *paint.List, r geom.Rect, ia Interaction, dis bool) {
	c, th, tk := w.c, w.th, w.tk
	rad := geom.Rad(tk.CardRadius)
	if dis {
		l.Opacity(0.5)
		defer l.Pop()
	}
	// The ring color (DraggerDragBorderColor) marks the drop target.
	bg, border := w.draggerLook(ia, false)
	if dis {
		bg, border = theme.Color{}, tk.DraggerBorderColor
	}
	if bg.A > 0 {
		l.Fill(r, rad, bg)
	}
	l.StrokeDashed(r, rad, 1, border, []float32{4, 3})
	hasHint := w.hint != ""
	block := float32(40 + 12 + 20)
	if hasHint {
		block += 4 + 16
	}
	top := r.Y + (r.H-block)/2
	// Icon in a bordered circle (size-10 rounded-full border).
	cr := geom.Rect{X: r.X + (r.W-40)/2, Y: top, W: 40, H: 40}
	l.Fill(cr, geom.Rad(20), th.BgContainer)
	l.Stroke(cr, geom.Rad(20), 1, th.BorderSecondary)
	drawIcon(l, icon.Upload, cr.X+10, cr.Y+10, 20, tk.DraggerIconColor)
	y := top + 40 + 12
	tw := entTextW(c, w.title, th.FontSize, text.Medium)
	entText(c, l, w.title, r.X+max(16, (r.W-tw)/2), geom.Rect{Y: y, H: 20}, th.FontSize, text.Medium, tk.DraggerTitleColor, r.W-32)
	if hasHint {
		hw := entTextW(c, w.hint, 12, text.Regular)
		entText(c, l, w.hint, r.X+max(16, (r.W-hw)/2), geom.Rect{Y: y + 24, H: 16}, 12, text.Regular, tk.DraggerHintColor, r.W-32)
	}
	if ia.FocusVisible {
		entFocusRing(c, l, r, rad.TL)
	}
}
