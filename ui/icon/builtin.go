package icon

import "math"

func circle(cx, cy, r float64) string {
	return arcPath(cx, cy, r)
}

func arcPath(cx, cy, r float64) string {
	f := func(v float64) string { return trim(v) }
	return "M" + f(cx-r) + " " + f(cy) + "A" + f(r) + " " + f(r) + " 0 1 0 " + f(cx+r) + " " + f(cy) + "A" + f(r) + " " + f(r) + " 0 1 0 " + f(cx-r) + " " + f(cy) + "Z"
}

func dot(cx, cy, r float64) string { return arcPath(cx, cy, r) }

// The holes of filled icons: shapes wound opposite to circle (clockwise
// on screen), so the non-zero rule cuts them out of an enclosing circle.
// They must not overlap each other.

type pt struct{ x, y float64 }

// hole is a circular hole.
func hole(cx, cy, r float64) string {
	f := trim
	return "M" + f(cx-r) + " " + f(cy) + "A" + f(r) + " " + f(r) + " 0 1 1 " + f(cx+r) + " " + f(cy) + "A" + f(r) + " " + f(r) + " 0 1 1 " + f(cx-r) + " " + f(cy) + "Z"
}

// bar is a vertical hole from y0 to y1 with half width hw.
func bar(cx, y0, y1, hw float64) string {
	return poly([]pt{{cx - hw, y0}, {cx + hw, y0}, {cx + hw, y1}, {cx - hw, y1}})
}

// cross is an X-shaped hole: a plus of arm length a and half width hw
// rotated 45°, as one polygon so the arms do not overlap.
func cross(cx, cy, a, hw float64) string {
	q := []pt{{hw, -a}, {hw, -hw}, {a, -hw}, {a, hw}, {hw, hw}, {hw, a}, {-hw, a}, {-hw, hw}, {-a, hw}, {-a, -hw}, {-hw, -hw}, {-hw, -a}}
	c, s := math.Cos(math.Pi/4), math.Sin(math.Pi/4)
	for i, p := range q {
		q[i] = pt{cx + p.x*c - p.y*s, cy + p.x*s + p.y*c}
	}
	return poly(q)
}

// thick is the outline of the polyline pts with half width hw (mitred
// joins, butt ends) as one polygon.
func thick(pts []pt, hw float64) string {
	n := len(pts)
	left, right := make([]pt, n), make([]pt, n)
	normal := func(a, b pt) pt {
		dx, dy := b.x-a.x, b.y-a.y
		l := math.Hypot(dx, dy)
		return pt{-dy / l, dx / l}
	}
	for i := range pts {
		var nv pt
		switch {
		case i == 0:
			nv = normal(pts[0], pts[1])
		case i == n-1:
			nv = normal(pts[n-2], pts[n-1])
		default:
			a, b := normal(pts[i-1], pts[i]), normal(pts[i], pts[i+1])
			m := pt{a.x + b.x, a.y + b.y}
			l := math.Hypot(m.x, m.y)
			m = pt{m.x / l, m.y / l}
			k := 1 / (m.x*a.x + m.y*a.y) // miter length
			nv = pt{m.x * k, m.y * k}
		}
		left[i] = pt{pts[i].x + nv.x*hw, pts[i].y + nv.y*hw}
		right[i] = pt{pts[i].x - nv.x*hw, pts[i].y - nv.y*hw}
	}
	out := append([]pt{}, left...)
	for i := n - 1; i >= 0; i-- {
		out = append(out, right[i])
	}
	return poly(out)
}

// poly is a closed polygon wound clockwise on screen (y down).
func poly(q []pt) string {
	area := 0.0
	for i := range q {
		j := (i + 1) % len(q)
		area += q[i].x*q[j].y - q[j].x*q[i].y
	}
	if area < 0 {
		for i, j := 0, len(q)-1; i < j; i, j = i+1, j-1 {
			q[i], q[j] = q[j], q[i]
		}
	}
	out := ""
	for i, p := range q {
		if i == 0 {
			out += "M"
		} else {
			out += "L"
		}
		out += trim(p.x) + " " + trim(p.y)
	}
	return out + "Z"
}

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
	CloseFilled    = def("close-circle-filled", "", circle(12, 12, 10)+cross(12, 12, 4.4, 1.1))
	CheckFilled    = def("check-circle-filled", "", circle(12, 12, 10)+thick([]pt{{7.1, 12.4}, {10.5, 15.8}, {17, 9.1}}, 1.1))
	InfoFilled     = def("info-circle-filled", "", circle(12, 12, 10)+hole(12, 7.3, 1.35)+bar(12, 10.6, 17.4, 1.1))
	WarningFilled  = def("exclamation-circle-filled", "", circle(12, 12, 10)+bar(12, 6.6, 13.4, 1.1)+hole(12, 16.7, 1.35))
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
	// Added with the component icon consolidation (original path data).
	ArrowLeft        = def("arrow-left", "M20 12H4.5M11 5.5L4.5 12L11 18.5", "").Directional()
	CaretLeft        = def("caret-left", "", "M9 12L15 6.5V17.5Z").Directional()
	CaretRight       = def("caret-right", "", "M15 12L9 6.5V17.5Z").Directional()
	PlusCircle       = def("plus-circle", circle(12, 12, 9)+"M12 8V16M8 12H16", "")
	MinusCircle      = def("minus-circle", circle(12, 12, 9)+"M8 12H16", "")
	MoreVertical     = def("more-vertical", "", dot(12, 5, 1.7)+dot(12, 12, 1.7)+dot(12, 19, 1.7))
	ChevronsUpDown   = def("chevrons-up-down", "M7.5 9.5L12 5L16.5 9.5M7.5 14.5L12 19L16.5 14.5", "")
	ExternalLink     = def("external-link", "M13.5 4H20V10.5M20 4L11 13M17.5 14V20H4V6.5H10", "").Directional()
	PanelLeft        = def("panel-left", "M5 3.5H19C19.8 3.5 20.5 4.2 20.5 5V19C20.5 19.8 19.8 20.5 19 20.5H5C4.2 20.5 3.5 19.8 3.5 19V5C3.5 4.2 4.2 3.5 5 3.5ZM9.5 3.5V20.5", "").Directional()
	Sliders          = def("sliders", "M4 7H20M4 17H20", dot(15, 7, 2.6)+dot(9, 17, 2.6))
	FilterFilled     = def("filter-filled", "", "M3 4H21L14.2 12.4V19.6L9.8 21.5V12.4Z")
	ColumnHeight     = def("column-height", "M4 4.5H20M4 19.5H20M12 7.5V16.5M9 10L12 7L15 10M9 14L12 17L15 14", "")
	VerticalAlignTop = def("vertical-align-top", "M4 3.5H20M12 21V8M7 12.5L12 7.5L17 12.5", "")
	VerticalAlignMid = def("vertical-align-middle", "M4 12H20M12 2.5V8.5M9 5.5L12 8.5L15 5.5M12 21.5V15.5M9 18.5L12 15.5L15 18.5", "")
	VerticalAlignBot = def("vertical-align-bottom", "M4 20.5H20M12 3V16M7 11.5L12 16.5L17 11.5", "")
	CustomerService  = def("customer-service", "M4.5 15V12C4.5 7.9 7.9 4.5 12 4.5C16.1 4.5 19.5 7.9 19.5 12V15M4.5 13.5H7.5V19H4.5ZM16.5 13.5H19.5V19H16.5ZM19.5 19C19.5 20.4 17.6 21 14.5 21", "")
	Comment          = def("comment", "M4 5H20V16H10.5L6 19.5V16H4Z", "")
	QuestionMark     = def("question", "M8.6 9C8.6 7 10.1 5.6 12 5.6C13.9 5.6 15.4 6.9 15.4 8.7C15.4 11.2 12 11.4 12 14.2M12 18.2V18.3", "")
	Sound            = def("sound", "M4 9.5H7.5L12 5.5V18.5L7.5 14.5H4ZM15.5 9C16.5 10 16.5 14 15.5 15M18 6.5C20.5 9 20.5 15 18 17.5", "").Directional()
	// IDE basics (original path data); the sets have many more.
	Play      = def("play", "", "M7 4.5L19.5 12L7 19.5Z")
	Pause     = def("pause", "", "M6.5 4.5H10V19.5H6.5ZM14 4.5H17.5V19.5H14Z")
	Stop      = def("stop", "", "M5.5 5.5H18.5V18.5H5.5Z")
	Undo      = def("undo", "M9 14L4 9L9 4M4 9H14.5C17.5 9 20 11.5 20 14.5C20 17.5 17.5 20 14.5 20H11", "").Directional()
	Redo      = def("redo", "M15 14L20 9L15 4M20 9H9.5C6.5 9 4 11.5 4 14.5C4 17.5 6.5 20 9.5 20H13", "").Directional()
	Save      = def("save", "M4 4H16.5L20 7.5V20H4ZM8 4V9H15V4M7.5 20V14H16.5V20", "")
	Terminal  = def("terminal", "M3.5 4.5H20.5V19.5H3.5ZM7 9L10 12L7 15M12.5 15.5H17", "")
	Code      = def("code", "M8.5 7L3.5 12L8.5 17M15.5 7L20.5 12L15.5 17M13.5 4.5L10.5 19.5", "")
	Bug       = def("bug", "M8 9.5C8 7.3 9.8 5.5 12 5.5C14.2 5.5 16 7.3 16 9.5V15C16 17.2 14.2 19 12 19C9.8 19 8 17.2 8 15ZM12 10V19M8 12H4M16 12H20M8 16L4.5 18M16 16L19.5 18M8 9L4.5 7M16 9L19.5 7M9.5 6L8 3.5M14.5 6L16 3.5", "")
	Key       = def("key", circle(7.5, 15.5, 4)+"M10.5 12.5L20 3M16.5 6.5L19 9M14 9L16 11", "")
	Unlock    = def("unlock", "M5 11H19V21H5ZM8 11V7.5C8 5.3 9.8 3.5 12 3.5C13.8 3.5 15.3 4.7 15.8 6.3M12 15V17", "")
	Database  = def("database", "M4.5 6C4.5 4.6 7.9 3.5 12 3.5C16.1 3.5 19.5 4.6 19.5 6C19.5 7.4 16.1 8.5 12 8.5C7.9 8.5 4.5 7.4 4.5 6ZM4.5 6V18C4.5 19.4 7.9 20.5 12 20.5C16.1 20.5 19.5 19.4 19.5 18V6M4.5 12C4.5 13.4 7.9 14.5 12 14.5C16.1 14.5 19.5 13.4 19.5 12", "")
	Branch    = def("branch", circle(6.5, 5.5, 2)+circle(6.5, 18.5, 2)+circle(17.5, 7.5, 2)+"M6.5 7.5V16.5M17.5 9.5C17.5 13 14 13.5 10 14C8 14.3 6.5 15 6.5 16.5", "")
	History   = def("history", "M4.5 12A7.5 7.5 0 1 0 7 6.4M6.5 2.5V6.8H10.8M12 8V12L15 14", "")
	Sync      = def("sync", "M19.5 10A7.5 7.5 0 0 0 5.6 7M4.5 3.5V7.5H8.5M4.5 14A7.5 7.5 0 0 0 18.4 17M19.5 20.5V16.5H15.5", "")
	Share     = def("share", circle(18, 5.5, 2.5)+circle(6, 12, 2.5)+circle(18, 18.5, 2.5)+"M8.2 10.8L15.8 6.7M8.2 13.2L15.8 17.3", "")
	Send      = def("send", "M21 3L10 14M21 3L14.5 21L10 14L3 9.5Z", "").Directional()
	Pin       = def("pin", "M9 3.5H15M10 3.5V9.5L6.5 13.5H17.5L14 9.5V3.5M12 13.5V20.5", "")
	Sun       = def("sun", circle(12, 12, 4)+"M12 2.5V4.5M12 19.5V21.5M2.5 12H4.5M19.5 12H21.5M5.3 5.3L6.7 6.7M17.3 17.3L18.7 18.7M5.3 18.7L6.7 17.3M17.3 6.7L18.7 5.3", "")
	Moon      = def("moon", "M20 14.5A8.5 8.5 0 1 1 9.5 4A6.8 6.8 0 0 0 20 14.5Z", "")
	Sparkles  = def("sparkles", "", "M10 3L11.6 8.4L17 10L11.6 11.6L10 17L8.4 11.6L3 10L8.4 8.4ZM18 14L18.8 16.2L21 17L18.8 17.8L18 20L17.2 17.8L15 17L17.2 16.2Z")
	FileAdd   = def("file-add", "M14 3H6V21H18V7ZM14 3V7H18M12 10.5V17.5M8.5 14H15.5", "")
	FolderAdd = def("folder-add", "M3 6H9.5L11.5 8H21V19H3ZM12 10.5V16.5M9 13.5H15", "")
	Scissor   = def("scissor", circle(6.5, 6.5, 2.8)+circle(6.5, 17.5, 2.8)+"M8.7 8.3L20 18M8.7 15.7L20 6", "")
	Tag       = def("tag", "M3.5 3.5H11.5L20.5 12.5L12.5 20.5L3.5 11.5Z", dot(8, 8, 1.4))
	Bookmark  = def("bookmark", "M6 3.5H18V20.5L12 16L6 20.5Z", "")
	Heart     = def("heart", "M12 20C12 20 3.5 15 3.5 9C3.5 6.2 5.6 4 8.2 4C9.8 4 11.2 4.8 12 6.1C12.8 4.8 14.2 4 15.8 4C18.4 4 20.5 6.2 20.5 9C20.5 15 12 20 12 20Z", "")
	Flag      = def("flag", "M5 21V4M5 4.5H18L15.5 9L18 13.5H5", "")
	Monitor   = def("monitor", "M3 4.5H21V16.5H3ZM8.5 20.5H15.5M12 16.5V20.5", "")
	Cpu       = def("cpu", "M7 7H17V17H7ZM10 10H14V14H10ZM9.5 3.5V7M14.5 3.5V7M9.5 17V20.5M14.5 17V20.5M3.5 9.5H7M3.5 14.5H7M17 9.5H20.5M17 14.5H20.5", "")
	Minimize  = def("window-minimize", "M5 12H19", "")
	Maximize  = def("window-maximize", "M5 5H19V19H5Z", "")
	Restore   = def("window-restore", "M5 9H15V19H5ZM9 9V5H19V15H15", "")
)

// Graph widgets (ui/graph): diagram toolbar (original path data).
var (
	FitView    = def("fit-view", "M4 8.5V4H8.5M15.5 4H20V8.5M20 15.5V20H15.5M8.5 20H4V15.5M9 9H15V15H9Z", "")
	AutoLayout = def("auto-layout", "M9.5 3.5H14.5V8H9.5ZM3.5 16H8.5V20.5H3.5ZM15.5 16H20.5V20.5H15.5ZM12 8V12M6 16V12H18V16", "")
)

// Data viewer icons (original path data): LogView's soft-wrap toggle.
var (
	WrapText = def("wrap-text", "M4 6H20M4 12H16.5C18.4 12 20 13.6 20 15.5C20 17.4 18.4 19 16.5 19H12.5M14.5 16.8L12.2 19L14.5 21.2M4 19H9", "")
)

// Workbench shell (StatusBar, TitleBar, NotificationCenter, HotkeyInput;
// original path data).
var (
	CheckAll = def("check-all", "M2.5 12.5L7 17L16 7M11.5 15.5L13 17L22 7", "")
	Clear    = def("clear", "M15 3L11.5 9.5M6.5 9.5H16.5L18 13H5ZM5 13L4 20.5H19L18 13M8.5 20.5L9 16.5M12.5 20.5V16.5M15.5 20.5L15 16.5", "")
)

// Docking layouts (original path data): the split action of a Dock group.
var (
	SplitPane = def("split-pane", "M3.5 4.5H20.5V19.5H3.5ZM12 4.5V19.5", "")
)

// AI chat components (Bubble, ToolCall, Prompts); original path data.
var (
	Robot = def("robot", "M5 9H19V19H5ZM12 9V6.5M9.5 13V14M14.5 13V14M9.5 16.5H14.5M3 12.5V15.5M21 12.5V15.5"+circle(12, 5, 1.5), "")
	Tool  = def("tool", "M15.9 4.1V8.1H19.9A5 5 0 1 1 15.9 4.1ZM11.6 12.4L4.5 19.5", "")
	Bulb  = def("bulb", "M9.5 18H14.5M10.5 21H13.5M8.8 15C7.1 13.8 6 11.8 6 9.5A6 6 0 0 1 18 9.5C18 11.8 16.9 13.8 15.2 15V15.5H8.8Z", "")
)
