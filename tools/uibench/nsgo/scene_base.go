package main

import (
	"strconv"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// The base scenes, the same in every framework's program (../scenes.md).
const (
	listRows   = 100_000
	listRowH   = 40
	listStep   = 8 // logical px scrolled per frame
	viewH      = 600
	gridRows   = 30
	gridCols   = 20
	cellW      = 40
	cellH      = 20
	gridChange = gridRows * gridCols
)

var (
	muted   = core.RGB(0x6b7280)
	cellInk = core.RGB(0x1d2433)
	cellBg  = [4]core.Color{core.RGB(0xe2d4f3), core.RGB(0xd2ebee), core.RGB(0xf6dbc9), core.RGB(0xdde7cd)}
)

var (
	centered     = core.Style{JustifyContent: core.JustifyCenter, AlignItems: core.AlignCenter, FlexGrow: 1, Gap: 12}
	captionStyle = core.Style{Color: muted, FontSize: 13}
	tapsStyle    = core.Style{FontWeight: core.FontWeightBold, FontSize: 48}
	buttonRow    = core.Style{Gap: 10, Direction: core.Row}
	rowStyle     = core.Style{Padding: core.Symmetric(0, 16), Gap: 12, AlignItems: core.AlignCenter, Height: core.Pt(listRowH), Direction: core.Row}
	labelStyle   = core.Style{FontSize: 14, FlexGrow: 1}
	detailStyle  = core.Style{Color: muted, FontSize: 13}
	cellText     = core.Style{Color: cellInk, FontSize: 11}
)

// counterRoot shows a caption, a tap count and Less / Reset / More. One
// frame callback, after the first commit, reports ready; no more follow.
func counterRoot(cx *core.Scope) core.Node {
	taps := core.NewSignal(cx, 0)
	by := func(d int) func() { return func() { taps.Set(taps.GetUntracked() + d) } }
	cx.RequestAnimationFrame(func(time.Time) { report(0) })
	return core.View{Style: centered, Children: []core.Node{
		core.Text{Style: captionStyle, Content: "Taps"},
		core.Text{Style: tapsStyle, ContentFunc: func() string { return strconv.Itoa(taps.Get()) }},
		core.View{Style: buttonRow, Children: []core.Node{
			ui.Button{Variant: ui.ButtonSecondary, Text: "Less", OnPress: by(-1)},
			ui.Button{Variant: ui.ButtonSecondary, Text: "Reset", OnPress: func() { taps.Set(0) }},
			ui.Button{Text: "More", OnPress: by(+1)},
		}},
	}}
}

// listRoot is the list scene: 100,000 rows whose list is scrolled 8
// logical px further on every frame callback.
func listRoot(cx *core.Scope) core.Node {
	ref := &core.Ref{}
	var y core.Points
	var step func(time.Time)
	step = func(time.Time) {
		if y += listStep; y > listRows*listRowH-viewH {
			y = 0
		}
		ref.ScrollTo(core.Point{Y: y}, false)
		report(1)
		cx.RequestAnimationFrame(step)
	}
	cx.RequestAnimationFrame(step)
	return core.List{
		Row:           listRow,
		Count:         func() int { return listRows },
		EstimatedSize: listRowH,
		Ref:           ref,
		Style:         core.Style{FlexGrow: 1},
	}
}

func listRow(_ *core.Scope, row core.ListRow) core.Node {
	text := func(style core.Style, s func(i int) string) core.Node {
		return core.Text{Style: style, ContentFunc: func() string { return s(row.Index()) }}
	}
	return core.View{Style: rowStyle, Children: []core.Node{
		text(labelStyle, func(i int) string { return "Row " + strconv.Itoa(i) }),
		text(detailStyle, func(i int) string { return strconv.Itoa(i * 7 % 1000) }),
		text(detailStyle, func(i int) string { return strconv.Itoa(i*13%100) + " ms" }),
	}}
}

// gridRoot is the grid scene: one signal holds the frame number and all
// 600 cell texts read it, so each frame callback updates 600 texts.
func gridRoot(cx *core.Scope) core.Node {
	tick := core.NewSignal(cx, 0)
	var step func(time.Time)
	step = func(time.Time) {
		tick.Set(tick.GetUntracked() + 1)
		report(gridChange)
		cx.RequestAnimationFrame(step)
	}
	cx.RequestAnimationFrame(step)

	lines := make([]core.Node, 0, gridRows)
	for r := 0; r < gridRows; r++ {
		cells := make([]core.Node, 0, gridCols)
		for col := 0; col < gridCols; col++ {
			cells = append(cells, gridCell(r*gridCols+col, cellBg[(r+col)%len(cellBg)], tick))
		}
		lines = append(lines, core.View{Style: core.Style{Direction: core.Row}, Children: cells})
	}
	return core.View{Style: core.Style{FlexGrow: 1}, Children: lines}
}

func gridCell(cell int, bg core.Color, tick *core.Signal[int]) core.Node {
	return core.View{
		Style: core.Style{Background: bg, Width: core.Pt(cellW), Height: core.Pt(cellH),
			JustifyContent: core.JustifyCenter, AlignItems: core.AlignCenter},
		Children: []core.Node{core.Text{Style: cellText,
			ContentFunc: func() string { return strconv.Itoa(cellValue(cell, tick.Get())) }}},
	}
}

// cellValue is a cell's number at a frame: 17 is not a multiple of 10000,
// so every cell changes every frame.
func cellValue(cell, frame int) int { return (cell*131 + frame*17) % 10000 }
