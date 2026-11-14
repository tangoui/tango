// Package icon draws vector UI icons (no icon font).
//
// An Icon is a list of Parts: SVG path data that is either filled or
// stroked, in the main color or in a secondary (two-tone) color, laid out
// in a square view box. The package's own set is designed on a 24×24 grid
// with a 1.8-unit stroke, close to Ant Design's outlined style; all path
// data in builtin.go is original to Tango.
//
// The complete Element Plus, Ant Design and Lucide (shadcn/ui) sets live
// in the subpackages element, antd and lucide. Their icons are statically
// initialized variables, so a program only links the icons it references;
// call a set's Register to make all of its icons reachable by name
// (ByName("lucide:search")).
package icon

import (
	"image"
	"image/color"
	"math"
	"sort"
	"sync"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// Icon is a vector (or bitmap, see FromImage) icon. Declare custom icons
// with a composite literal, New or FromImage:
//
//	var Branch = &icon.Icon{Name: "app:branch", Box: [4]float32{0, 0, 1024, 1024},
//		Parts: []icon.Part{{D: "M..."}}}
//
// The exported fields must not change once the icon is first drawn.
type Icon struct {
	Name string
	// Box is the view box (min x, min y, width, height) Parts are drawn
	// in, like an SVG viewBox. It is scaled uniformly to fit the target
	// square and centered. The zero value is the 24×24 grid.
	Box [4]float32
	// Width is the stroke width of stroked Parts in Box units. Zero means
	// StrokeWidth on a 24-unit grid, scaled to Box.
	Width float32
	// Parts are painted in order.
	Parts []Part
	// Dir marks a directional icon (an arrow, a chevron, text lines that
	// start at the left): it mirrors horizontally in right-to-left layouts.
	// See Directional.
	Dir bool

	img  image.Image // FromImage: the bitmap
	mask bool        // FromImage: only alpha is used, tinted with the color

	once   sync.Once
	parsed []*paint.Path

	mu    sync.Mutex
	cache map[sizeKey][]*paint.Path // renditions by size, angle, mirror and stroke
	tints map[color.NRGBA]image.Image
}

// Part is one shape of an icon.
type Part struct {
	// D is SVG path data in Box units ("M3 12H21", arcs included). It
	// is parsed on first draw and panics if malformed.
	D string
	// Stroke outlines the path at the icon's stroke width (round caps
	// and joins) instead of filling it (non-zero rule).
	Stroke bool
	// Secondary paints the part in the secondary color of a two-tone icon
	// (DrawOpts.Secondary) instead of the main color.
	Secondary bool
}

// Directional marks ic as directional, so it mirrors horizontally when
// drawn for a right-to-left layout (see DrawOpts.RTL), and returns ic. Use
// it on custom icons whose shape points along the reading direction:
//
//	next := icon.New("next", "M5 12H19M13 6L19 12L13 18", "").Directional()
//
// Call it before the icon is first drawn. Icons are not directional by
// default; clocks, spinners, media controls and symmetric shapes must not
// mirror.
func (ic *Icon) Directional() *Icon {
	ic.Dir = true
	return ic
}

// IsDirectional reports whether ic mirrors in right-to-left layouts.
func (ic *Icon) IsDirectional() bool { return ic != nil && ic.Dir }

// IsImage reports whether ic is a bitmap icon made by FromImage.
func (ic *Icon) IsImage() bool { return ic != nil && ic.img != nil }

// New makes an icon from SVG path data on the same 24×24 grid as the
// built-in set: stroke is outlined with the icon color at StrokeWidth
// (round caps and joins), fill is filled with it; either may be empty.
// The path data is parsed on first draw and panics if malformed, like the
// built-ins. Icons made with New are not registered with ByName; see
// Register.
func New(name, stroke, fill string) *Icon {
	ic := &Icon{Name: name}
	if fill != "" {
		ic.Parts = append(ic.Parts, Part{D: fill})
	}
	if stroke != "" {
		ic.Parts = append(ic.Parts, Part{D: stroke, Stroke: true})
	}
	return ic
}

// FromImage makes a bitmap icon, for example one from a plugin's file
// icon theme. With mask set only the image's alpha is used and the icon
// takes the draw color like a vector icon (a monochrome symbol); without
// it the image is drawn as is (a colored logo). The image is scaled to
// fit the target square, keeping its aspect ratio.
func FromImage(name string, img image.Image, mask bool) *Icon {
	return &Icon{Name: name, img: img, mask: mask}
}

// The registry behind ByName.
var (
	regMu sync.RWMutex
	all   = map[string]*Icon{}
)

func def(name, stroke, fill string) *Icon {
	ic := New(name, stroke, fill)
	all[name] = ic
	return ic
}

// Register makes icons reachable by ByName under their Name, replacing
// icons registered earlier under the same name. It is safe to call from
// several goroutines.
func Register(icons ...*Icon) {
	regMu.Lock()
	defer regMu.Unlock()
	for _, ic := range icons {
		if ic != nil && ic.Name != "" {
			all[ic.Name] = ic
		}
	}
}

// RegisterAs makes ic reachable by ByName under name as well as (not
// instead of) its own Name: an alias.
func RegisterAs(name string, ic *Icon) {
	if ic == nil || name == "" {
		return
	}
	regMu.Lock()
	defer regMu.Unlock()
	all[name] = ic
}

// ByName looks up a registered icon: the built-in set ("search"), icons
// passed to Register and the icon sets whose Register was called
// ("ep:search", "ant:search-outlined", "lucide:search").
func ByName(name string) *Icon {
	regMu.RLock()
	defer regMu.RUnlock()
	return all[name]
}

// Names lists all registered icon names, sorted.
func Names() []string {
	regMu.RLock()
	out := make([]string, 0, len(all))
	for k := range all {
		out = append(out, k)
	}
	regMu.RUnlock()
	sort.Strings(out)
	return out
}

func (ic *Icon) load() {
	ic.once.Do(func() {
		ic.parsed = make([]*paint.Path, len(ic.Parts))
		for i, p := range ic.Parts {
			ic.parsed[i] = paint.MustSVGPath(p.D)
		}
	})
}

// StrokeWidth is the default stroke width on the 24-unit grid.
const StrokeWidth = 1.8

// box returns ic's view box with the zero value resolved.
func (ic *Icon) box() (x, y, size float32) {
	b := ic.Box
	if b[2] <= 0 || b[3] <= 0 {
		return 0, 0, 24
	}
	return b[0], b[1], max(b[2], b[3])
}

// Draw paints ic into the square r with color c, rotated by angle radians.
// It never mirrors; see DrawWith for right-to-left layouts.
//
// The icon's paths at a given size and angle are built once and reused
// (see sized): a renderer may cache rasterized paths per *paint.Path (the
// GPU renderer does), so a fresh *Path every frame would never hit that
// cache. Position is applied as a translation around the cached paths.
func Draw(l *paint.List, ic *Icon, r geom.Rect, c color.NRGBA, angle float64) {
	DrawWith(l, ic, r, c, DrawOpts{Angle: angle})
}

// DrawOpts are the options of DrawWith.
type DrawOpts struct {
	// Angle rotates the icon by radians around its center.
	Angle float64
	// RTL draws for a right-to-left layout: directional icons mirror.
	RTL bool
	// Mirror mirrors the icon whether or not it is directional.
	Mirror bool
	// Secondary is the color of a two-tone icon's secondary parts. The
	// zero value is the main color at 20% opacity, a light tint on any
	// background.
	Secondary color.NRGBA
	// StrokeWidth overrides the stroke width of stroked parts, in units
	// of a 24-unit grid (scaled to each icon's Box). Zero keeps the
	// icon's own width. Themes use it to match a design system's line
	// weight (shadcn/ui draws 2-unit strokes).
	StrokeWidth float32
}

// Style restyles every icon drawn into a paint.List whose IconStyle is
// the style: themes use it to draw a design system's own icons for the
// built-in set. The ui runner installs the theme's style while it paints.
type Style struct {
	// Replace maps icons to the icons drawn in their place (for example
	// icon.Down to the Element Plus ArrowDown). Unlisted icons draw as
	// they are.
	Replace map[*Icon]*Icon
	// StrokeWidth is DrawOpts.StrokeWidth for draws that leave it zero.
	StrokeWidth float32
}

// Resolve returns the icon drawn for ic under s (ic itself when s is nil
// or does not replace it).
func (s *Style) Resolve(ic *Icon) *Icon {
	if s != nil {
		if r := s.Replace[ic]; r != nil {
			return r
		}
	}
	return ic
}

// Mirrored reports whether ic is drawn mirrored with these options.
func (o DrawOpts) Mirrored(ic *Icon) bool { return o.Mirror || o.RTL && ic.IsDirectional() }

// DrawWith paints ic into the square r with color c. The icon is rotated
// first and then mirrored around the vertical axis through its center, so
// a "right" chevron rotated 90° to point down still points down in RTL and
// callers keep their angles. When l.IconStyle is a *Style, ic is first
// replaced as the style says. Every draw is reported to l.IconNote when it
// is set (mirror checks and audits) under the name of the icon asked for.
func DrawWith(l *paint.List, ic *Icon, r geom.Rect, c color.NRGBA, o DrawOpts) {
	if ic == nil || c.A == 0 {
		return
	}
	name := ic.Name // the icon asked for, whatever the style draws
	if st, _ := l.IconStyle.(*Style); st != nil {
		ic = st.Resolve(ic)
		if o.StrokeWidth == 0 {
			o.StrokeWidth = st.StrokeWidth
		}
	}
	mirrored := o.Mirrored(ic)
	if l.IconNote != nil {
		l.IconNote(name, ic.Dir, mirrored, r)
	}
	if ic.img != nil {
		ic.drawImage(l, r, c)
		return
	}
	ic.load()
	_, _, bs := ic.box()
	s := min(r.W, r.H) / bs
	ox := r.X + (r.W-bs*s)/2
	oy := r.Y + (r.H-bs*s)/2
	paths := ic.sized(s, o.Angle, mirrored, o.StrokeWidth)
	sec := o.Secondary
	if sec == (color.NRGBA{}) {
		sec = color.NRGBA{R: c.R, G: c.G, B: c.B, A: uint8(int(c.A) * 51 / 255)}
	}
	l.Transform(geom.Translate(ox, oy))
	for i, p := range paths {
		if p == nil {
			continue
		}
		col := c
		if ic.Parts[i].Secondary {
			col = sec
		}
		if col.A != 0 {
			l.Ops = append(l.Ops, paint.FillPath{Path: p, Color: col})
		}
	}
	l.Pop()
}

// drawImage draws a FromImage icon fit into r.
func (ic *Icon) drawImage(l *paint.List, r geom.Rect, c color.NRGBA) {
	b := ic.img.Bounds()
	if b.Empty() {
		return
	}
	iw, ih := float32(b.Dx()), float32(b.Dy())
	k := min(r.W/iw, r.H/ih)
	w, h := iw*k, ih*k
	img := ic.img
	if ic.mask {
		img = ic.tinted(c)
	}
	l.Ops = append(l.Ops, paint.Image{Img: img, Rect: geom.Rect{X: r.X + (r.W-w)/2, Y: r.Y + (r.H-h)/2, W: w, H: h}})
}

// maxTints bounds the tinted copies kept per mask icon.
const maxTints = 8

// tinted returns the mask image colored c (alpha times c.A), cached per
// color: renderers cache textures per image, so the same color must give
// the same image.
func (ic *Icon) tinted(c color.NRGBA) image.Image {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	if t, ok := ic.tints[c]; ok {
		return t
	}
	b := ic.img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := ic.img.At(x, y).RGBA()
			i := out.PixOffset(x-b.Min.X, y-b.Min.Y)
			out.Pix[i], out.Pix[i+1], out.Pix[i+2] = c.R, c.G, c.B
			out.Pix[i+3] = uint8(a * uint32(c.A) / 0xffff)
		}
	}
	if ic.tints == nil || len(ic.tints) >= maxTints {
		ic.tints = map[color.NRGBA]image.Image{}
	}
	ic.tints[c] = out
	return out
}

// sizeKey identifies one cached rendition of an icon.
type sizeKey struct {
	s      float32 // scale (box units to px), quantized
	angle  float64
	mirror bool
	stroke float32 // DrawOpts.StrokeWidth
}

// maxSized bounds the renditions kept per icon (sizes and angles in use);
// beyond it the cache is dropped and rebuilt, which only costs a re-tessellation.
const maxSized = 16

// sized returns the icon's parts as fillable paths (strokes outlined) at
// scale s and rotation angle (around the box's centre), then mirrored
// horizontally around the centre when mirror is set, with the box's
// top-left at the origin. The returned paths are shared and must not be
// modified.
func (ic *Icon) sized(s float32, angle float64, mirror bool, strokeOverride float32) []*paint.Path {
	k := sizeKey{s: float32(math.Round(float64(s)*256) / 256), angle: angle, mirror: mirror, stroke: strokeOverride}
	ic.mu.Lock()
	defer ic.mu.Unlock()
	if v, ok := ic.cache[k]; ok {
		return v
	}
	bx, by, bs := ic.box()
	c := bs / 2 * k.s
	tf := func(p *paint.Path) *paint.Path {
		q := p
		if bx != 0 || by != 0 {
			q = q.Transform(geom.Affine{A: 1, D: 1, E: -bx, F: -by})
		}
		q = q.Transformed(k.s, 0, 0)
		if angle != 0 {
			q = q.Rotated(angle, c, c)
		}
		if mirror {
			q = q.Transform(geom.Affine{A: -1, D: 1, E: 2 * c})
		}
		return q
	}
	sw := ic.Width
	if sw <= 0 {
		sw = StrokeWidth * bs / 24
	}
	if strokeOverride > 0 {
		sw = strokeOverride * bs / 24
	}
	v := make([]*paint.Path, len(ic.parsed))
	for i, p := range ic.parsed {
		q := tf(p)
		if ic.Parts[i].Stroke {
			q = paint.StrokeToFill(q, paint.StrokeStyle{Width: sw * k.s, Cap: paint.CapRound})
		}
		v[i] = q
	}
	if ic.cache == nil || len(ic.cache) >= maxSized {
		ic.cache = map[sizeKey][]*paint.Path{}
	}
	ic.cache[k] = v
	return v
}
