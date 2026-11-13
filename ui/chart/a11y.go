package chart

import (
	"fmt"
	"math"
	"strings"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
)

// Accessibility. Every chart's root box is a semantic node: RoleFigure
// named by A11yLabel (or the chart's title, or its localized kind) with a
// summary description (kind, series, points, x range, value range) built
// only while accessibility is on. Single-value charts are meters / a
// progress bar with a range; sparklines are images named by their summary.
// Legend entries are focusable toggle buttons. DataTable builds a real
// ui.Table of the rows for apps that offer a "view as table" switch.

// chartSem is what a chart tells finish about itself.
type chartSem struct {
	role a11y.Role // RoleFigure when zero
	kind string    // localized chart kind (default name)
	name string    // chart title (Gauge / Liquid caption)
	desc string    // summary; filled only when accessibility is on

	ranged      bool // meter / progress bar
	now, lo, hi float64
	value       string
}

// A11yLabel sets the accessible name of the chart (default: its title or
// the localized chart kind, e.g. "折线图").
func (b *common[T]) A11yLabel(s string) *T { b.a11yName = s; return b.self }

// on reports whether the semantics tree is built this frame (summaries are
// computed only then).
func (f *frame) on() bool { return f.c.A11yEnabled() }

// kind records the chart kind and its default role.
func (f *frame) kind(name string, role a11y.Role) {
	f.sem.kind, f.sem.role = name, role
}

// locFmt formats like fmt.Sprintf without the "%!(EXTRA ...)" note, so a
// locale format may use only some of the arguments.
func locFmt(format string, args ...any) string {
	s := fmt.Sprintf(format, args...)
	if i := strings.Index(s, "%!(EXTRA "); i >= 0 {
		s = s[:i]
	}
	return s
}

// summary formats the description: kind, series and points, then the x
// range (first, last) and the value range when known.
func (f *frame) summary(series, points int, x0, x1 string, lo, hi float64, vf Formatter) string {
	loc := f.c.Locale().Chart
	var b strings.Builder
	b.WriteString(locFmt(loc.SummaryFmt, f.sem.kind, series, points))
	if x0 != "" || x1 != "" {
		b.WriteString(locFmt(loc.XRangeFmt, x0, x1))
	}
	if !math.IsInf(lo, 0) && !math.IsInf(hi, 0) && !math.IsNaN(lo) && !math.IsNaN(hi) {
		b.WriteString(locFmt(loc.YRangeFmt, vf(lo), vf(hi)))
	}
	return b.String()
}

// valueRange returns the min and max of the non-NaN values and their count.
func valueRange(vals [][]float64) (lo, hi float64, n int) {
	lo, hi = math.Inf(1), math.Inf(-1)
	for _, vs := range vals {
		for _, v := range vs {
			if math.IsNaN(v) {
				continue
			}
			n++
			lo, hi = math.Min(lo, v), math.Max(hi, v)
		}
	}
	return lo, hi, n
}

// xLabel is the label of the i-th x value of a grid.
func (g *grid) xLabel(i int) string {
	switch {
	case i < 0 || i >= len(g.xs):
		return ""
	case g.kind == kindCat && i < len(g.cats):
		return g.cats[i]
	case g.kind == kindTime:
		return numTime(g.xs[i], g.loc).Format("2006-01-02")
	}
	return FormatNumber(g.xs[i], min(decimalsOf(g.xs[i]), 2))
}

// gridSummary describes a series × x chart.
func (f *frame) gridSummary(g *grid, vf Formatter) string {
	lo, hi, n := valueRange(g.vals)
	return f.summary(len(g.names), n, g.xLabel(0), g.xLabel(len(g.xs)-1), lo, hi, vf)
}

// semantics annotates the chart root (called by finish).
func (f *frame) semantics(sp *plotSpec) {
	sem := f.sem
	role := sem.role
	if role == a11y.RoleNone {
		role = a11y.RoleFigure
	}
	name := f.label
	if name == "" {
		name = sem.name
	}
	if name == "" {
		name = sem.kind
	}
	f.root.Semantics(role, name)
	if sp.loading {
		f.root.A11yState(a11y.Busy)
	}
	if !f.on() {
		return
	}
	desc := sem.desc
	if sp.empty && !sp.loading {
		desc = f.c.Locale().NoData
	}
	image := role == a11y.RoleImage && f.label == ""
	f.root.A11y(func(s *a11y.Node) {
		if image && desc != "" {
			s.Name = desc // a sparkline is read as its summary
		} else if desc != s.Name {
			s.Description = desc
		}
		if sem.ranged {
			s.HasRange = true
			s.ValueNow, s.ValueMin, s.ValueMax = sem.now, sem.lo, sem.hi
			s.Value = sem.value
		}
	})
}

// DataTable builds a ui.Table of rows with one column per field (titled by
// the field name): the accessible alternative of a chart, for apps that
// offer a "view as table" toggle next to it.
//
//	if showTable {
//		chart.DataTable(c, rows, "month", "city", "temp").Show()
//	} else {
//		chart.Line(c, rows).X("month").Y("temp").Series("city").Show()
//	}
func DataTable(c *ui.Context, rows []Row, fields ...string) *ui.TableW {
	cols := make([]ui.TableColumn, len(fields))
	for i, fl := range fields {
		fl := fl
		col := ui.TableColumn{Key: fl, Title: fl, Text: func(r int) string { return cellText(rows[r][fl]) }}
		if len(rows) > 0 {
			if _, ok := Num(rows[0][fl]); ok {
				// Numbers align to the inline end: right in LTR, left in
				// RTL tables (logical, like the table's other columns).
				col.Align = ui.AlignEnd
			}
		}
		cols[i] = col
	}
	return ui.Table(c, cols, len(rows))
}

func cellText(v any) string {
	if v == nil {
		return "-"
	}
	if x, ok := Num(v); ok {
		if _, isStr := v.(string); !isStr {
			return FormatNumber(x, min(decimalsOf(x), 2))
		}
	}
	return Str(v)
}
