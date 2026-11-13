// Package chart is the chart library of the Tango self-drawn UI: line,
// area, column / bar, pie / donut, scatter / bubble, radar, gauge, funnel,
// heatmap, dual axes, liquid and tiny (sparkline) charts, drawn with the
// ui/paint display list and styled after Ant Design Charts (G2) for the Ant
// flavor and ECharts for Element Plus.
//
// Charts are built per frame like every other widget, with a chain that
// ends in Show:
//
//	chart.Line(c, rows).X("month").Y("value").Series("city").Smooth().Show()
//	chart.Column(c, rows).X("month").Y("sales").Series("shop").Stack().Show()
//	chart.Pie(c, rows).Angle("value").Color("type").Inner(0.64).Show()
//	chart.TinyArea(c, []float64{3, 5, 2, 8, 6}).Show()
//
// Data is a []chart.Row (map[string]any); RowsOf converts a typed slice.
// Field values may be numbers of any Go numeric type, strings (categories)
// or time.Time. Legends toggle series, tooltips follow the pointer with a
// crosshair, values animate on entry and when the data changes, and long
// lines are downsampled with LTTB to the plot width.
//
// In right-to-left scopes the plot keeps its left-to-right geometry (time
// and value axes read left to right in data visualisation), while the
// chrome follows the scope: legend items flow from the inline start and
// LegendLeft / LegendRight mean start / end, tooltips open on the inline
// end side with right-to-left rows, and category, time and other label
// text is shaped right to left (numbers keep their own direction). Mirror
// opts into a mirrored plot: the x axis runs right to left with the value
// axis on the right, and polar charts sweep counter-clockwise.
//
// The building blocks are exported for custom charts: scales (Linear, Log,
// TimeScale, Band, Point), tick generation (NiceTicks, TimeTicks), number
// formatting (FormatNumber, FormatCompact), stacking (Stack), downsampling
// (LTTB), coordinates (Cartesian, Polar) and the theme-aware Style with its
// palette registry (RegisterStyle).
package chart
