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

// counter is a number and three buttons. After its first frame is on
// screen it reports ready and asks for no more frames.
type counter struct {
	n int
	k clock
}

func (s *counter) view(c *ui.Context) {
	if s.k.next(c) {
		switch s.k.frames {
		case 1:
			c.AnimationFrame() // one more frame, after the first is presented
		case 2:
			report(0)
		}
	}
	probe(ui.Column(c).Fill().Center().Gap(16)).Children(func() {
		ui.Text(c, strconv.Itoa(s.n)).FontSize(56).Bold()
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c.Key("decrement"), "−").Clicked() {
				s.n--
			}
			if ui.Button(c.Key("reset"), "Reset").Clicked() {
				s.n = 0
			}
			if ui.PrimaryButton(c.Key("increment"), "+").Clicked() {
				s.n++
			}
		})
	})
}

// list is a virtual list of 100,000 rows of three texts, scrolled 8
// logical px further every frame through its tracked scroll state.
type list struct {
	k      clock
	state  ui.ListState
	scroll ui.ScrollState
}

func (s *list) view(c *ui.Context) {
	c.AnimationFrame() // MyGo's per-frame request while something moves
	advance := s.k.next(c) && s.k.frames >= 2
	if advance {
		s.scroll.Y += listStep
		if s.scroll.Y > listRows*listRowH-viewH {
			s.scroll.Y = 0
		}
	}
	probe(ui.List(c, &s.state, listRows, func(i int) {
		ui.Row(c).Height(listRowH).Padding(0, 16).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Row "+strconv.Itoa(i)).FontSize(14).Grow(1).NoWrap()
			ui.Text(c, strconv.Itoa(i*7%1000)).FontSize(13).TextColor(muted).NoWrap()
			ui.Text(c, strconv.Itoa(i*13%100)+" ms").FontSize(13).TextColor(muted).NoWrap()
		})
	}).Fill().TrackScroll(&s.scroll))
	if advance {
		report(1)
	}
}

// grid is 600 cells, each with its own background, whose numbers all
// change every frame.
type grid struct {
	k     clock
	frame int
}

func (s *grid) view(c *ui.Context) {
	c.AnimationFrame() // MyGo's per-frame request while something moves
	advance := s.k.next(c) && s.k.frames >= 2
	if advance {
		s.frame++
	}
	probe(ui.Column(c).Fill()).Children(func() {
		for row := range gridRows {
			ui.Row(c).Children(func() {
				for col := range gridCols {
					cell := row*gridCols + col
					ui.Box(c).Size(cellW, cellH).Center().Background(cellBg[(row+col)%4]).Children(func() {
						ui.Text(c, strconv.Itoa(cellValue(cell, s.frame))).FontSize(11).TextColor(cellInk).NoWrap()
					})
				}
			})
		}
	})
	if advance {
		report(gridChange)
	}
}

// cellValue is cell's number at frame: every cell changes every frame.
func cellValue(cell, frame int) int { return (cell*7919 + frame*31) % 10000 }
