package main

import (
	"strconv"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
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
	muted   = theme.Hex("#6b7280")
	cellInk = theme.Hex("#101010")
	cellBg  = [4]theme.Color{theme.Hex("#f6c9c4"), theme.Hex("#f4e8b8"), theme.Hex("#c6e9c8"), theme.Hex("#c9d8f4")}
)

// counterPage is a number and three buttons. After its first frame is on
// screen it reports ready and asks for no more frames.
func counterPage() func(*ui.Context) {
	n := 0
	var k clock
	return func(c *ui.Context) {
		if k.next(c) {
			switch k.frames {
			case 1:
				c.WakeAt(c.Now()) // one more frame, after the first is presented
			case 2:
				report(0)
			}
		}
		ui.Column(c, func() {
			ui.Text(c, strconv.Itoa(n)).Size(56).Weight(text.Bold)
			ui.Row(c, func() {
				if ui.Button(c, "−").Clicked() {
					n--
				}
				if ui.Button(c, "Reset").Clicked() {
					n = 0
				}
				if ui.Button(c, "+").Primary().Clicked() {
					n++
				}
			}, layout.Gap(8))
		}, layout.Fill(), layout.Center(), layout.Gap(16))
	}
}

// listPage is a virtual list of 100,000 rows of three texts, scrolled 8
// logical px further every frame. Listy is Tango's virtual list that
// scrolls to a pixel offset; VirtualList only scrolls to whole rows.
func listPage() func(*ui.Context) {
	var k clock
	offset := 0.0
	maxOffset := float64(listRows*listRowH - viewH)
	return func(c *ui.Context) {
		c.WakeAt(c.Now())
		advance := k.next(c) && k.frames >= 2
		if advance {
			offset += listStep
			if offset > maxOffset {
				offset = 0
			}
		}
		ui.Listy(c, listRows, func(i int) {
			ui.Row(c, func() {
				ui.Text(c, "Row "+strconv.Itoa(i)).Size(14).Layout(layout.Grow(1))
				ui.Text(c, strconv.Itoa(i*7%1000)).Size(13).Color(muted)
				ui.Text(c, strconv.Itoa(i*13%100)+" ms").Size(13).Color(muted)
			}, layout.Gap(12), layout.Middle())
		}).Virtual().Height(viewH).ItemHeight(listRowH).Split(false).RowHoverable(false).
			ScrollTo(offset).Show()
		if advance {
			report(1)
		}
	}
}

// gridPage is 600 cells, each with its own background, whose numbers all
// change every frame.
func gridPage() func(*ui.Context) {
	var k clock
	frame := 0
	return func(c *ui.Context) {
		c.WakeAt(c.Now())
		advance := k.next(c) && k.frames >= 2
		if advance {
			frame++
		}
		ui.Column(c, func() {
			for row := range gridRows {
				ui.Row(c, func() {
					for col := range gridCols {
						cell := row*gridCols + col
						ui.Div(c, layout.Size(cellW, cellH), layout.Center()).Bg(cellBg[(row+col)%4]).Body(func() {
							ui.Text(c, strconv.Itoa(cellValue(cell, frame))).Size(11).Color(cellInk)
						})
					}
				})
			}
		}, layout.Fill())
		if advance {
			report(gridChange)
		}
	}
}

// cellValue is cell's number at frame: every cell changes every frame.
func cellValue(cell, frame int) int { return (cell*7919 + frame*31) % 10000 }
