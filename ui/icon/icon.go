// Package icon is a small built-in set of outlined UI icons drawn as
// vector paths (no icon font). Icons are designed on a 24×24 grid with a
// 1.8-unit stroke, close to Ant Design's outlined style. All path data in
// this file is original to Tango.
package icon

import (
	"image/color"
	"math"
	"sync"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// Icon is a vector icon.
type Icon struct {
	Name   string
	stroke string // SVG path data stroked with the icon color
	fill   string // SVG path data filled with the icon color
	once   sync.Once
	sp, fp *paint.Path
	// dir marks a directional icon (an arrow, a chevron, text lines that
	// start at the left): it mirrors horizontally in right-to-left layouts.
	dir bool

	mu    sync.Mutex
	cache map[sizeKey]sized // renditions by size, angle and mirror (sized)
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
	ic.dir = true
	return ic
}

// IsDirectional reports whether ic mirrors in right-to-left layouts.
func (ic *Icon) IsDirectional() bool { return ic != nil && ic.dir }

func def(name, stroke, fill string) *Icon {
	ic := &Icon{Name: name, stroke: stroke, fill: fill}
	all[name] = ic
	return ic
}

var all = map[string]*Icon{}

// New makes an icon from SVG path data on the same 24×24 grid as the
// built-in set: stroke is outlined with the icon color at StrokeWidth
// (round caps and joins), fill is filled with it; either may be empty.
// The path data is parsed on first draw and panics if malformed, like the
// built-ins. Icons made with New are not registered with ByName.
func New(name, stroke, fill string) *Icon { return &Icon{Name: name, stroke: stroke, fill: fill} }

// ByName looks up an icon.
func ByName(name string) *Icon { return all[name] }

// Names lists all icon names.
func Names() []string {
	out := make([]string, 0, len(all))
	for k := range all {
		out = append(out, k)
	}
	return out
}

func (ic *Icon) load() {
	ic.once.Do(func() {
		if ic.stroke != "" {
			ic.sp = paint.MustSVGPath(ic.stroke)
		}
		if ic.fill != "" {
			ic.fp = paint.MustSVGPath(ic.fill)
		}
	})
}

// StrokeWidth is the stroke width on the 24-unit grid.
const StrokeWidth = 1.8

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
}

// Mirrored reports whether ic is drawn mirrored with these options.
func (o DrawOpts) Mirrored(ic *Icon) bool { return o.Mirror || o.RTL && ic.IsDirectional() }

// DrawWith paints ic into the square r with color c. The icon is rotated
// first and then mirrored around the vertical axis through its center, so
// a "right" chevron rotated 90° to point down still points down in RTL and
// callers keep their angles. Every draw is reported to l.IconNote when it
// is set (mirror checks and audits).
func DrawWith(l *paint.List, ic *Icon, r geom.Rect, c color.NRGBA, o DrawOpts) {
	if ic == nil || c.A == 0 {
		return
	}
	mirrored := o.Mirrored(ic)
	if l.IconNote != nil {
		l.IconNote(ic.Name, ic.dir, mirrored, r)
	}
	ic.load()
	s := min(r.W, r.H) / 24
	ox := r.X + (r.W-24*s)/2
	oy := r.Y + (r.H-24*s)/2
	fill, stroke := ic.sized(s, o.Angle, mirrored)
	if fill == nil && stroke == nil {
		return
	}
	l.Transform(geom.Translate(ox, oy))
	if fill != nil {
		l.Ops = append(l.Ops, paint.FillPath{Path: fill, Color: c})
	}
	if stroke != nil {
		l.Ops = append(l.Ops, paint.FillPath{Path: stroke, Color: c})
	}
	l.Pop()
}

// sizeKey identifies one cached rendition of an icon.
type sizeKey struct {
	s      float32 // scale (24-unit grid to px), quantized
	angle  float64
	mirror bool
}

type sized struct{ fill, stroke *paint.Path }

// maxSized bounds the renditions kept per icon (sizes and angles in use);
// beyond it the cache is dropped and rebuilt, which only costs a re-tessellation.
const maxSized = 16

// sized returns the icon's fill path and stroked outline at scale s and
// rotation angle (around the 24-unit square's centre), then mirrored
// horizontally around the centre when mirror is set, with the square's
// top-left at the origin. The returned paths are shared and must not be
// modified.
func (ic *Icon) sized(s float32, angle float64, mirror bool) (fill, stroke *paint.Path) {
	k := sizeKey{s: float32(math.Round(float64(s)*256) / 256), angle: angle, mirror: mirror}
	ic.mu.Lock()
	defer ic.mu.Unlock()
	if v, ok := ic.cache[k]; ok {
		return v.fill, v.stroke
	}
	tf := func(p *paint.Path) *paint.Path {
		q := p.Transformed(k.s, 0, 0)
		if angle != 0 {
			q = q.Rotated(angle, 12*k.s, 12*k.s)
		}
		if mirror {
			q = q.Transform(geom.Affine{A: -1, D: 1, E: 24 * k.s})
		}
		return q
	}
	var v sized
	if ic.fp != nil {
		v.fill = tf(ic.fp)
	}
	if ic.sp != nil {
		v.stroke = paint.StrokeToFill(tf(ic.sp), paint.StrokeStyle{Width: StrokeWidth * k.s, Cap: paint.CapRound})
	}
	if ic.cache == nil || len(ic.cache) >= maxSized {
		ic.cache = map[sizeKey]sized{}
	}
	ic.cache[k] = v
	return v.fill, v.stroke
}

func circle(cx, cy, r float64) string {
	return arcPath(cx, cy, r)
}

func arcPath(cx, cy, r float64) string {
	f := func(v float64) string { return trim(v) }
	return "M" + f(cx-r) + " " + f(cy) + "A" + f(r) + " " + f(r) + " 0 1 0 " + f(cx+r) + " " + f(cy) + "A" + f(r) + " " + f(r) + " 0 1 0 " + f(cx-r) + " " + f(cy) + "Z"
}

func dot(cx, cy, r float64) string { return arcPath(cx, cy, r) }

func trim(v float64) string {
	v = math.Round(v*100) / 100
	s := ""
	if v < 0 {
		s = "-"
		v = -v
	}
	i := int64(v)
	fr := int64(math.Round((v - float64(i)) * 100))
	if fr == 100 {
		i++
		fr = 0
	}
	out := s + itoa(i)
	if fr != 0 {
		d := itoa(fr)
		if fr < 10 {
			d = "0" + d
		}
		for len(d) > 0 && d[len(d)-1] == '0' {
			d = d[:len(d)-1]
		}
		out += "." + d
	}
	return out
}

func itoa(i int64) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	n := len(b)
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	return string(b[n:])
}

func gear() string {
	// Eight-tooth gear outline.
	out := ""
	const teeth = 8
	for i := 0; i < teeth*4; i++ {
		a := float64(i) / float64(teeth*4) * 2 * math.Pi
		r := 9.0
		if (i/2)%2 == 1 {
			r = 7.0
		}
		x, y := 12+r*math.Cos(a-math.Pi/32*2), 12+r*math.Sin(a-math.Pi/32*2)
		if i == 0 {
			out += "M" + trim(x) + " " + trim(y)
		} else {
			out += "L" + trim(x) + " " + trim(y)
		}
	}
	return out + "Z" + circle(12, 12, 3)
}

func star() string {
	out := ""
	for i := 0; i < 10; i++ {
		a := -math.Pi/2 + float64(i)*math.Pi/5
		r := 9.5
		if i%2 == 1 {
			r = 4.2
		}
		x, y := 12+r*math.Cos(a), 12.6+r*math.Sin(a)
		if i == 0 {
			out += "M" + trim(x) + " " + trim(y)
		} else {
			out += "L" + trim(x) + " " + trim(y)
		}
	}
	return out + "Z"
}

// The icon set.
var (
	Search         = def("search", circle(11, 11, 7)+"M16.2 16.2L20.5 20.5", "")
	Close          = def("close", "M6 6L18 18M18 6L6 18", "")
	Check          = def("check", "M4.5 12.5L9.5 17.5L19.5 6.5", "")
	Down           = def("down", "M5.5 9L12 15.5L18.5 9", "")
	Up             = def("up", "M5.5 15L12 8.5L18.5 15", "")
	Left           = def("left", "M15 5.5L8.5 12L15 18.5", "").Directional()
	Right          = def("right", "M9 5.5L15.5 12L9 18.5", "").Directional()
	DoubleLeft     = def("double-left", "M12 6L6 12L12 18M18 6L12 12L18 18", "").Directional()
	DoubleRight    = def("double-right", "M6 6L12 12L6 18M12 6L18 12L12 18", "").Directional()
	Plus           = def("plus", "M12 4.5V19.5M4.5 12H19.5", "")
	Minus          = def("minus", "M4.5 12H19.5", "")
	User           = def("user", circle(12, 8, 4.2)+"M4 21C4 16.6 7.6 14 12 14C16.4 14 20 16.6 20 21", "")
	Team           = def("team", circle(9, 8, 3.6)+"M2.5 20C2.5 16.2 5.4 13.8 9 13.8C12.6 13.8 15.5 16.2 15.5 20"+"M15 4.6C16.8 4.9 18.2 6.3 18.2 8.2C18.2 10 16.9 11.4 15.2 11.7M17.6 14.4C20 15.2 21.5 17.2 21.5 20", "")
	Home           = def("home", "M3 11.5L12 3.5L21 11.5M5.5 9.5V20.5H18.5V9.5M10 20.5V15H14V20.5", "")
	Appstore       = def("appstore", "M3.5 3.5H10.5V10.5H3.5ZM13.5 3.5H20.5V10.5H13.5ZM3.5 13.5H10.5V20.5H3.5ZM13.5 13.5H20.5V20.5H13.5Z", "")
	Dashboard      = def("dashboard", "M3 17A9 9 0 1 1 21 17M12 17L16 10.5M6.2 13.5L4.7 12.8M7.8 9.2L6.8 7.8M12 7.5V5.8", dot(12, 17, 1.6))
	Table          = def("table", "M3.5 4.5H20.5V19.5H3.5ZM3.5 9.5H20.5M3.5 14.5H20.5M9.5 9.5V19.5", "")
	Form           = def("form", "M14 3.5H6.5V20.5H17.5V7ZM14 3.5V7H17.5M9 11.5H15M9 15H15M9 8H11.5", "").Directional()
	File           = def("file", "M14 3H6V21H18V7ZM14 3V7H18", "")
	Bell           = def("bell", "M6 16.5V11C6 7.7 8.7 5 12 5C15.3 5 18 7.7 18 11V16.5L19.5 18.5H4.5ZM10 21H14M12 3V5", "")
	Info           = def("info-circle", circle(12, 12, 9)+"M12 11V16.8", dot(12, 7.8, 1.2))
	CheckCircle    = def("check-circle", circle(12, 12, 9)+"M8 12.2L11 15.2L16.2 9.5", "")
	CloseCircle    = def("close-circle", circle(12, 12, 9)+"M9 9L15 15M15 9L9 15", "")
	Exclamation    = def("exclamation-circle", circle(12, 12, 9)+"M12 7.2V13", dot(12, 16.4, 1.2))
	Question       = def("question-circle", circle(12, 12, 9)+"M9.3 9.5C9.3 8 10.5 7 12 7C13.6 7 14.7 8 14.7 9.4C14.7 11.4 12 11.4 12 13.6", dot(12, 16.6, 1.2))
	Warning        = def("warning", "M12 3.5L21.5 20H2.5ZM12 9.5V14", dot(12, 16.8, 1.1))
	Loading        = def("loading", "M12 3A9 9 0 1 1 3 12", "")
	Calendar       = def("calendar", "M3.5 5.5H20.5V20.5H3.5ZM3.5 10H20.5M8 3V7.5M16 3V7.5", "")
	Clock          = def("clock", circle(12, 12, 9)+"M12 7V12L15.5 14", "")
	Eye            = def("eye", "M2 12C4.5 7.2 8 5 12 5C16 5 19.5 7.2 22 12C19.5 16.8 16 19 12 19C8 19 4.5 16.8 2 12Z"+circle(12, 12, 3.2), "")
	EyeInvisible   = def("eye-invisible", "M2 12C4.5 7.2 8 5 12 5C16 5 19.5 7.2 22 12C19.5 16.8 16 19 12 19C8 19 4.5 16.8 2 12Z"+circle(12, 12, 3.2)+"M4 20L20 4", "")
	Edit           = def("edit", "M4 20H8L19 9L15 5L4 16ZM13 7L17 11M4 20H20", "")
	Delete         = def("delete", "M4 6.5H20M9 6.5V4H15V6.5M6 6.5L7 21H17L18 6.5M10 10.5V17M14 10.5V17", "")
	MenuFold       = def("menu-fold", "M3.5 5H20.5M11 12H20.5M3.5 19H20.5M7.5 9L4.5 12L7.5 15", "").Directional()
	MenuUnfold     = def("menu-unfold", "M3.5 5H20.5M11 12H20.5M3.5 19H20.5M4.5 9L7.5 12L4.5 15", "").Directional()
	Logout         = def("logout", "M10 4H5V20H10M15 8L19 12L15 16M19 12H9", "").Directional()
	Mail           = def("mail", "M3 5.5H21V18.5H3ZM3 6.5L12 13L21 6.5", "")
	Lock           = def("lock", "M5 11H19V21H5ZM8 11V7.5C8 5.3 9.8 3.5 12 3.5C14.2 3.5 16 5.3 16 7.5V11M12 15V17", "")
	Star           = def("star", star(), "")
	StarFilled     = def("star-filled", "", star())
	Upload         = def("upload", "M12 15.5V4M7 8.5L12 3.5L17 8.5M4 14.5V20H20V14.5", "")
	Download       = def("download", "M12 3.5V15M7 10L12 15L17 10M4 14.5V20H20V14.5", "")
	Reload         = def("reload", "M19.5 12A7.5 7.5 0 1 1 17 6.4M17.5 2.5V6.8H13.2", "")
	Filter         = def("filter", "M3.5 4.5H20.5L14 12.5V19.5L10 17.5V12.5Z", "")
	More           = def("more", "", dot(5, 12, 1.7)+dot(12, 12, 1.7)+dot(19, 12, 1.7))
	CaretUp        = def("caret-up", "", "M12 6.5L18.5 14.5H5.5Z")
	CaretDown      = def("caret-down", "", "M12 17.5L5.5 9.5H18.5Z")
	ArrowUp        = def("arrow-up", "M12 20V4.5M5.5 11L12 4.5L18.5 11", "")
	ArrowDown      = def("arrow-down", "M12 4V19.5M5.5 13L12 19.5L18.5 13", "")
	ArrowRight     = def("arrow-right", "M4 12H19.5M13 5.5L19.5 12L13 18.5", "").Directional()
	Setting        = def("setting", gear(), "")
	BarChart       = def("bar-chart", "M3.5 20.5H20.5M6.5 17V11M11 17V6M15.5 17V9M20 17V13", "")
	PieChart       = def("pie-chart", "M11 3.5A8.5 8.5 0 1 0 20.5 13H11ZM14 2.5A8 8 0 0 1 21.5 10H14Z", "")
	Shop           = def("shop", "M3.5 9L5 4H19L20.5 9M3.5 9H20.5M3.5 9C3.5 10.7 4.9 12 6.3 12C7.7 12 9.1 10.7 9.1 9C9.1 10.7 10.5 12 12 12C13.5 12 14.9 10.7 14.9 9C14.9 10.7 16.3 12 17.7 12C19.1 12 20.5 10.7 20.5 9M5 12V20.5H19V12M10 20.5V16H14V20.5", "")
	Message        = def("message", "M3.5 4.5H20.5V16.5H9L4.5 20V16.5H3.5ZM8 10.5H8.01M12 10.5H12.01M16 10.5H16.01", "").Directional()
	Global         = def("global", circle(12, 12, 9)+"M3 12H21M12 3C9.5 5.5 8.5 8.7 8.5 12C8.5 15.3 9.5 18.5 12 21C14.5 18.5 15.5 15.3 15.5 12C15.5 8.7 14.5 5.5 12 3Z", "")
	Profile        = def("profile", "M4 3.5H20V20.5H4ZM8 8H16M8 12H16M8 16H12", "").Directional()
	Link           = def("link", "M10 14L14 10M8.5 11.5L6 14C4.6 15.4 4.6 17.6 6 19C7.4 20.4 9.6 20.4 11 19L13.5 16.5M10.5 7.5L13 5C14.4 3.6 16.6 3.6 18 5C19.4 6.4 19.4 8.6 18 10L15.5 12.5", "")
	Copy           = def("copy", "M8 8H20V20H8ZM16 8V4H4V16H8", "")
	Smile          = def("smile", circle(12, 12, 9)+"M8 14C9 15.5 10.4 16.2 12 16.2C13.6 16.2 15 15.5 16 14", dot(9, 9.5, 1.1)+dot(15, 9.5, 1.1))
	Fullscreen     = def("fullscreen", "M4 9V4H9M15 4H20V9M20 15V20H15M9 20H4V15", "")
	Rise           = def("rise", "M3 17L9 11L13 15L21 7M15 7H21V13", "")
	Fall           = def("fall", "M3 7L9 13L13 9L21 17M15 17H21V11", "")
	Inbox          = def("inbox", "M3.5 13.5L6 5H18L20.5 13.5V19.5H3.5ZM3.5 13.5H8.5L9.5 15.5H14.5L15.5 13.5H20.5", "")
	CheckSmall     = def("check-small", "M6 12.5L10 16.5L18 8", "")
	CloseFilled    = def("close-circle-filled", "", circle(12, 12, 10))
	CheckFilled    = def("check-circle-filled", "", circle(12, 12, 10))
	InfoFilled     = def("info-circle-filled", "", circle(12, 12, 10))
	WarningFilled  = def("exclamation-circle-filled", "", circle(12, 12, 10))
	Paperclip      = def("paper-clip", "M16.5 7.5L8.6 15.4C7.8 16.2 7.8 17.4 8.6 18.1C9.4 18.9 10.6 18.9 11.4 18.1L19 10.5C20.6 8.9 20.6 6.4 19 4.8C17.4 3.2 14.9 3.2 13.3 4.8L5.5 12.6C3.2 14.9 3.2 18.6 5.5 20.9", "")
	Picture        = def("picture", "M3.5 4.5H20.5V19.5H3.5ZM3.5 16L8.5 11L12.5 15L15 12.5L20.5 18M15.5 8.5H15.51", "")
	CloudUpload    = def("cloud-upload", "M7 18.5H6.5C4.3 18.5 2.5 16.7 2.5 14.5C2.5 12.4 4.1 10.7 6.1 10.5C6.6 7.4 9.1 5 12.2 5C15.2 5 17.7 7.2 18.2 10.1C20.1 10.4 21.5 12 21.5 14C21.5 16.5 19.5 18.5 17 18.5M12 20.5V12M9 15L12 12L15 15", "")
	Swap           = def("swap", "M4 8.5H19.5M15.5 4.5L19.5 8.5L15.5 12.5M20 15.5H4.5M8.5 11.5L4.5 15.5L8.5 19.5", "")
	ZoomIn         = def("zoom-in", circle(11, 11, 6)+"M15.4 15.4L20.5 20.5M11 8V14M8 11H14", "")
	ZoomOut        = def("zoom-out", circle(11, 11, 6)+"M15.4 15.4L20.5 20.5M8 11H14", "")
	PictureOff     = def("picture-off", "M3.5 5H13M17 5H20.5V19H3.5V5M3.5 16L9 11L11.5 13.5M14.5 4L10 20", "")
	SwapVertical   = def("swap-vertical", "M8 4V19M4.5 15.5L8 19L11.5 15.5M16 20V5M12.5 8.5L16 5L19.5 8.5", "")
	RotateLeft     = def("rotate-left", "M4.5 12A7.5 7.5 0 1 0 7 6.4M6.5 2.5V6.8H10.8", "")
	RotateRight    = def("rotate-right", "M19.5 12A7.5 7.5 0 1 1 17 6.4M17.5 2.5V6.8H13.2", "")
	Folder         = def("folder", "M3 6H9.5L11.5 8H21V19H3Z", "")
	FolderOpen     = def("folder-open", "M3 19V6H9.5L11.5 8H19V11M3 19L6 11H22L19 19Z", "")
	FileText       = def("file-text", "M14 3H6V21H18V7ZM14 3V7H18M9 12H15M9 16H15", "").Directional()
	Like           = def("like", "M7.5 10.5V20H4V10.5ZM7.5 10.5L11 3.5C12.8 3.5 13.5 4.6 13.3 6.3L12.8 9.5H18.6C19.8 9.5 20.6 10.6 20.3 11.8L18.6 18.6C18.4 19.4 17.7 20 16.8 20H7.5", "")
	Holder         = def("holder", "", dot(8.5, 6.1, 1.6)+dot(15.5, 6.1, 1.6)+dot(8.5, 12, 1.6)+dot(15.5, 12, 1.6)+dot(8.5, 17.9, 1.6)+dot(15.5, 17.9, 1.6))
	FullscreenExit = def("fullscreen-exit", "M9 4V9H4M15 4V9H20M9 20V15H4M15 20V15H20", "")
)
