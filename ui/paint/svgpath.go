package paint

import (
	"fmt"
	"math"
	"strconv"
)

// ParseSVGPath parses SVG path data ("M3 12h18", arcs included) into a Path.
func ParseSVGPath(d string) (*Path, error) {
	p := &Path{}
	i := 0
	var cmd byte
	var cx, cy, sx, sy float64 // current, subpath start
	var lcx, lcy float64       // last control point (for S/T)
	var lastCmd byte
	num := func() (float64, error) {
		for i < len(d) && (d[i] == ' ' || d[i] == ',' || d[i] == '\n' || d[i] == '\t') {
			i++
		}
		st := i
		if i < len(d) && (d[i] == '-' || d[i] == '+') {
			i++
		}
		dot := false
		for i < len(d) {
			c := d[i]
			if c >= '0' && c <= '9' {
				i++
			} else if c == '.' && !dot {
				dot = true
				i++
			} else if (c == 'e' || c == 'E') && i > st {
				i++
				if i < len(d) && (d[i] == '-' || d[i] == '+') {
					i++
				}
			} else {
				break
			}
		}
		if st == i {
			return 0, fmt.Errorf("paint: bad path number at %d in %q", st, d)
		}
		return strconv.ParseFloat(d[st:i], 64)
	}
	flag := func() (bool, error) {
		for i < len(d) && (d[i] == ' ' || d[i] == ',') {
			i++
		}
		if i >= len(d) {
			return false, fmt.Errorf("paint: missing arc flag")
		}
		c := d[i]
		i++
		return c == '1', nil
	}
	hasNum := func() bool {
		for i < len(d) && (d[i] == ' ' || d[i] == ',' || d[i] == '\n' || d[i] == '\t') {
			i++
		}
		return i < len(d) && (d[i] == '-' || d[i] == '+' || d[i] == '.' || (d[i] >= '0' && d[i] <= '9'))
	}
	f := func(v float64) float32 { return float32(v) }
	for {
		for i < len(d) && (d[i] == ' ' || d[i] == ',' || d[i] == '\n' || d[i] == '\t') {
			i++
		}
		if i >= len(d) {
			break
		}
		c := d[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			cmd = c
			i++
		} else if cmd == 0 {
			return nil, fmt.Errorf("paint: path must start with a command: %q", d)
		}
		rel := cmd >= 'a'
		var ox, oy float64
		if rel {
			ox, oy = cx, cy
		}
		upper := cmd &^ 0x20
		switch upper {
		case 'Z':
			p.Close()
			cx, cy = sx, sy
			lastCmd = 'Z'
			continue
		case 'M':
			x, err := num()
			if err != nil {
				return nil, err
			}
			y, err := num()
			if err != nil {
				return nil, err
			}
			cx, cy = ox+x, oy+y
			sx, sy = cx, cy
			p.MoveTo(f(cx), f(cy))
			// Subsequent pairs are implicit LineTo.
			if rel {
				cmd = 'l'
			} else {
				cmd = 'L'
			}
		case 'L':
			x, err := num()
			if err != nil {
				return nil, err
			}
			y, err := num()
			if err != nil {
				return nil, err
			}
			cx, cy = ox+x, oy+y
			p.LineTo(f(cx), f(cy))
		case 'H':
			x, err := num()
			if err != nil {
				return nil, err
			}
			cx = ox + x
			p.LineTo(f(cx), f(cy))
		case 'V':
			y, err := num()
			if err != nil {
				return nil, err
			}
			cy = oy + y
			p.LineTo(f(cx), f(cy))
		case 'C', 'S':
			var x1, y1 float64
			if upper == 'C' {
				a, err := num()
				if err != nil {
					return nil, err
				}
				b, err := num()
				if err != nil {
					return nil, err
				}
				x1, y1 = ox+a, oy+b
			} else if lastCmd == 'C' || lastCmd == 'S' {
				x1, y1 = 2*cx-lcx, 2*cy-lcy
			} else {
				x1, y1 = cx, cy
			}
			var v [4]float64
			for k := range v {
				n, err := num()
				if err != nil {
					return nil, err
				}
				v[k] = n
			}
			x2, y2 := ox+v[0], oy+v[1]
			cx, cy = ox+v[2], oy+v[3]
			p.CubeTo(f(x1), f(y1), f(x2), f(y2), f(cx), f(cy))
			lcx, lcy = x2, y2
		case 'Q', 'T':
			var x1, y1 float64
			if upper == 'Q' {
				a, err := num()
				if err != nil {
					return nil, err
				}
				b, err := num()
				if err != nil {
					return nil, err
				}
				x1, y1 = ox+a, oy+b
			} else if lastCmd == 'Q' || lastCmd == 'T' {
				x1, y1 = 2*cx-lcx, 2*cy-lcy
			} else {
				x1, y1 = cx, cy
			}
			a, err := num()
			if err != nil {
				return nil, err
			}
			b, err := num()
			if err != nil {
				return nil, err
			}
			cx, cy = ox+a, oy+b
			p.QuadTo(f(x1), f(y1), f(cx), f(cy))
			lcx, lcy = x1, y1
		case 'A':
			rx, err := num()
			if err != nil {
				return nil, err
			}
			ry, err := num()
			if err != nil {
				return nil, err
			}
			rot, err := num()
			if err != nil {
				return nil, err
			}
			large, err := flag()
			if err != nil {
				return nil, err
			}
			sweep, err := flag()
			if err != nil {
				return nil, err
			}
			x, err := num()
			if err != nil {
				return nil, err
			}
			y, err := num()
			if err != nil {
				return nil, err
			}
			nx, ny := ox+x, oy+y
			arcTo(p, cx, cy, rx, ry, rot, large, sweep, nx, ny)
			cx, cy = nx, ny
		default:
			return nil, fmt.Errorf("paint: unsupported path command %q", cmd)
		}
		lastCmd = upper
		_ = hasNum
	}
	return p, nil
}

// MustSVGPath is ParseSVGPath that panics; for literal icon data.
func MustSVGPath(d string) *Path {
	p, err := ParseSVGPath(d)
	if err != nil {
		panic(err)
	}
	return p
}

// arcTo converts an SVG endpoint arc to cubic Béziers (SVG spec F.6).
func arcTo(p *Path, x1, y1, rx, ry, phiDeg float64, large, sweep bool, x2, y2 float64) {
	if rx == 0 || ry == 0 {
		p.LineTo(float32(x2), float32(y2))
		return
	}
	rx, ry = math.Abs(rx), math.Abs(ry)
	phi := phiDeg * math.Pi / 180
	cphi, sphi := math.Cos(phi), math.Sin(phi)
	dx, dy := (x1-x2)/2, (y1-y2)/2
	x1p := cphi*dx + sphi*dy
	y1p := -sphi*dx + cphi*dy
	lam := x1p*x1p/(rx*rx) + y1p*y1p/(ry*ry)
	if lam > 1 {
		s := math.Sqrt(lam)
		rx, ry = rx*s, ry*s
	}
	num := rx*rx*ry*ry - rx*rx*y1p*y1p - ry*ry*x1p*x1p
	den := rx*rx*y1p*y1p + ry*ry*x1p*x1p
	co := 0.0
	if den != 0 && num > 0 {
		co = math.Sqrt(num / den)
	}
	if large == sweep {
		co = -co
	}
	cxp := co * rx * y1p / ry
	cyp := -co * ry * x1p / rx
	cx := cphi*cxp - sphi*cyp + (x1+x2)/2
	cy := sphi*cxp + cphi*cyp + (y1+y2)/2
	ang := func(ux, uy, vx, vy float64) float64 {
		a := math.Atan2(ux*vy-uy*vx, ux*vx+uy*vy)
		return a
	}
	t1 := ang(1, 0, (x1p-cxp)/rx, (y1p-cyp)/ry)
	dt := ang((x1p-cxp)/rx, (y1p-cyp)/ry, (-x1p-cxp)/rx, (-y1p-cyp)/ry)
	if !sweep && dt > 0 {
		dt -= 2 * math.Pi
	} else if sweep && dt < 0 {
		dt += 2 * math.Pi
	}
	n := int(math.Ceil(math.Abs(dt) / (math.Pi / 2)))
	if n == 0 {
		return
	}
	step := dt / float64(n)
	k := 4.0 / 3.0 * math.Tan(step/4)
	pt := func(t float64) (float64, float64) {
		x := rx * math.Cos(t)
		y := ry * math.Sin(t)
		return cphi*x - sphi*y + cx, sphi*x + cphi*y + cy
	}
	der := func(t float64) (float64, float64) {
		x := -rx * math.Sin(t)
		y := ry * math.Cos(t)
		return cphi*x - sphi*y, sphi*x + cphi*y
	}
	t := t1
	for i := 0; i < n; i++ {
		ax, ay := pt(t)
		adx, ady := der(t)
		bx, by := pt(t + step)
		bdx, bdy := der(t + step)
		p.CubeTo(float32(ax+k*adx), float32(ay+k*ady), float32(bx-k*bdx), float32(by-k*bdy), float32(bx), float32(by))
		t += step
	}
}
