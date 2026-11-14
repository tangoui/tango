package ui

import (
	"image"

	"github.com/tangoui/tango/ui/a11y"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// UploadFile is an entry of an Upload list. Status is "uploading", "done"
// or "error"; Percent (0..100) is shown while uploading. Files the Upload
// adds without Action / CustomRequest are "done" (accepted locally, see
// CustomRequest).
type UploadFile struct {
	Name    string
	Size    int64
	Status  string
	Percent float64
	Thumb   image.Image

	// Set for files the Upload added itself (Picker, dropped files):
	Path     string // local path of the file
	UID      int64  // identifies the file while it uploads (0 = none)
	Err      string // why the upload failed
	Response []byte // body of the server's reply to Action
}

const (
	upText = iota
	upCard
	upDrag
)

// UploadW shows an upload trigger and the file list. The UI layer never
// opens a file dialog itself: OnPick (or Picked) tells the host to show one
// (e.g. with the dialog package) and append the chosen files to the list.
type UploadW struct {
	_ [0]func()

	c         *Context
	n         *node
	files     *[]UploadFile
	kind      int
	onPick    func()
	maxCount  int
	disabled  bool
	dragOver  bool
	loc       *Locale // captured at build; paint runs outside ConfigProvider
	tip       string
	btnText   string
	title     string
	hint      string
	width     float32
	picked    bool
	removed   int
	previewed int
	z         *lazy

	accept   string                                                    // Accept
	maxSize  int64                                                     // MaxSize (bytes)
	before   func(f UploadFile) error                                  // BeforeUpload
	picker   func(accept string, multiple, dir bool) ([]string, error) // Picker
	multiple bool                                                      // Multiple
	dir      bool                                                      // Directory
	req      upRequest                                                 // Action, Headers ...
	st       *upState
	added    []UploadFile
	th       *theme.Theme // Upload Alias applied
	tk       *theme.UploadToken
}

// Upload adds an upload area bound to *files.
func Upload(c *Context, files *[]UploadFile) *UploadW {
	loc := c.Locale()
	w := &UploadW{c: c, files: files, removed: -1, previewed: -1, loc: loc, btnText: loc.Upload.Button, title: loc.Upload.DragTitle}
	w.th = c.ThemeFor(theme.CompUpload)
	w.tk = &w.th.Components.Upload
	if c.Theme.Flavor == theme.Element {
		w.title = loc.Upload.DropPrefix
	}
	w.n = c.add("upload", "")
	w.n.ln.Dir = layout.Column
	w.st = State[upState](c, w.n.id, nil)
	w.configDefaults()
	w.z = c.lazy(w.n, w.build)
	return w
}

// PictureCard shows files as thumbnail cards with a "+" card trigger.
func (w *UploadW) PictureCard() *UploadW { w.kind = upCard; return w }

// Dragger shows a large dashed drop area as the trigger.
func (w *UploadW) Dragger() *UploadW { w.kind = upDrag; return w }

// OnPick sets the callback run when the user activates the trigger.
func (w *UploadW) OnPick(fn func()) *UploadW { w.onPick = fn; return w }

// MaxCount limits the number of files; the trigger is disabled (cards:
// hidden) once reached.
func (w *UploadW) MaxCount(n int) *UploadW { w.maxCount = n; return w }

// Tip shows a help line under the trigger.
func (w *UploadW) Tip(s string) *UploadW { w.tip = s; return w }

// ButtonText sets the text of the default button trigger.
func (w *UploadW) ButtonText(s string) *UploadW { w.btnText = s; return w }

// DragText sets the main text and hint of the dragger.
func (w *UploadW) DragText(title, hint string) *UploadW { w.title, w.hint = title, hint; return w }

// DragOver highlights the dragger while the host reports a file drag over
// the window.
func (w *UploadW) DragOver(on bool) *UploadW { w.dragOver = on; return w }

// Width fixes the width (text list and dragger).
func (w *UploadW) Width(px float32) *UploadW { w.n.ln.Width = px; return w }

// Disabled disables picking and removing.
func (w *UploadW) Disabled(on bool) *UploadW {
	if on {
		w.disabled = true
	}
	return w
}

// Picked reports that the trigger was activated this frame.
func (w *UploadW) Picked() bool { w.z.run(); return w.picked }

// Removed returns the index of the file removed this frame, or -1.
func (w *UploadW) Removed() int { w.z.run(); return w.removed }

// Previewed returns the index of the card whose preview icon was clicked, or -1.
func (w *UploadW) Previewed() int { w.z.run(); return w.previewed }

// Changed reports a removal this frame.
func (w *UploadW) Changed() bool { w.z.run(); return w.removed >= 0 }

func (w *UploadW) isDisabled() bool { return w.disabled || w.n.disabled }

func (w *UploadW) full() bool { return w.maxCount > 0 && len(*w.files) >= w.maxCount }

func (w *UploadW) pick() {
	if w.isDisabled() || w.full() {
		return
	}
	w.picked = true
	if w.onPick != nil {
		w.onPick()
	}
	w.startPick()
	w.c.invalid = true
}

// remove schedules removal of file i; it is applied after the list was
// built so indices stay valid while iterating.
func (w *UploadW) remove(i int) {
	if w.isDisabled() || i < 0 || i >= len(*w.files) {
		return
	}
	w.removed = i
	w.cancelJob((*w.files)[i].UID)
	w.c.invalid = true
}

func (w *UploadW) applyRemove() {
	if i := w.removed; i >= 0 && i < len(*w.files) {
		*w.files = append((*w.files)[:i:i], (*w.files)[i+1:]...)
	}
}

func (w *UploadW) build() {
	c, th, tk := w.c, w.th, w.tk
	el := th.Flavor == theme.Element
	dis := w.isDisabled()
	defer w.applyRemove()
	w.sync()
	switch w.kind {
	case upCard:
		w.n.ln.NoStretch = true
		Row(c, func() {
			for i := range *w.files {
				i := i
				Key(c, i, func() { w.card(i) })
			}
			if !w.full() {
				w.cardTrigger()
			}
		}, layout.Gap(8), layout.WrapItems()).Semantics(a11y.RoleList, "")
	case upDrag:
		w.dragger()
	default:
		w.n.ln.NoStretch = w.n.ln.Width == 0
		b := Button(c, w.btnText).Disabled(dis || w.full())
		if el {
			b.Primary()
		} else {
			b.Icon(icon.Upload)
		}
		if b.Clicked() {
			w.pick()
		}
	}
	if w.tip != "" {
		Text(c, w.tip).Size(12).LineHeight(20).Color(tk.TipColor).Layout(layout.Margin(geom.Insets{Top: 7}))
	}
	w.rejections()
	if w.kind != upCard && len(*w.files) > 0 {
		Column(c, func() {
			for i := range *w.files {
				i := i
				Key(c, i, func() { w.listItem(i) })
			}
		}, layout.Gap(0), layout.Margin(geom.Insets{Top: 8}), layout.MinW(260)).Semantics(a11y.RoleList, "")
	}
}

// ---------------------------------------------------------------------------
// Text list

func (w *UploadW) listItem(i int) {
	c, th, tk := w.c, w.th, w.tk
	el := th.Flavor == theme.Element
	f := (*w.files)[i]
	n := c.add("up-item", f.Name)
	shad := th.Flavor == theme.StyleShadcn
	lineH := tk.ItemHeight
	h := lineH
	if f.Status == "uploading" {
		h += 10
		if shad {
			h = lineH // the bar sits inside the bordered row
		}
	}
	n.ln.Height = h
	if !el {
		n.ln.Margin.Top = 8
		if i == 0 {
			n.ln.Margin.Top = 0
		}
	} else if !shad {
		n.ln.Margin.Top = 4
	}
	n.setRole(a11y.RoleListItem)
	w.upItemSem(n, f)
	ia := c.interact(n, false, input.CursorDefault)
	rp := float32(0) // right inset of the action buttons
	if shad {
		rp = 6
	}
	delRect := func(r geom.Rect) geom.Rect {
		return geom.Rect{X: r.Right() - 26 - rp, Y: r.Y, W: 26, H: lineH}
	}
	var del, retry Interaction
	canRetry := w.canRetry(f)
	if !w.isDisabled() {
		c.body(n, func() {
			if canRetry {
				retry = w.upAction("up-retry", w.loc.Retry, layout.Abs(layout.Unset, 0, 26+rp, layout.Unset), 26, lineH)
			}
			del = w.upAction("up-remove", w.loc.RemoveItem(f.Name), layout.Abs(layout.Unset, 0, rp, layout.Unset), 26, lineH)
		})
	}
	if del.Clicked {
		w.remove(i)
	}
	if retry.Clicked {
		w.retry(i)
	}
	if shad {
		n.paint = func(l *paint.List, r geom.Rect) {
			w.shadUpItem(l, r, f, ia, del, retry, canRetry)
		}
		return
	}
	n.paint = func(l *paint.List, r geom.Rect) {
		// Laid out in LTR terms and mirrored in RTL (mx / mr): file icon
		// and name at the inline start, retry / remove at the inline end
		// (over the action nodes, whose Abs Right offsets mirror too).
		rtl := c.IsRTL()
		mx := func(x, w float32) float32 { return dtMirror(rtl, r, x, w) }
		mr := func(rr geom.Rect) geom.Rect { return dtMirrorRect(rtl, rr, r) }
		row := geom.Rect{X: r.X, Y: r.Y, W: r.W, H: lineH}
		hov := (ia.Hovered || del.Hovered || del.FocusVisible) && !w.isDisabled()
		if hov {
			l.Fill(row, geom.Rad(tk.ItemRadius), tk.ItemHoverBg)
		}
		col, icCol := tk.ItemColor, tk.ItemIconColor
		if hov && tk.ItemHoverColor.A > 0 {
			col = tk.ItemHoverColor
		}
		if f.Status == "error" {
			col, icCol = tk.ErrorColor, tk.ErrorColor
		}
		x := row.X + 4
		is := float32(14)
		if f.Status == "uploading" && !el {
			drawIconRot(l, mr(geom.Rect{X: x, Y: row.Y + (row.H-is)/2, W: is, H: is}), tk.ItemIconColor, spinAngle(c))
		} else if el {
			drawIcon(l, icon.File, mx(x, is), row.Y+(row.H-is)/2, is, icCol)
		} else {
			icon.Draw(l, icon.Paperclip, mr(geom.Rect{X: x, Y: row.Y + (row.H-is)/2, W: is, H: is}), icCol, 0)
		}
		x += is + 8
		nameW := row.Right() - x - 30
		if canRetry {
			nameW -= 26
			rr := mr(geom.Rect{X: r.Right() - 52, Y: r.Y, W: 26, H: lineH})
			rc := tk.ErrorColor
			if retry.Hovered {
				rc = tk.ErrorHoverColor
			}
			drawIcon(l, icon.Reload, rr.X+(rr.W-14)/2, rr.Y+(rr.H-14)/2, 14, rc)
		}
		if rtl {
			tw := min(entTextW(c, f.Name, th.FontSize, text.Regular), max(nameW, 0))
			entText(c, l, f.Name, mx(x, tw), row, th.FontSize, text.Regular, col, nameW)
		} else {
			entText(c, l, f.Name, x, row, th.FontSize, text.Regular, col, nameW)
		}
		dr := mr(delRect(r))
		switch {
		case hov || (f.Status == "error" && !el && !w.isDisabled()):
			dc := tk.ActionsColor
			if el {
				drawIcon(l, icon.Close, dr.X+(dr.W-12)/2, dr.Y+(dr.H-12)/2, 12, dc)
			} else {
				if f.Status == "error" {
					dc = tk.ErrorColor
				}
				drawIcon(l, icon.Delete, dr.X+(dr.W-14)/2, dr.Y+(dr.H-14)/2, 14, dc)
			}
		case el && f.Status == "done":
			drawStatusIcon(l, "success", geom.Rect{X: dr.X + (dr.W-14)/2, Y: dr.Y + (dr.H-14)/2, W: 14, H: 14}, tk.SuccessColor, th.BgContainer)
		}
		if f.Status == "uploading" {
			pr := geom.Rect{X: r.X + 4 + is + 8, Y: r.Y + lineH + 2, W: r.W - (is + 12) - 4, H: 2}
			if el {
				pct := itoaEnt(int(f.Percent+0.5)) + "%"
				pw := entTextW(c, pct, 12, text.Regular)
				pr.W -= pw + 8
				entText(c, l, pct, mx(pr.Right()+8, pw), geom.Rect{Y: pr.Y - 9, H: 20}, 12, text.Regular, th.TextSecondary, 0)
				pr.H = 6
				pr.Y -= 2
			}
			upBar(l, tk, mr(pr), f.Percent, rtl)
		}
	}
}

// upAction adds an icon button of a file entry (remove, preview) as a node
// over the icon its parent paints (absolute position pos, size wd×ht); it
// is focusable so the file list can be operated with the keyboard.
func (w *UploadW) upAction(key, name string, pos layout.Option, wd, ht float32) Interaction {
	c := w.c
	n := c.add(key, name)
	flexbox.Apply(&n.ln.Style, pos)
	n.ln.Width, n.ln.Height = wd, ht
	n.setRole(a11y.RoleButton)
	ia := c.interact(n, true, input.CursorPointer)
	n.over = func(l *paint.List, r geom.Rect) {
		if ia.FocusVisible {
			entFocusRing(c, l, r, 4)
		}
	}
	return ia
}

// upItemSem describes a file entry: its status and upload progress.
func (w *UploadW) upItemSem(n *node, f UploadFile) {
	if !w.c.a11yOn {
		return
	}
	loc := w.loc
	n.fill(semFunc(func(s *a11y.Node) {
		switch f.Status {
		case "uploading":
			s.Description = loc.Upload.Uploading
			s.States |= a11y.Busy
			rangeSem(s, f.Percent, 0, 100, 0)
			s.Value = itoaEnt(int(f.Percent+0.5)) + "%"
		case "error":
			s.Description = loc.Error
			if f.Err != "" {
				s.Description += ": " + f.Err
			}
		}
	}))
}

// upBar draws a progress bar filling from its inline start (the right
// edge when rtl).
func upBar(l *paint.List, tk *theme.UploadToken, r geom.Rect, pct float64, rtl bool) {
	pct = max(0, min(100, pct))
	l.Fill(r, geom.Rad(r.H/2), tk.ProgressTrailColor)
	fill := geom.Rect{X: r.X, Y: r.Y, W: r.W * float32(pct/100), H: r.H}
	if rtl {
		fill.X = r.Right() - fill.W
	}
	l.Fill(fill, geom.Rad(r.H/2), tk.ProgressColor)
}

// ---------------------------------------------------------------------------
// Picture cards

func (w *UploadW) cardSize() float32 { return w.tk.PictureCardSize }

func upCardIcons(r geom.Rect) (eye, del geom.Rect) {
	ics := upCardIconRow(r, 2)
	return ics[0], ics[1]
}

// upCardIconRow lays out n 20px mask icons centred on card r, 12px apart
// (preview, retry, remove).
func upCardIconRow(r geom.Rect, n int) []geom.Rect {
	const sz, gap = 20, 12
	w := float32(n)*sz + float32(n-1)*gap
	x, y := r.X+(r.W-w)/2, r.Y+r.H/2-sz/2
	out := make([]geom.Rect, n)
	for i := range out {
		out[i] = geom.Rect{X: x + float32(i)*(sz+gap), Y: y, W: sz, H: sz}
	}
	return out
}

func (w *UploadW) card(i int) {
	c, th, tk := w.c, w.th, w.tk
	el := th.Flavor == theme.Element
	f := (*w.files)[i]
	n := c.add("up-card", f.Name)
	s := w.cardSize()
	n.ln.Width, n.ln.Height = s, s
	n.setRole(a11y.RoleListItem)
	w.upItemSem(n, f)
	ia := c.interact(n, false, input.CursorPointer)
	n.aKbd = true // its preview / remove buttons are focusable
	var eyeIA, delIA, retryIA Interaction
	canRetry := w.canRetry(f) && !w.isDisabled()
	if f.Status != "uploading" {
		// Buttons over the icons of the hover mask (hit area grown by 2px
		// as before): preview, retry (failed uploads), remove.
		cnt := 1
		if canRetry {
			cnt++
		}
		if !w.isDisabled() {
			cnt++
		}
		ics := upCardIconRow(geom.Rect{W: s, H: s}, max(2, cnt))
		at := func(r geom.Rect) layout.Option { return layout.Abs(r.X-2, r.Y-2, layout.Unset, layout.Unset) }
		c.body(n, func() {
			k := 0
			eyeIA = w.upAction("up-preview", w.loc.Upload.Preview, at(ics[k]), ics[k].W+4, ics[k].H+4)
			k++
			if canRetry {
				retryIA = w.upAction("up-retry", w.loc.Retry, at(ics[k]), ics[k].W+4, ics[k].H+4)
				k++
			}
			if !w.isDisabled() {
				delIA = w.upAction("up-remove", w.loc.RemoveItem(f.Name), at(ics[k]), ics[k].W+4, ics[k].H+4)
			}
		})
	}
	if delIA.Clicked {
		w.remove(i)
	}
	if retryIA.Clicked {
		w.retry(i)
	}
	if eyeIA.Clicked {
		w.previewed = i
		c.invalid = true
	}
	mask := func() bool { return ia.Hovered || eyeIA.FocusVisible || delIA.FocusVisible || retryIA.FocusVisible }
	n.paint = func(l *paint.List, r geom.Rect) {
		rad := geom.Rad(tk.CardRadius)
		border := tk.PictureCardBorder
		if f.Status == "error" {
			border = tk.ErrorColor
		}
		l.Fill(r, rad, tk.PictureCardBg)
		defer l.Stroke(r, rad, 1, border)
		in := r
		irad := rad
		if !el {
			in = r.Inset(geom.All(8))
			irad = geom.Rad(th.RadiusSM)
		}
		switch {
		case f.Status == "uploading":
			l.Fill(in, irad, tk.PictureCardInnerBg)
			entText(c, l, w.loc.Upload.Uploading, in.X+(in.W-entTextW(c, w.loc.Upload.Uploading, th.FontSize, text.Regular))/2, geom.Rect{Y: in.Y + in.H/2 - 18, H: 22}, th.FontSize, text.Regular, th.TextTertiary, 0)
			upBar(l, tk, geom.Rect{X: in.X + 4, Y: in.Y + in.H/2 + 10, W: in.W - 8, H: 2}, f.Percent, c.IsRTL())
			return
		case f.Thumb != nil:
			l.DrawImage(f.Thumb, in, irad)
		default:
			l.Fill(in, irad, tk.PictureCardInnerBg)
			gc := th.TextQuaternary
			if f.Status == "error" {
				gc = tk.ErrorColor
			}
			gs := in.W * 0.32
			icon.Draw(l, icon.Picture, geom.Rect{X: in.X + (in.W-gs)/2, Y: in.Y + (in.H-gs)/2 - 8, W: gs, H: gs}, gc, 0)
			nc := th.TextTertiary
			if f.Status == "error" {
				nc = tk.ErrorColor
			}
			nw := min(entTextW(c, f.Name, 12, text.Regular), in.W-12)
			entText(c, l, f.Name, in.X+(in.W-nw)/2, geom.Rect{Y: in.Y + in.H/2 + gs/2 - 4, H: 20}, 12, text.Regular, nc, in.W-12)
		}
		if tk.SuccessColor.A > 0 && f.Status == "done" && !mask() {
			// Success corner ribbon (top inline-end corner).
			if c.IsRTL() {
				p := paint.NewPath().MoveTo(r.X+40, r.Y).LineTo(r.X+rad.TL, r.Y).QuadTo(r.X, r.Y, r.X, r.Y+rad.TL).LineTo(r.X, r.Y+40).Close()
				l.Path(p, tk.SuccessColor)
				drawIcon(l, icon.Check, r.X+5, r.Y+5, 12, th.TextOnPrimary)
			} else {
				p := paint.NewPath().MoveTo(r.Right()-40, r.Y).LineTo(r.Right()-rad.TR, r.Y).QuadTo(r.Right(), r.Y, r.Right(), r.Y+rad.TR).LineTo(r.Right(), r.Y+40).Close()
				l.Path(p, tk.SuccessColor)
				drawIcon(l, icon.Check, r.Right()-17, r.Y+5, 12, th.TextOnPrimary)
			}
		}
		if mask() {
			l.Fill(in, irad, tk.MaskBg)
			cnt := 1
			if canRetry {
				cnt++
			}
			if !w.isDisabled() {
				cnt++
			}
			ics := upCardIconRow(r, max(2, cnt))
			if c.IsRTL() { // in the order of the action nodes (mirrored by the layout)
				for k := range ics {
					ics[k] = mirrorRect(ics[k], r)
				}
			}
			white := tk.MaskColor
			k := 0
			drawIcon(l, icon.Eye, ics[k].X+2, ics[k].Y+2, 16, white)
			k++
			if canRetry {
				drawIcon(l, icon.Reload, ics[k].X+2, ics[k].Y+2, 16, white)
				k++
			}
			if !w.isDisabled() {
				drawIcon(l, icon.Delete, ics[k].X+2, ics[k].Y+2, 16, white)
			}
		}
	}
}

func (w *UploadW) cardTrigger() {
	c, th, tk := w.c, w.th, w.tk
	el := th.Flavor == theme.Element
	n := c.add("up-trigger", w.loc.Upload.Trigger)
	n.setRole(a11y.RoleButton)
	s := w.cardSize()
	n.ln.Width, n.ln.Height = s, s
	dis := w.isDisabled()
	cur := input.CursorPointer
	if dis {
		n.disabled, cur = true, input.CursorNotAllowed
	}
	ia := c.interact(n, true, cur)
	if ia.Clicked {
		w.pick()
	}
	if th.Flavor == theme.StyleShadcn {
		n.paint = func(l *paint.List, r geom.Rect) { w.shadUpTrigger(l, r, ia, dis) }
		return
	}
	n.paint = func(l *paint.List, r geom.Rect) {
		rad := geom.Rad(tk.CardRadius)
		border := tk.TriggerBorderColor
		if ia.Hovered && !dis {
			border = tk.TriggerHoverBorderColor
		}
		bg := tk.TriggerBg
		if ia.Hovered && !dis && tk.TriggerHoverBg.A > 0 {
			bg = tk.TriggerHoverBg
		}
		if dis {
			bg = tk.DisabledBg
		}
		l.Fill(r, rad, bg)
		l.StrokeDashed(r, rad, 1, border, []float32{4, 3})
		col := tk.TriggerColor
		if dis {
			col = th.TextDisabled
		}
		if el {
			drawIcon(l, icon.Plus, r.X+(r.W-28)/2, r.Y+(r.H-28)/2, 28, col)
		} else {
			drawIcon(l, icon.Plus, r.X+(r.W-14)/2, r.Y+r.H/2-22+4, 14, col)
			tw := entTextW(c, w.loc.Upload.Trigger, th.FontSize, text.Regular)
			entText(c, l, w.loc.Upload.Trigger, r.X+(r.W-tw)/2, geom.Rect{Y: r.Y + r.H/2 + 1, H: 22}, th.FontSize, text.Regular, col, 0)
		}
		if ia.FocusVisible {
			entFocusRing(c, l, r, rad.TL)
		}
	}
}

// ---------------------------------------------------------------------------
// Dragger

func (w *UploadW) dragger() {
	if w.isDisabled() || w.full() {
		w.draggerBox()
		return
	}
	// Files dropped on the dragger (Runner.DropFiles) are added like
	// picked ones; the zone's own highlight is off, the dragger paints its
	// drag-over state.
	z := DropZone(w.c, func(over bool) {
		w.dragOver = w.dragOver || over
		w.draggerBox()
	}).Highlight(false)
	if paths := z.Dropped(); paths != nil {
		w.addPaths(paths)
	}
}

func (w *UploadW) draggerBox() {
	c, th, tk := w.c, w.th, w.tk
	el := th.Flavor == theme.Element
	n := c.add("up-dragger", "dragger")
	name := w.title
	if el {
		name += w.loc.Upload.Button // painted right after the prefix
	}
	n.setRole(a11y.RoleButton).setName(name)
	if c.a11yOn && w.hint != "" {
		hint := w.hint
		n.fill(semFunc(func(s *a11y.Node) { s.Description = hint }))
	}
	dis := w.isDisabled() || w.full()
	n.ln.Height = tk.DraggerHeight
	n.ln.Measure = func(maxW, _ float32) geom.Size { return geom.Size{W: 360, H: n.ln.Height} }
	cur := input.CursorPointer
	if dis {
		n.disabled, cur = true, input.CursorNotAllowed
	}
	ia := c.interact(n, true, cur)
	if ia.Clicked {
		w.pick()
	}
	if th.Flavor == theme.StyleShadcn {
		n.paint = func(l *paint.List, r geom.Rect) { w.shadUpDragger(l, r, ia, dis) }
		return
	}
	n.paint = func(l *paint.List, r geom.Rect) {
		rad := geom.Rad(tk.CardRadius)
		bg, border := w.draggerLook(ia, dis)
		l.Fill(r, rad, bg)
		l.StrokeDashed(r, rad, 1, border, []float32{4, 3})
		if el {
			is := float32(64)
			icon.Draw(l, icon.CloudUpload, geom.Rect{X: r.X + (r.W-is)/2, Y: r.Y + 34, W: is, H: is}, tk.DraggerIconColor, 0)
			tcol := tk.DraggerTitleColor
			if dis {
				tcol = th.TextDisabled
			}
			w1 := entTextW(c, w.title, th.FontSize, text.Regular)
			em := w.loc.Upload.Button
			w2 := entTextW(c, em, th.FontSize, text.Regular)
			x := r.X + (r.W-w1-w2)/2
			// The title reads first: at the right in RTL.
			x1, x2 := x, x+w1
			if c.IsRTL() {
				x1, x2 = x+w2, x
			}
			line := geom.Rect{Y: r.Y + 34 + is + 12, H: 22}
			entText(c, l, w.title, x1, line, th.FontSize, text.Regular, tcol, 0)
			ec := th.Primary.Base
			if dis {
				ec = th.TextDisabled
			}
			entText(c, l, em, x2, line, th.FontSize, text.Regular, ec, 0)
			if w.hint != "" {
				hw := entTextW(c, w.hint, 12, text.Regular)
				entText(c, l, w.hint, r.X+(r.W-hw)/2, geom.Rect{Y: line.Y + 26, H: 20}, 12, text.Regular, tk.DraggerHintColor, 0)
			}
			return
		}
		is := float32(48)
		ic := tk.DraggerIconColor
		tcol, hcol := tk.DraggerTitleColor, tk.DraggerHintColor
		if dis {
			ic, tcol, hcol = th.TextDisabled, th.TextDisabled, th.TextDisabled
		}
		top := r.Y + (r.H-114)/2
		drawIcon(l, icon.Inbox, r.X+(r.W-is)/2, top, is, ic)
		y := top + is + 16
		tw := entTextW(c, w.title, th.FontSizeLG, text.Regular)
		entText(c, l, w.title, r.X+max(16, (r.W-tw)/2), geom.Rect{Y: y, H: 24}, th.FontSizeLG, text.Regular, tcol, r.W-32)
		if w.hint != "" {
			hw := entTextW(c, w.hint, th.FontSize, text.Regular)
			entText(c, l, w.hint, r.X+max(16, (r.W-hw)/2), geom.Rect{Y: y + 28, H: 22}, th.FontSize, text.Regular, hcol, r.W-32)
		}
		if ia.FocusVisible {
			entFocusRing(c, l, r, rad.TL)
		}
	}
}

// draggerLook returns the background (zero: none) and dashed border color
// of the dragger: hover and drag-over states, disabled.
func (w *UploadW) draggerLook(ia Interaction, dis bool) (bg, border theme.Color) {
	tk := w.tk
	bg, border = tk.DraggerBg, tk.DraggerBorderColor
	switch {
	case dis:
		bg = tk.DisabledBg
	case w.dragOver:
		if tk.DraggerHoverBg.A > 0 {
			bg = tk.DraggerHoverBg
		}
		border = tk.DraggerDragBorderColor
	case ia.Hovered:
		if tk.DraggerHoverBg.A > 0 {
			bg = tk.DraggerHoverBg
		}
		border = tk.DraggerHoverBorderColor
	}
	return bg, border
}
