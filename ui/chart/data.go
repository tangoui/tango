package chart

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"
)

// Row is one datum: field name to value. Values may be any Go number,
// string (category), time.Time, or nil (missing).
type Row = map[string]any

// RowsOf converts typed items to rows.
//
//	rows := chart.RowsOf(sales, func(s Sale) chart.Row {
//		return chart.Row{"month": s.Month, "value": s.Amount}
//	})
func RowsOf[T any](items []T, fn func(T) Row) []Row {
	out := make([]Row, len(items))
	for i, it := range items {
		out[i] = fn(it)
	}
	return out
}

// Num converts a field value to float64. Strings are not parsed: a string
// is a category, not a number.
func Num(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, !math.IsNaN(x)
	case float32:
		return float64(x), !math.IsNaN(float64(x))
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case int32:
		return float64(x), true
	case int16:
		return float64(x), true
	case int8:
		return float64(x), true
	case uint:
		return float64(x), true
	case uint64:
		return float64(x), true
	case uint32:
		return float64(x), true
	case uint16:
		return float64(x), true
	case uint8:
		return float64(x), true
	case time.Duration:
		return float64(x), true
	case interface{ Float64() (float64, error) }: // json.Number
		f, err := x.Float64()
		return f, err == nil
	}
	return 0, false
}

// Str converts a field value to its category label.
func Str(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case time.Time:
		return x.Format("2006-01-02")
	case fmt.Stringer:
		return x.String()
	}
	if f, ok := Num(v); ok {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return fmt.Sprint(v)
}

// TimeOf converts a field value to a time.
func TimeOf(v any) (time.Time, bool) {
	switch x := v.(type) {
	case time.Time:
		return x, true
	case *time.Time:
		if x != nil {
			return *x, true
		}
	}
	return time.Time{}, false
}

// fieldKind classifies an x field.
type fieldKind uint8

const (
	kindCat fieldKind = iota
	kindNum
	kindTime
)

func detectKind(data []Row, field string) fieldKind {
	for _, r := range data {
		v := r[field]
		if v == nil {
			continue
		}
		if _, ok := TimeOf(v); ok {
			return kindTime
		}
		if _, ok := Num(v); ok {
			return kindNum
		}
		return kindCat
	}
	return kindCat
}

// uniq returns the distinct labels of field in order of first appearance.
func uniq(data []Row, field string) []string {
	if field == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, r := range data {
		s := Str(r[field])
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func timeNum(t time.Time) float64 { return float64(t.UnixNano()) / 1e9 }

func numTime(v float64, loc *time.Location) time.Time {
	s, f := math.Modf(v)
	t := time.Unix(int64(s), int64(f*1e9))
	if loc != nil {
		t = t.In(loc)
	}
	return t
}

// grid is series × x data aligned on a shared x axis.
type grid struct {
	kind   fieldKind
	cats   []string  // kindCat: x labels; xs are 0..n-1
	xs     []float64 // sorted distinct x values
	names  []string  // series names (one "" series when ungrouped)
	vals   [][]float64
	loc    *time.Location
	sorted bool
}

func (g *grid) empty() bool { return len(g.xs) == 0 }

// buildGrid groups rows by series and x. Missing cells are NaN.
func buildGrid(data []Row, xf, yf, sf string) *grid {
	return buildGridKind(data, xf, yf, sf, detectKind(data, xf))
}

// buildCatGrid is buildGrid with the x field always treated as categories
// (bars, heatmaps).
func buildCatGrid(data []Row, xf, yf, sf string) *grid {
	return buildGridKind(data, xf, yf, sf, kindCat)
}

func buildGridKind(data []Row, xf, yf, sf string, kind fieldKind) *grid {
	g := &grid{kind: kind}
	g.names = uniq(data, sf)
	if sf == "" {
		g.names = []string{""}
	}
	sIdx := make(map[string]int, len(g.names))
	for i, n := range g.names {
		sIdx[n] = i
	}
	switch g.kind {
	case kindCat:
		g.cats = uniq(data, xf)
		g.xs = make([]float64, len(g.cats))
		cIdx := make(map[string]int, len(g.cats))
		for i, s := range g.cats {
			g.xs[i] = float64(i)
			cIdx[s] = i
		}
		g.alloc()
		for _, r := range data {
			y, ok := Num(r[yf])
			if !ok {
				continue
			}
			s := 0
			if sf != "" {
				s = sIdx[Str(r[sf])]
			}
			g.vals[s][cIdx[Str(r[xf])]] = y
		}
		return g
	}
	// Numeric or time x: collect, sort, dedupe.
	type pt struct {
		x, y float64
		s    int
	}
	pts := make([]pt, 0, len(data))
	ordered := true
	for _, r := range data {
		var x float64
		if g.kind == kindTime {
			t, ok := TimeOf(r[xf])
			if !ok {
				continue
			}
			if g.loc == nil {
				g.loc = t.Location()
			}
			x = timeNum(t)
		} else {
			v, ok := Num(r[xf])
			if !ok {
				continue
			}
			x = v
		}
		y, ok := Num(r[yf])
		if !ok {
			y = math.NaN()
		}
		s := 0
		if sf != "" {
			s = sIdx[Str(r[sf])]
		}
		if n := len(pts); n > 0 && x < pts[n-1].x {
			ordered = false
		}
		pts = append(pts, pt{x, y, s})
	}
	if !ordered {
		sort.SliceStable(pts, func(i, j int) bool { return pts[i].x < pts[j].x })
	}
	xs := make([]float64, 0, len(pts))
	for _, p := range pts {
		if n := len(xs); n == 0 || xs[n-1] != p.x {
			xs = append(xs, p.x)
		}
	}
	g.xs = xs
	g.alloc()
	j := 0
	for _, p := range pts {
		for xs[j] != p.x {
			j++
		}
		g.vals[p.s][j] = p.y
	}
	return g
}

func (g *grid) alloc() {
	g.vals = make([][]float64, len(g.names))
	for i := range g.vals {
		v := make([]float64, len(g.xs))
		for j := range v {
			v[j] = math.NaN()
		}
		g.vals[i] = v
	}
}

// nearestX returns the index of the x value closest to v (xs sorted).
func nearestX(xs []float64, v float64) int {
	n := len(xs)
	if n == 0 {
		return -1
	}
	i := sort.SearchFloat64s(xs, v)
	if i <= 0 {
		return 0
	}
	if i >= n {
		return n - 1
	}
	if v-xs[i-1] <= xs[i]-v {
		return i - 1
	}
	return i
}
