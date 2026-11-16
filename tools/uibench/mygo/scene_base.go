package main

import (
	"strconv"

	"github.com/egoist/mygo/ui"
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
	muted   = ui.Hex("#6b7280")
	cellInk = ui.Hex("#101010")
	cellBg  = [4]ui.Color{ui.Hex("#f6c9c4"), ui.Hex("#f4e8b8"), ui.Hex("#c6e9c8"), ui.Hex("#c9d8f4")}
)

// counter shows a caption, a tap count and Less / Reset / More. It
// reports ready on the frame after the first and then requests none.
type counter struct {
	taps int
	k    clock
}

func (s *counter) view(c *ui.Context) {
	if s.k.next(c) {
		if s.k.frames == 1 {
			c.AnimationFrame() // the frame after the first is presented is ready
		} else if s.k.frames == 2 {
			report(0)
		}
	}
	probe(ui.Column(c).Center().Fill().Gap(12)).Children(func() {
		ui.Text(c, "Taps").TextColor(muted).FontSize(13)
		ui.Text(c, strconv.Itoa(s.taps)).Bold().FontSize(48)
		ui.Row(c).Gap(10).Children(func() { s.buttons(c) })
	})
}

func (s *counter) buttons(c *ui.Context) {
	if ui.Button(c.Key("taps-less"), "Less").Clicked() {
		s.taps--
	}
	if ui.Button(c.Key("taps-reset"), "Reset").Clicked() {
		s.taps = 0
	}
	if ui.PrimaryButton(c.Key("taps-more"), "More").Clicked() {
		s.taps++
	}
}

// scrollingList is the list scene: 100,000 rows, its scroll position moved
// 8 logical px each frame through the state the list tracks.
type scrollingList struct {
	k      clock
	offset ui.ScrollState
	items  ui.ListState
}

func (s *scrollingList) view(c *ui.Context) {
	c.AnimationFrame() // MyGo's per-frame request while something moves
	moved := s.k.next(c) && s.k.frames >= 2
	if moved {
		if s.offset.Y += listStep; s.offset.Y > listRows*listRowH-viewH {
			s.offset.Y = 0
		}
	}
	probe(ui.List(c, &s.items, listRows, func(i int) { listRow(c, i) }).TrackScroll(&s.offset).Fill())
	if moved {
		report(1)
	}
}

func listRow(c *ui.Context, i int) {
	ui.Row(c).AlignItems(ui.Center).Gap(12).Padding(0, 16).Height(listRowH).Children(func() {
		ui.Text(c, "Row "+strconv.Itoa(i)).Grow(1).FontSize(14).NoWrap()
		ui.Text(c, strconv.Itoa(i*7%1000)).TextColor(muted).FontSize(13).NoWrap()
		ui.Text(c, strconv.Itoa(i*13%100)+" ms").TextColor(muted).FontSize(13).NoWrap()
	})
}

// dashboard is the grid scene: 30 rows of 20 cells, rebuilt every frame
// with all 600 numbers new.
type dashboard struct {
	k    clock
	tick int
}

func (s *dashboard) view(c *ui.Context) {
	c.AnimationFrame() // MyGo's per-frame request while something moves
	moved := s.k.next(c) && s.k.frames >= 2
	if moved {
		s.tick++
	}
	probe(ui.Column(c).Fill()).Children(func() {
		for r := 0; r < gridRows; r++ {
			ui.Row(c).Children(func() {
				for col := 0; col < gridCols; col++ {
					gridCell(c, cellValue(r*gridCols+col, s.tick), cellBg[(r+col)%len(cellBg)])
				}
			})
		}
	})
	if moved {
		report(gridChange)
	}
}

func gridCell(c *ui.Context, value int, bg ui.Color) {
	ui.Box(c).Background(bg).Width(cellW).Height(cellH).Center().Children(func() {
		ui.Text(c, strconv.Itoa(value)).TextColor(cellInk).FontSize(11).NoWrap()
	})
}

// cellValue is a cell's number at a frame: 17 is not a multiple of 10000,
// so every cell changes every frame.
func cellValue(cell, frame int) int { return (cell*131 + frame*17) % 10000 }
