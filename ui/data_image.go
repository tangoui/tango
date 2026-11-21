package ui

import (
	"fmt"
	"image"
	"math"

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

// ---------------------------------------------------------------------------
// Scaled bitmap cache (decoding and resampling live in ui/paint)

// imgScaled caches the device-resolution bitmap of one placement.
type imgScaled struct {
	key imgKey
	out *image.RGBA
}

type imgKey struct {
	src          image.Image
	sx, sy       float64
	sw, sh       float64
	w, h         int
	rot          int
	flipX, flipY bool
}

func (cache *imgScaled) get(k imgKey, src func() *image.RGBA) *image.RGBA {
	if cache.out == nil || cache.key != k {
		cache.key = k
		cache.out = paint.Resample(src(), k.sx, k.sy, k.sw, k.sh, k.w, k.h)
	}
	return cache.out
}

// imgPlace computes for fit mode the destination rect inside r and the
// visible source rectangle of an iw×ih image.
func imgPlace(fit string, r geom.Rect, iw, ih float64) (dst geom.Rect, sx, sy, sw, sh float64) {
	sw, sh = iw, ih
	ar := iw / ih
	rr := float64(r.W) / float64(r.H)
	switch fit {
	case "fill":
		return r, 0, 0, iw, ih
	case "contain", "scale-down":
		dst = r
		if ar > rr {
			dst.H = float32(float64(r.W) / ar)
			dst.Y += (r.H - dst.H) / 2
		} else {
			dst.W = float32(float64(r.H) * ar)
			dst.X += (r.W - dst.W) / 2
		}
		if fit == "scale-down" && float64(dst.W) > iw {
			dst = geom.Rect{X: r.X + (r.W-float32(iw))/2, Y: r.Y + (r.H-float32(ih))/2, W: float32(iw), H: float32(ih)}
		}
		return dst, 0, 0, iw, ih
	}
	// cover: crop the source to the box aspect ratio, centered
	if ar > rr {
		sw = ih * rr
		sx = (iw - sw) / 2
	} else {
		sh = iw / rr
		sy = (ih - sh) / 2
	}
	return r, sx, sy, sw, sh
}

// ---------------------------------------------------------------------------
// Image widget

// ImageW displays a bitmap with loading placeholder, error fallback and an
// optional full-screen preview (Ant Design Image / Element Plus el-image).
type ImageW struct {
	_ [0]func()

	c        *Context
	n        *node
	img      image.Image
	fit      string
	radius   float32
	radSet   bool
	loading  bool
	failed   bool
	fallback image.Image
	preview  bool
	group    []image.Image
	groupIdx int
	alt      string
	loc      *Locale // captured at build; paint runs outside ConfigProvider

	// Preview behaviour (zero: the theme's ImageToken defaults).
	minScale, maxScale, step float32
	noMove                   bool // Movable(false)
	maskClose                int8 // MaskClosable: 0 theme, 1 on, -1 off
	infinite                 int8 // Infinite: 0 theme, 1 on, -1 off
	progress                 int8 // ShowProgress: 0 theme, 1 on, -1 off
	noEsc                    bool // CloseOnEscape(false)
	previewSrc               image.Image
	ia                       Interaction
	st                       *imgState
	th                       *theme.Theme
	tk                       *theme.ImageToken
}

type imgState struct {
	thumb      imgScaled
	view       imgScaled
	open       bool
	scale      float32
	angle      float64    // preview rotation target, degrees clockwise
	rotDrag    bool       // the current drag rotates (Shift) instead of panning
	center     geom.Point // preview image centre in the last frame
	flipX      bool
	flipY      bool
	idx        int
	pan        geom.Point
	last       geom.Point
	rotSrc     *image.RGBA
	rotKey     imgKey
	overlay    ID
	overlayPan ID
	imgRect    geom.Rect // preview image rect in the last frame
	original   bool      // Element mode "original": natural size instead of fitted
}

// Image adds an image; img may be nil (shows the placeholder). The default
// size is the image's pixel size (200×200 when unknown); use Width/Height.
func Image(c *Context, img image.Image) *ImageW {
	w := &ImageW{c: c, img: img, fit: "fill", loc: c.Locale(), th: c.ThemeFor(theme.CompImage)}
	w.tk = &w.th.Components.Image
	// ConfigProvider.Defaults("Image", ...) keys: "preview" (bool),
	// "fit" (Element fit: "fill" | "contain" | "cover" | "none" | "scale-down").
	w.preview = Prop(c, "Image", "preview", false)
	w.fit = Prop(c, "Image", "fit", w.fit)
	w.n = c.add("image", "")
	w.n.ln.NoStretch = true
	w.n.ln.AlignSelf = layout.AlignAuto
	w.n.ln.Measure = w.measure
	w.n.paint = w.paint
	w.n.setRole(a11y.RoleImage).fill(w)
	w.st = State[imgState](c, w.n.id, func() imgState { return imgState{scale: 1} })
	c.lazy(w.n, w.finish)
	return w
}

// Width / Height fix the box size in px (the other side follows the image
// aspect ratio when only one is set).
func (w *ImageW) Width(px float32) *ImageW  { w.n.ln.Width = px; return w }
func (w *ImageW) Height(px float32) *ImageW { w.n.ln.Height = px; return w }

// Size fixes both sides.
func (w *ImageW) Size(wpx, hpx float32) *ImageW {
	w.n.ln.Width, w.n.ln.Height = wpx, hpx
	return w
}

// Fit sets object-fit: "fill" (default), "contain", "cover", "scale-down".
func (w *ImageW) Fit(f string) *ImageW { w.fit = f; return w }

// Radius rounds the corners.
func (w *ImageW) Radius(px float32) *ImageW { w.radius, w.radSet = px, true; return w }

// Loading shows the placeholder instead of the image.
func (w *ImageW) Loading(on bool) *ImageW { w.loading = on; return w }

// Failed shows the error fallback (broken picture / "加载失败").
func (w *ImageW) Failed(on bool) *ImageW { w.failed = on; return w }

// Fallback sets the image shown when loading failed (Ant `fallback`).
func (w *ImageW) Fallback(img image.Image) *ImageW { w.fallback = img; return w }

// Alt sets the accessible label (also used by Driver.Find).
func (w *ImageW) Alt(s string) *ImageW { w.alt = s; w.n.label = s; return w }

// A11yLabel sets the accessible name (like Alt, without changing the
// Driver.Find label).
func (w *ImageW) A11yLabel(s string) *ImageW { w.alt = s; w.n.setName(s); return w }

// altText is the accessible name: Alt / A11yLabel or the generic "image".
func (w *ImageW) altText() string {
	if w.alt != "" {
		return w.alt
	}
	return w.loc.A11y.Image
}

func (w *ImageW) semantics(s *a11y.Node) {
	if s.Name == "" {
		s.Name = w.loc.A11y.Image
	}
	if w.loading && w.showing() == nil {
		s.States |= a11y.Busy
	}
	if w.failed && w.showing() == nil && s.Description == "" {
		s.Description = w.loc.LoadFailed
	}
	if w.preview && w.showing() != nil {
		s.States |= a11y.HasPopup
		if w.st.open {
			s.States |= a11y.Expanded
		}
	}
}

// Preview enables click-to-preview.
func (w *ImageW) Preview(on bool) *ImageW { w.preview = on; return w }

// PreviewGroup previews imgs (this image is imgs[index]) with prev/next
// navigation and a counter. An index out of range is clamped.
func (w *ImageW) PreviewGroup(imgs []image.Image, index int) *ImageW {
	w.preview, w.group, w.groupIdx = true, imgs, max(0, min(index, len(imgs)-1))
	return w
}

// PreviewSrc shows img in the preview instead of the image itself (Ant
// preview.src: a larger original behind a thumbnail).
func (w *ImageW) PreviewSrc(img image.Image) *ImageW { w.previewSrc, w.preview = img, true; return w }

// OpenPreview opens the preview now (Ant preview.open); call it once per
// action, like Rotate. PreviewOpen reports the state.
func (w *ImageW) OpenPreview() *ImageW {
	w.preview = true
	if !w.st.open && w.showing() != nil {
		w.openPreview()
	}
	return w
}

// MinScale and MaxScale bound the preview zoom (Ant minScale 1 / maxScale
// 50; Element min-scale 0.2 / max-scale 7; the theme picks the default).
func (w *ImageW) MinScale(k float32) *ImageW { w.minScale = max(0.01, k); return w }
func (w *ImageW) MaxScale(k float32) *ImageW { w.maxScale = max(0.01, k); return w }

// ScaleStep sets the zoom step of the preview buttons: each step scales
// by 1+step (Ant scaleStep 0.5; Element zoom-rate 1.2 = step 0.2).
func (w *ImageW) ScaleStep(step float32) *ImageW { w.step = max(0.01, step); return w }

// Movable(false) stops dragging the preview image around (Ant
// preview.movable, default true).
func (w *ImageW) Movable(on bool) *ImageW { w.noMove = !on; return w }

// MaskClosable sets whether a click on the preview backdrop closes it
// (Ant mask.closable, default true; Element hide-on-click-modal, default
// false; the theme picks the default).
func (w *ImageW) MaskClosable(on bool) *ImageW { w.maskClose = tri(on); return w }

// Infinite sets whether the group preview wraps from the last image to
// the first (Element infinite, default true; Ant stops at the ends).
func (w *ImageW) Infinite(on bool) *ImageW { w.infinite = tri(on); return w }

// ShowProgress shows the "2 / 5" counter of a group preview (Ant always,
// Element show-progress, default false; the theme picks the default).
func (w *ImageW) ShowProgress(on bool) *ImageW { w.progress = tri(on); return w }

// CloseOnEscape(false) keeps the preview open on Escape (Element
// close-on-press-escape, default true).
func (w *ImageW) CloseOnEscape(on bool) *ImageW { w.noEsc = !on; return w }

// tri maps a bool option to 1 / -1 (0 stays "the theme's default").
func tri(on bool) int8 {
	if on {
		return 1
	}
	return -1
}

// triOr resolves a tri-state option against the default def.
func triOr(v int8, def bool) bool {
	if v == 0 {
		return def
	}
	return v > 0
}

// scales returns the preview zoom bounds and step.
func (w *ImageW) scales() (lo, hi, step float32) {
	lo, hi, step = w.tk.PreviewMinScale, w.tk.PreviewMaxScale, w.tk.PreviewScaleStep
	if w.minScale > 0 {
		lo = w.minScale
	}
	if w.maxScale > 0 {
		hi = w.maxScale
	}
	if w.step > 0 {
		step = w.step
	}
	return lo, max(lo, hi), step
}

// openPreview opens the preview at the image's group index.
func (w *ImageW) openPreview() {
	st := w.st
	st.open, st.scale, st.angle, st.flipX, st.flipY, st.pan = true, 1, 0, false, false, geom.Point{}
	st.idx, st.original = w.groupIdx, false
	w.c.invalid = true
}

// Layout applies layout options.
func (w *ImageW) Layout(o ...layout.Option) *ImageW { flexbox.Apply(&w.n.ln.Style, o...); return w }

// Rotate turns the open preview by deg degrees (clockwise, any angle;
// relative to its current rotation). Call it once per action, e.g. from a
// button's Clicked branch. The toolbar's rotate buttons step by 90°, and
// Shift-dragging the preview image rotates it freely.
func (w *ImageW) Rotate(deg float64) *ImageW {
	if w.st.open {
		w.st.angle += deg
		w.c.invalid = true
	}
	return w
}

// PreviewAngle returns the preview rotation in degrees (clockwise).
func (w *ImageW) PreviewAngle() float64 { return w.st.angle }

// PreviewScale returns the preview zoom factor (1 = fitted size).
func (w *ImageW) PreviewScale() float32 { return w.st.scale }

// PreviewOpen reports whether the preview overlay is open.
func (w *ImageW) PreviewOpen() bool { return w.st.open }

// ID returns the widget ID.
func (w *ImageW) ID() ID { return w.n.id }

func (w *ImageW) measure(maxW, maxH float32) geom.Size {
	iw, ih := float32(200), float32(200)
	if w.img != nil {
		b := w.img.Bounds()
		iw, ih = float32(b.Dx()), float32(b.Dy())
	}
	ln := &w.n.ln
	switch {
	case ln.Width > 0 && ln.Height == 0:
		return geom.Size{W: ln.Width, H: ln.Width * ih / iw}
	case ln.Height > 0 && ln.Width == 0:
		return geom.Size{W: ln.Height * iw / ih, H: ln.Height}
	}
	return geom.Size{W: iw, H: ih}
}

func (w *ImageW) showing() image.Image {
	if w.loading {
		return nil
	}
	if w.failed {
		return w.fallback
	}
	return w.img
}

func (w *ImageW) rad() geom.Radii {
	if w.radSet {
		return geom.Rad(w.radius)
	}
	return geom.Radii{}
}

func (w *ImageW) finish() {
	c := w.c
	if w.preview && w.showing() != nil {
		w.ia = c.interact(w.n, true, input.CursorPointer)
		if w.ia.Clicked && !w.n.disabled {
			w.openPreview()
		}
	}
	w.previewOverlay()
}

func (w *ImageW) paint(l *paint.List, r geom.Rect) {
	c, th, tk := w.c, w.th, w.tk
	el := th.Flavor == theme.Element
	rad := w.rad()
	img := w.showing()
	if img == nil {
		// Placeholder (loading) or fallback (error) box. shadcn pulses the
		// loading box (animate-pulse) instead of Ant's shimmer sweep.
		if w.loading && th.Flavor == theme.StyleShadcn {
			l.Opacity(shadPulse(c))
			defer l.Pop()
		}
		l.Fill(r, rad, tk.PlaceholderBg)
		if el {
			s := w.loc.Loading
			if w.failed {
				s = w.loc.LoadFailed
			}
			lay := c.dataShape(s, th.FontSize, text.Regular)
			dataText(l, lay, r.X+(r.W-lay.Width)/2, r.Y+(r.H-lay.Height)/2, tk.PlaceholderColor)
			return
		}
		s := min(r.W, r.H) * 0.32
		s = max(16, min(s, 48))
		ic := icon.Picture
		if w.failed {
			ic = icon.PictureOff
		}
		icon.Draw(l, ic, geom.Rect{X: r.X + (r.W-s)/2, Y: r.Y + (r.H-s)/2, W: s, H: s}, tk.PlaceholderColor, 0)
		if w.loading && !c.still() && th.Flavor != theme.StyleShadcn {
			// Shimmer sweep like Ant's Skeleton.Image.
			c.animating = true
			t := float32(c.now.UnixMilli()%1400) / 1400
			x := r.X - r.W + 2*r.W*t
			l.Clip(r, rad)
			l.Add(paint.LinearGradient{Rect: geom.Rect{X: x, Y: r.Y, W: r.W / 2, H: r.H}, P0: geom.Pt(x, r.Y), P1: geom.Pt(x+r.W/4, r.Y), From: theme.Fade(tk.PlaceholderBg, 0), To: tk.PlaceholderBg})
			l.Add(paint.LinearGradient{Rect: geom.Rect{X: x + r.W/4, Y: r.Y, W: r.W / 4, H: r.H}, P0: geom.Pt(x+r.W/4, r.Y), P1: geom.Pt(x+r.W/2, r.Y), From: tk.PlaceholderBg, To: theme.Fade(tk.PlaceholderBg, 0)})
			l.Pop()
		}
		return
	}
	b := img.Bounds()
	dst, sx, sy, sw, sh := imgPlace(w.fit, r, float64(b.Dx()), float64(b.Dy()))
	scale := c.PixelScale()
	k := imgKey{src: img, sx: sx, sy: sy, sw: sw, sh: sh, w: int(math.Round(float64(dst.W * scale))), h: int(math.Round(float64(dst.H * scale)))}
	bm := w.st.thumb.get(k, func() *image.RGBA { return paint.ToRGBA(img) })
	l.DrawImage(bm, dst, rad)
	if w.preview && !el {
		a := c.Anim(w.n.id, "hover", b2f(w.ia.Hovered), th.DurationSlow)
		if a > 0.01 {
			l.Fill(r, rad, theme.Fade(tk.PreviewMaskBg, float64(a)))
			fg := theme.Fade(tk.PreviewMaskColor, float64(a))
			lay := c.dataShape(w.loc.Image.Preview, th.FontSize, text.Regular)
			tw := 14 + 4 + lay.Width
			x := r.X + (r.W-tw)/2
			y := r.Y + (r.H-lay.Height)/2
			ix, tx := x, x+18
			if c.IsRTL() { // eye at the inline start (right)
				ix, tx = x+tw-14, x
			}
			drawIcon(l, icon.Eye, ix, y+(lay.Height-14)/2, 14, fg)
			dataText(l, lay, tx, y, fg)
		}
	}
	if w.ia.FocusVisible {
		l.Stroke(r.Expand(2), rad.Grow(2), 2, th.FocusRing)
	}
}

// ---------------------------------------------------------------------------
// Preview overlay

func (w *ImageW) previewImages() []image.Image {
	if len(w.group) > 0 {
		return w.group
	}
	if w.previewSrc != nil {
		return []image.Image{w.previewSrc}
	}
	return []image.Image{w.showing()}
}

func (w *ImageW) previewOverlay() {
	c, th, tk := w.c, w.th, w.tk
	st, loc := w.st, w.loc
	vis := c.Anim(w.n.id, "preview", b2f(st.open), th.DurationMid)
	if vis <= 0.001 {
		return
	}
	imgs := w.previewImages()
	st.idx = max(0, min(st.idx, len(imgs)-1))
	img := imgs[st.idx]
	if img == nil {
		return
	}
	el := th.Flavor == theme.Element
	imgMinScale, imgMaxScale, step := w.scales()
	infinite := triOr(w.infinite, tk.PreviewInfinite)
	// go moves to image i of the group: wrapping when Infinite, else
	// stopping at the ends.
	goImg := func(i int) {
		n := len(imgs)
		if infinite {
			i = (i%n + n) % n
		} else {
			i = max(0, min(i, n-1))
		}
		if i != st.idx {
			st.idx = i
			st.scale, st.angle, st.flipX, st.flipY, st.pan = 1, 0, false, false, geom.Point{}
			c.invalid = true
		}
	}
	ov := Overlay(c, OverlayOptions{Place: ScreenFill}, func() {
		// Left / Right switch images. Read before the toolbar buttons are
		// built: the focused button takes the frame's keys (Enter / Space).
		if st.open && len(imgs) > 1 {
			for _, k := range c.keys {
				if !k.Down {
					continue
				}
				// Arrow keys are visual: in RTL Left shows the next image.
				switch prev, next := c.inlineKey(k.Key); {
				case prev:
					goImg(st.idx - 1)
				case next:
					goImg(st.idx + 1)
				}
			}
		}
		// Drag surface for panning (below the controls).
		pan := c.add("img-pan", loc.Image.PreviewLabel)
		// The drag / wheel surface shows the image: arrow keys and the
		// toolbar buttons are its keyboard equivalents.
		pan.setRole(a11y.RoleImage).setName(w.altText())
		pan.aKbd = true
		st.overlayPan = pan.id
		flexbox.Apply(&pan.ln.Style, layout.Abs(0, 0, 0, 0))
		ia := c.interact(pan, false, input.CursorDefault)
		if ia.PressedNow {
			st.last = c.pointer
			st.rotDrag = c.pressMods.Has(input.ModShift) && st.imgRect.Contains(c.pressPos)
		}
		if c.Dragging(pan.id) && (st.rotDrag || !w.noMove) {
			if p, ok := c.Pointer(); ok {
				if st.rotDrag {
					// Rotate by the angle swept around the image centre.
					a0 := math.Atan2(float64(st.last.Y-st.center.Y), float64(st.last.X-st.center.X))
					a1 := math.Atan2(float64(p.Y-st.center.Y), float64(p.X-st.center.X))
					d := (a1 - a0) * 180 / math.Pi
					if d > 180 {
						d -= 360
					} else if d < -180 {
						d += 360
					}
					st.angle += d
				} else {
					st.pan.X += p.X - st.last.X
					st.pan.Y += p.Y - st.last.Y
				}
				st.last = p
			}
		}
		// Wheel zooms about the pointer, within [imgMinScale, imgMaxScale].
		if _, dy, ok := c.WheelFor(pan.id); ok && dy != 0 {
			k := float32(math.Pow(1.25, float64(-dy)/40))
			k = max(1/1.5, min(k, 1.5))
			ns := max(imgMinScale, min(st.scale*k, imgMaxScale))
			if ns != st.scale {
				p := c.pointer
				f := 1 - ns/st.scale
				st.pan.X += (p.X - st.center.X) * f
				st.pan.Y += (p.Y - st.center.Y) * f
				st.scale = ns
				if ns <= 1 {
					st.pan = geom.Point{} // not larger than fitted: keep it centred
				}
				c.invalid = true
			}
		}
		if ia.Clicked && ia.Clicks >= 2 && tk.PreviewDblClickZoom {
			// rc-image: a double click zooms one step, or resets.
			if st.scale == 1 {
				st.scale = min(imgMaxScale, 1+step)
			} else {
				st.scale, st.pan = 1, geom.Point{}
			}
		} else if ia.Clicked && triOr(w.maskClose, tk.PreviewMaskClosable) && !st.imgRect.Contains(c.pressPos) && c.pressPos == c.pointer {
			st.open = false // clicking the backdrop closes
		}

		btn := func(label string, ic *icon.Icon, std *icon.Icon, disabled bool, place layout.Option, size, iconSize float32, round bool) bool {
			fg, fgHover := tk.PreviewOperationColor, tk.PreviewOperationHoverColor
			if round {
				fg, fgHover = tk.PreviewSwitchColor, tk.PreviewSwitchHoverColor
			}
			n := c.add("img-op", label)
			n.ln.Width, n.ln.Height = size, size
			flexbox.Apply(&n.ln.Style, place)
			n.disabled = disabled
			n.setRole(a11y.RoleButton)
			bia := c.interact(n, true, input.CursorPointer)
			id := n.id
			n.paint = func(l *paint.List, r geom.Rect) {
				hv := c.Anim(id, "hover", b2f(bia.Hovered), th.DurationMid)
				if round && !disabled { // Ant: a disabled switch has no background
					l.Fill(r, geom.Rad(size/2), tk.PreviewOperationBg)
					if hv > 0 && tk.PreviewOperationHoverBg.A > 0 {
						l.Fill(r, geom.Rad(size/2), theme.Fade(tk.PreviewOperationHoverBg, float64(hv)))
					}
				}
				col := theme.Lerp(fg, fgHover, hv)
				if disabled {
					col = tk.PreviewOperationDisabledColor
				}
				ir := geom.Rect{X: r.X + (r.W-iconSize)/2, Y: r.Y + (r.H-iconSize)/2, W: iconSize, H: iconSize}
				if ic == nil {
					ic = std
				}
				c.paintIcon(l, ic, ir, col, 0) // prev / next mirror; rotate icons do not
				if bia.FocusVisible {
					rad := geom.Radii{}
					if round {
						rad = geom.Rad(size / 2)
					}
					drawFocusRing(l, th, r, rad)
				}
			}
			return bia.Clicked && !disabled
		}
		U := layout.Unset
		// Close button: top-end (Abs offsets are logical: top-left in RTL).
		sz, is := tk.PreviewOperationSize, tk.PreviewOperationIconSize
		cm := tk.PreviewCloseOffset
		if btn(loc.Image.ClosePreview, nil, icon.Close, false, layout.Abs(U, cm, cm, U), sz, is, true) {
			st.open = false
		}
		// Prev / next for groups: prev at the inline start, next at the end.
		if len(imgs) > 1 {
			m := tk.PreviewSwitchOffset
			mid := c.size.H/2 - sz/2
			if btn(loc.Image.Prev, nil, icon.Left, st.idx == 0 && !infinite, layout.Abs(m, mid, U, U), sz, is, true) {
				goImg(st.idx - 1)
			}
			if btn(loc.Image.Next, nil, icon.Right, st.idx == len(imgs)-1 && !infinite, layout.Abs(U, mid, m, U), sz, is, true) {
				goImg(st.idx + 1)
			}
		}
		// Toolbar.
		zoomIn := func() { st.scale = min(imgMaxScale, st.scale*(1+step)) }
		zoomOut := func() {
			st.scale = max(imgMinScale, st.scale/(1+step))
			if st.scale <= 1 {
				st.pan = geom.Point{}
			}
		}
		if el {
			Row(c, func() {
				if btn(loc.Image.ZoomOut, icon.ZoomOut, nil, st.scale <= imgMinScale, layout.Option{}, 23, 23, false) {
					zoomOut()
				}
				if btn(loc.Image.ZoomIn, icon.ZoomIn, nil, st.scale >= imgMaxScale, layout.Option{}, 23, 23, false) {
					zoomIn()
				}
				imgSep(c, tk)
				// EP toggleMode: contain (fitted) <-> original (natural
				// size), resetting the transform.
				if btn(loc.Reset, nil, icon.Fullscreen, false, layout.Option{}, 23, 23, false) {
					st.original = !st.original
					st.scale, st.angle, st.pan, st.flipX, st.flipY = 1, 0, geom.Point{}, false, false
				}
				imgSep(c, tk)
				if btn(loc.Image.RotateLeft, icon.RotateLeft, nil, false, layout.Option{}, 23, 23, false) {
					st.angle -= 90
				}
				if btn(loc.Image.RotateRight, icon.RotateRight, nil, false, layout.Option{}, 23, 23, false) {
					st.angle += 90
				}
			}, layout.Abs(c.size.W/2-140, U, U, 30), layout.W(280), layout.H(44), layout.Middle(),
				layout.JustifyContent(layout.JustifySpaceAround), layout.PadVH(0, 23)).Bg(tk.PreviewToolbarBg).Radius(22)
		} else {
			as, gap, pad := tk.PreviewActionSize, tk.PreviewActionGap, tk.PreviewActionsPadding
			tw := 6*as + 5*gap + 2*pad
			bar := Row(c, func() {
				if btn(loc.Image.FlipVertical, icon.SwapVertical, nil, false, layout.Option{}, as, is, false) {
					st.flipY = !st.flipY
				}
				if btn(loc.Image.FlipHorizontal, icon.Swap, nil, false, layout.Option{}, as, is, false) {
					st.flipX = !st.flipX
				}
				if btn(loc.Image.RotateLeft, icon.RotateLeft, nil, false, layout.Option{}, as, is, false) {
					st.angle -= 90
				}
				if btn(loc.Image.RotateRight, icon.RotateRight, nil, false, layout.Option{}, as, is, false) {
					st.angle += 90
				}
				if btn(loc.Image.ZoomOut, icon.ZoomOut, nil, st.scale <= imgMinScale, layout.Option{}, as, is, false) {
					zoomOut()
				}
				if btn(loc.Image.ZoomIn, icon.ZoomIn, nil, st.scale >= imgMaxScale, layout.Option{}, as, is, false) {
					zoomIn()
				}
			}, layout.Abs(c.size.W/2-tw/2, U, U, 32), layout.W(tw), layout.H(as), layout.Middle(), layout.Gap(gap), layout.PadVH(0, pad))
			bar.Bg(tk.PreviewToolbarBg).Radius(100)
		}
		if len(imgs) > 1 && triOr(w.progress, tk.PreviewShowProgress) {
			s := fmt.Sprintf("%d / %d", st.idx+1, len(imgs))
			n := c.add("img-count", "")
			n.setRole(a11y.RoleText).setName(s)
			flexbox.Apply(&n.ln.Style, layout.Abs(0, U, 0, tk.PreviewProgressBottom), layout.H(22))
			n.paint = func(l *paint.List, r geom.Rect) {
				lay := c.dataShape(s, th.FontSize, text.Regular)
				dataText(l, lay, r.X+(r.W-lay.Width)/2, r.Y, tk.PreviewProgressColor)
			}
		}
		if !st.open {
			c.invalid = true
		}
	})
	st.overlay = ov.n.id
	ov.Opacity(vis)
	ov.n.setRole(a11y.RoleDialog).setName(loc.Image.PreviewLabel).setState(a11y.Modal)
	if c.a11yOn {
		ov.n.fill(semFunc(dialogSem))
	}
	if st.open {
		c.TrapFocus(ov.n.id)
	}
	if st.open && !w.noEsc && c.EscapeAt(ov.ID()) {
		st.open = false
		c.invalid = true
	}
	mask := tk.PreviewBg
	ov.n.paint = func(l *paint.List, r geom.Rect) {
		l.Fill(r, geom.Radii{}, mask)
		scale := c.PixelScale()
		// Rotation eases between toolbar steps and follows a rotate drag.
		dur := th.DurationMid
		if st.rotDrag && c.Dragging(st.overlayPan) {
			dur = 0
		}
		ang := float64(c.Anim(w.n.id, "rot", float32(st.angle), dur))
		ib := img.Bounds()
		ow, oh := float32(ib.Dx()), float32(ib.Dy())
		// Base size: natural size, shrunk so the target orientation stays
		// clear of the controls.
		sn, cs := math.Sincos(st.angle * math.Pi / 180)
		sn, cs = math.Abs(sn), math.Abs(cs)
		bw := ow*float32(cs) + oh*float32(sn)
		bh := ow*float32(sn) + oh*float32(cs)
		fit := min(1, (r.W-160)/bw, (r.H-250)/bh)
		if st.original {
			fit = 1
		}
		zoom := st.scale * (0.9 + 0.1*vis)
		dw, dh := ow*fit*zoom, oh*fit*zoom
		cen := geom.Pt(r.X+r.W/2+st.pan.X, r.Y+r.H/2+st.pan.Y)
		st.center = cen
		if q := math.Round(ang / 90); math.Abs(ang-q*90) > 1e-3 {
			// Arbitrary angle: draw through a rotation; the renderer
			// samples the bitmap bilinearly.
			src := w.rotated(img, 0)
			dst := geom.Rect{X: cen.X - dw/2, Y: cen.Y - dh/2, W: dw, H: dh}
			m := geom.RotateAround(ang*math.Pi/180, cen)
			st.imgRect = m.Bounds(dst)
			l.Transform(m)
			l.DrawImage(src, dst, geom.Radii{})
			l.Pop()
			return
		} else if int(q)%2 != 0 {
			dw, dh = dh, dw
			src := w.rotated(img, int(q))
			w.drawPreview(l, r, src, cen, dw, dh, scale)
		} else {
			w.drawPreview(l, r, w.rotated(img, int(q)), cen, dw, dh, scale)
		}
	}
}

// drawPreview draws src (already oriented) as a dw×dh rect centred on cen,
// resampling only the part visible in r so deep zoom stays cheap.
func (w *ImageW) drawPreview(l *paint.List, r geom.Rect, src *image.RGBA, cen geom.Point, dw, dh, scale float32) {
	st := w.st
	sb := src.Bounds()
	iw, ih := float32(sb.Dx()), float32(sb.Dy())
	dst := geom.Rect{X: cen.X - dw/2, Y: cen.Y - dh/2, W: dw, H: dh}
	st.imgRect = dst
	visR := dst.Intersect(r)
	if visR.Empty() {
		return
	}
	sx := float64((visR.X - dst.X) / dw * iw)
	sy := float64((visR.Y - dst.Y) / dh * ih)
	sw := float64(visR.W / dw * iw)
	sh := float64(visR.H / dh * ih)
	k := imgKey{src: src, sx: sx, sy: sy, sw: sw, sh: sh, w: int(math.Round(float64(visR.W * scale))), h: int(math.Round(float64(visR.H * scale)))}
	bm := st.view.get(k, func() *image.RGBA { return src })
	l.DrawImage(bm, visR, geom.Radii{})
}

func imgSep(c *Context, tk *theme.ImageToken) {
	n := c.add("img-sep", "")
	n.aHidden = true
	n.ln.Width, n.ln.Height = 1, 22
	n.paint = func(l *paint.List, r geom.Rect) { l.Fill(r, geom.Radii{}, tk.PreviewSplitColor) }
}

// rotated returns the preview source mirrored and turned by quarter
// quarter turns (cached until they change).
func (w *ImageW) rotated(img image.Image, quarter int) *image.RGBA {
	st := w.st
	k := imgKey{src: img, rot: ((quarter % 4) + 4) % 4, flipX: st.flipX, flipY: st.flipY}
	if st.rotSrc == nil || st.rotKey != k {
		st.rotKey = k
		st.rotSrc = paint.Orient(img, k.rot, k.flipX, k.flipY)
	}
	return st.rotSrc
}
