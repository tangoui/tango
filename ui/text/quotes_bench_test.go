package text

import (
	"fmt"
	"strconv"
	"testing"
)

// quotesMarket is the streaming quotes scene of the six-way benchmark
// (40 rows of symbol, price, change, percent and volume; mulberry32 from
// seed 42; a tick changes a quarter of the rows, drawn with replacement).
type quotesMarket struct {
	sym         [40]string
	open, price [40]float64
	vol         [40]int
	seed        uint32
}

func (q *quotesMarket) next() float64 {
	q.seed += 0x6d2b79f5
	t := q.seed
	t = (t ^ (t >> 15)) * (t | 1)
	t ^= t + (t^(t>>7))*(t|61)
	return float64(t^(t>>14)) / 4294967296.0
}

func newQuotesMarket() *quotesMarket {
	q := &quotesMarket{seed: 42}
	for i := range q.sym {
		var b [4]byte
		for j := range b {
			b[j] = byte('A' + int(q.next()*26))
		}
		px := 20 + q.next()*480
		q.sym[i], q.open[i], q.price[i], q.vol[i] = string(b[:]), px, px, int(q.next()*1000000)
	}
	return q
}

func (q *quotesMarket) tick() {
	for range len(q.sym) / 4 {
		i := int(q.next() * float64(len(q.sym)))
		q.price[i] *= 1 + (q.next()-0.5)*0.004
		q.vol[i] += int(q.next() * 500)
	}
}

// BenchmarkCTQuotesFrame lays out one frame of the quotes scene the way ui
// labels do (ui.Text: ShapeInto with the cell width, measured at the
// layout width and painted at width+0.5, the Layout reused per label):
// the title, 40 symbols and 160 numeric cells, a quarter of the rows
// changing every frame. Unchanged labels hit the Shape cache.
func BenchmarkCTQuotesFrame(b *testing.B) {
	UseSystemFonts(true)
	widths := [5]float32{80, 90, 80, 80, 100}
	cell := Style{Size: 13, LineHeight: 18}
	strong := cell
	strong.Weight = 600
	title := Style{Size: 18, LineHeight: 28, Weight: 600}
	q := newQuotesMarket()
	var lays [1 + 40*5]*Layout
	ticks := 0
	frame := func() {
		shape := func(k int, s string, st Style, w float32) {
			lays[k] = ShapeInto(lays[k], s, st, Options{MaxWidth: w})
			lays[k] = ShapeInto(lays[k], s, st, Options{MaxWidth: w + 0.5})
		}
		shape(0, fmt.Sprintf("Streaming quotes · tick %d", ticks), title, 528)
		for i := range q.sym {
			ch := q.price[i] - q.open[i]
			vals := [5]string{q.sym[i], strconv.FormatFloat(q.price[i], 'f', 2, 64), strconv.FormatFloat(ch, 'f', 2, 64),
				strconv.FormatFloat(ch/q.open[i]*100, 'f', 2, 64) + "%", strconv.Itoa(q.vol[i])}
			for j, s := range vals {
				st := cell
				if j == 0 {
					st = strong
				}
				shape(1+i*5+j, s, st, widths[j])
			}
		}
	}
	frame()
	b.ReportAllocs()
	for b.Loop() {
		q.tick()
		ticks++
		frame()
	}
}
