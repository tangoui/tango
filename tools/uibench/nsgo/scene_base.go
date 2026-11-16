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
	cellInk = core.RGB(0x101010)
	cellBg  = [4]core.Color{core.RGB(0xf6c9c4), core.RGB(0xf4e8b8), core.RGB(0xc6e9c8), core.RGB(0xc9d8f4)}
)

// counterRoot is a number and three buttons. Its one frame callback, after
// the first commit, reports ready; then it asks for no frames.
func counterRoot(cx *core.Scope) core.Node {
	n := core.NewSignal(cx, 0)
	cx.RequestAnimationFrame(func(time.Time) { report(0) })
	return core.View{
		Style: core.Style{FlexGrow: 1, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 16},
		Children: []core.Node{
			core.Text{
				ContentFunc: func() string { return strconv.Itoa(n.Get()) },
				Style:       core.Style{FontSize: 56, FontWeight: core.FontWeightBold},
			},
			core.View{
				Style: core.Style{Direction: core.Row, Gap: 8},
				Children: []core.Node{
					ui.Button{Text: "−", Variant: ui.ButtonSecondary, OnPress: func() { n.Update(func(v int) int { return v - 1 }) }},
					ui.Button{Text: "Reset", Variant: ui.ButtonSecondary, OnPress: func() { n.Set(0) }},
					ui.Button{Text: "+", OnPress: func() { n.Update(func(v int) int { return v + 1 }) }},
				},
			},
		},
	}
}

// listRoot is a virtual list of 100,000 rows of three texts, scrolled 8
// logical px further every frame.
func listRoot(cx *core.Scope) core.Node {
	list := &core.Ref{}
	offset := core.Points(0)
	var frame func(time.Time)
	frame = func(time.Time) {
		offset += listStep
		if offset > listRows*listRowH-viewH {
			offset = 0
		}
		list.ScrollTo(core.Point{Y: offset}, false)
		report(1)
		cx.RequestAnimationFrame(frame)
	}
	cx.RequestAnimationFrame(frame)
	return core.List{
		Ref:           list,
		Count:         func() int { return listRows },
		EstimatedSize: listRowH,
		Style:         core.Style{FlexGrow: 1},
		Row: func(cx *core.Scope, row core.ListRow) core.Node {
			return core.View{
				Style: core.Style{Direction: core.Row, Height: core.Pt(listRowH), AlignItems: core.AlignCenter, Gap: 12, Padding: core.Symmetric(0, 16)},
				Children: []core.Node{
					core.Text{ContentFunc: func() string { return "Row " + strconv.Itoa(row.Index()) }, Style: core.Style{FlexGrow: 1, FontSize: 14}},
					core.Text{ContentFunc: func() string { return strconv.Itoa(row.Index() * 7 % 1000) }, Style: core.Style{FontSize: 13, Color: muted}},
					core.Text{ContentFunc: func() string { return strconv.Itoa(row.Index()*13%100) + " ms" }, Style: core.Style{FontSize: 13, Color: muted}},
				},
			}
		},
	}
}

// gridRoot is 600 cells, each with its own background, whose numbers all
// change every frame: one signal of the frame, which every cell's text
// reads, so a frame updates 600 texts.
func gridRoot(cx *core.Scope) core.Node {
	tick := core.NewSignal(cx, 0)
	var frame func(time.Time)
	frame = func(time.Time) {
		tick.Set(tick.GetUntracked() + 1)
		report(gridChange)
		cx.RequestAnimationFrame(frame)
	}
	cx.RequestAnimationFrame(frame)

	rows := make([]core.Node, gridRows)
	for row := range rows {
		cells := make([]core.Node, gridCols)
		for col := range cells {
			cell := row*gridCols + col
			cells[col] = core.View{
				Style: core.Style{Width: core.Pt(cellW), Height: core.Pt(cellH), AlignItems: core.AlignCenter,
					JustifyContent: core.JustifyCenter, Background: cellBg[(row+col)%4]},
				Children: []core.Node{core.Text{
					ContentFunc: func() string { return strconv.Itoa(cellValue(cell, tick.Get())) },
					Style:       core.Style{FontSize: 11, Color: cellInk},
				}},
			}
		}
		rows[row] = core.View{Style: core.Style{Direction: core.Row}, Children: cells}
	}
	return core.View{Style: core.Style{FlexGrow: 1}, Children: rows}
}

// cellValue is cell's number at frame: every cell changes every frame.
func cellValue(cell, frame int) int { return (cell*7919 + frame*31) % 10000 }
