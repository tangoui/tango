//go:build darwin

package editor

import (
	"bytes"
	"image"
	"os"
	"path/filepath"
	"runtime/pprof"
	"sort"
	"strings"
	"testing"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/layout"
)

// inputProfileDevice observes real uploads without replacing their execution.
type inputProfileDevice struct {
	*syncDevice
	sizes                    map[int]image.Point
	active                   bool
	frames, uploads, matches int
	uploadSizes, freeSizes   map[image.Point]int
}

func (d *inputProfileDevice) Retained(size image.Point) bool { return d.d.Retained(size) }

func (d *inputProfileDevice) TrimImages() { d.d.TrimImages() }

func (d *inputProfileDevice) Execute(f *gpu.Frame) error {
	candidates := map[image.Point]int{}
	for _, id := range f.ImageFrees {
		if size, ok := d.sizes[id]; ok {
			candidates[size]++
			if d.active {
				d.freeSizes[size]++
			}
			delete(d.sizes, id)
		}
	}
	for _, u := range f.ImageUploads {
		if d.active {
			d.uploads++
			d.uploadSizes[u.Size]++
			if d.sizes[u.ID] == u.Size || candidates[u.Size] > 0 {
				d.matches++
				if d.sizes[u.ID] != u.Size {
					candidates[u.Size]--
				}
			}
		}
		d.sizes[u.ID] = u.Size
	}
	if d.active {
		d.frames++
	}
	return d.syncDevice.Execute(f)
}

// TestEditorWarmInputProfile excludes construction, highlighting and navigation
// from the CPU profile. Opt in with TANGO_EDITOR_INPUT_PROFILE=<output directory>.
// Removing input dispatch must fail the exact-content assertion below.
func TestEditorWarmInputProfile(t *testing.T) {
	dir := os.Getenv("TANGO_EDITOR_INPUT_PROFILE")
	if dir == "" {
		t.Skip("set TANGO_EDITOR_INPUT_PROFILE to a profile output directory")
	}
	data, err := os.ReadFile(perfFile(t, 20000))
	if err != nil {
		t.Fatal(err)
	}
	st := NewState(string(data), "profile.ts", theme.Base("vs"))
	defer st.Close()
	h := &perfHost{st: st, w: &perfWindow{size: perfSize, scale: 2}}
	h.dev = &syncDevice{d: metalDevice(t)}
	d := &inputProfileDevice{syncDevice: h.dev, sizes: map[int]image.Point{}, uploadSizes: map[image.Point]int{}, freeSizes: map[image.Point]int{}}
	ren := gpu.NewRenderer(d)
	ren.SetVerify(false)
	h.r = ui.NewRunner(func(c *ui.Context) {
		h.vw = View(c, st).Layout(layout.Grow(0), layout.H(perfSize.H-1))
	}, ui.RunnerOptions{Renderer: ren, NoAnimations: true})
	defer h.r.Close()
	h.r.Attach(h.w)
	h.frame(t)
	highlightAllBackground(t, st)
	h.frame(t)
	h.focus(t)
	h.caretTo(t, 10000)
	start := st.Snapshot()
	at := st.Sels[0].Head
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, "warm-input.pprof"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := pprof.StartCPUProfile(f); err != nil {
		t.Fatal(err)
	}
	d.active = true
	previous := map[int]*image.RGBA{}
	replacements, identical := 0, 0
	for i := range 1000 {
		clear(previous)
		for id, strip := range h.vw.vs.mini.strips {
			previous[id] = strip.img
		}
		h.typeKey(keyAt(i))
		h.frame(t)
		for id, strip := range h.vw.vs.mini.strips {
			old := previous[id]
			if old != nil && strip.img != nil && old != strip.img {
				replacements++
				if old.Rect == strip.img.Rect && bytes.Equal(old.Pix, strip.img.Pix) {
					identical++
				}
			}
		}
	}
	d.active = false
	pprof.StopCPUProfile()
	snap := st.Snapshot()
	if snap.Len() != start.Len()+1000 || snap.LineCount() != start.LineCount()+25 || snap.Slice(at, at+1000) != strings.Repeat(strings.Repeat("x", 39)+"\n", 25) {
		t.Fatal("profile workload did not accept exactly 1000 keys")
	}
	if !st.Options.Minimap.Enabled || !st.Options.Minimap.RenderCharacters || !st.Options.Guides.Indentation || st.Highlighter() == nil {
		t.Fatal("profile lost editor features")
	}
	t.Logf("warm frames=%d image uploads=%d exact-size same-frame/same-ID opportunities=%d", d.frames, d.uploads, d.matches)
	t.Logf("strip replacements=%d identical pixels=%d", replacements, identical)
	for _, hist := range []struct {
		name string
		m    map[image.Point]int
	}{{"uploads", d.uploadSizes}, {"frees", d.freeSizes}} {
		var sizes []image.Point
		for size := range hist.m {
			sizes = append(sizes, size)
		}
		sort.Slice(sizes, func(i, j int) bool { return hist.m[sizes[i]] > hist.m[sizes[j]] })
		for _, size := range sizes[:min(12, len(sizes))] {
			t.Logf("%s size=%v count=%d", hist.name, size, hist.m[size])
		}
	}
}
