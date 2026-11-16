// Package proto is the line protocol every uibench scene program speaks on
// standard output, whatever framework draws it. The runner reads these
// lines; it never links the scene programs.
//
//	bench scene <name> <w>x<h>  the scene and its window, in points, at start
//	bench ready                 the first frame that did the scene's work
//	bench renderer <name> <ns>  the drawing path, with the time (Unix ns):
//	                            at ready, after every per-second report,
//	                            and as soon as it changes between frames
//	bench frames <fps>          frames of the last second, per second
//	bench work <n>              items changed in the last second, per second
//	bench intervals_us <a,b>    the gaps between those frames, in µs
//
// Earlier programs printed "bench renderer <name>" once, without a time;
// the runner still reads that line.
//
// Scene, Frame and Renderer must be called from one goroutine, the UI thread.
package proto

import (
	"bufio"
	"os"
	"strconv"
	"time"
)

var (
	out = bufio.NewWriterSize(os.Stdout, 16<<10)

	now = time.Now

	ready     bool
	renderer  string    // the path last reported to Renderer
	since     time.Time // start of the second being counted
	last      time.Time // previous frame
	frames    int
	work      int
	intervals []int64
	line      []byte
)

// Scene names the scene and its window size in points, once, at start.
func Scene(name string, width, height int) {
	out.WriteString("bench scene " + name + " " + strconv.Itoa(width) + "x" + strconv.Itoa(height) + "\n")
	out.Flush()
}

// Renderer gives the drawing path, such as Tango's uihost.Renderer. Call
// it every frame: a change after ready is reported at once, and the path
// is repeated at ready and with every per-second report.
func Renderer(name string) {
	if name == renderer {
		return
	}
	renderer = name
	if ready {
		writeRenderer(now())
		out.Flush()
	}
}

func writeRenderer(t time.Time) {
	if renderer == "" {
		return
	}
	line = append(line[:0], "bench renderer "...)
	line = append(line, renderer...)
	line = append(line, ' ')
	line = strconv.AppendInt(line, t.UnixNano(), 10)
	line = append(line, '\n')
	out.Write(line)
}

// Frame counts one frame that did the scene's work; work is how many items
// the frame changed. The first call reports ready and starts the clock.
func Frame(work int) { frameAt(time.Now(), work) }

func frameAt(now time.Time, n int) {
	if !ready {
		ready, since, last = true, now, now
		out.WriteString("bench ready\n")
		writeRenderer(now)
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
	writeRenderer(now)
	out.Flush()
	frames, work, intervals, since = 0, 0, intervals[:0], now
}
