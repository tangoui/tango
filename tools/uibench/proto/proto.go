// Package proto is the line protocol every uibench scene program speaks on
// standard output, whatever framework draws it. The runner reads these
// lines; it never links the scene programs.
//
//	bench renderer <name>     the drawing path the framework took
//	bench ready               the first frame that did the scene's work
//	bench frames <fps>        frames of the last second, per second
//	bench work <n>            items changed in the last second, per second
//	bench intervals_us <a,b>  the gaps between those frames, in µs
//
// Frame and Renderer must be called from one goroutine, the UI thread.
package proto

import (
	"bufio"
	"os"
	"strconv"
	"time"
)

var (
	out = bufio.NewWriterSize(os.Stdout, 16<<10)

	ready     bool
	since     time.Time // start of the second being counted
	last      time.Time // previous frame
	frames    int
	work      int
	intervals []int64
	line      []byte
)

// Renderer reports the drawing path, such as Tango's uihost.Renderer.
func Renderer(name string) {
	out.WriteString("bench renderer " + name + "\n")
	out.Flush()
}

// Frame counts one frame that did the scene's work; work is how many items
// the frame changed. The first call reports ready and starts the clock.
func Frame(work int) { frameAt(time.Now(), work) }

func frameAt(now time.Time, n int) {
	if !ready {
		ready, since, last = true, now, now
		out.WriteString("bench ready\n")
		out.Flush()
		return
	}
	intervals = append(intervals, now.Sub(last).Microseconds())
	last = now
	frames++
	work += n
	d := now.Sub(since)
	if d < time.Second {
		return
	}
	s := d.Seconds()
	line = append(line[:0], "bench frames "...)
	line = strconv.AppendFloat(line, float64(frames)/s, 'f', 1, 64)
	line = append(line, "\nbench work "...)
	line = strconv.AppendFloat(line, float64(work)/s, 'f', 1, 64)
	line = append(line, "\nbench intervals_us "...)
	for i, us := range intervals {
		if i > 0 {
			line = append(line, ',')
		}
		line = strconv.AppendInt(line, us, 10)
	}
	line = append(line, '\n')
	out.Write(line)
	out.Flush()
	frames, work, intervals, since = 0, 0, intervals[:0], now
}
