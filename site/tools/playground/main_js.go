//go:build js && wasm

package main

import (
	"encoding/json"
	"image"
	"strings"
	"syscall/js"
	"time"
	"unicode/utf8"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

type browserWindow struct {
	host      js.Value
	size      geom.Size
	scale     float32
	pixels    js.Value
	pixelsLen int
	clipboard string
}

func (w *browserWindow) Size() geom.Size { return w.size }
func (w *browserWindow) Scale() float32  { return w.scale }
func (w *browserWindow) Invalidate()     { w.host.Call("requestFrame") }
func (w *browserWindow) InvalidateAt(t time.Time) {
	w.host.Call("wake", max(0, float64(time.Until(t).Milliseconds())))
}
func (w *browserWindow) SetCursor(c input.Cursor) {
	names := []string{"default", "pointer", "text", "not-allowed", "ew-resize", "ns-resize", "grab", "grabbing", "move"}
	if int(c) < len(names) {
		w.host.Call("cursor", names[c])
	}
}
func (w *browserWindow) ReadClipboard() string   { return w.clipboard }
func (w *browserWindow) WriteClipboard(s string) { w.clipboard = s; w.host.Call("clipboard", s) }
func (w *browserWindow) SetTextInput(on bool, r geom.Rect) {
	w.host.Call("textInput", on, r.X, r.Y, r.H)
}
func (w *browserWindow) Present(img *image.RGBA) error {
	if img == nil {
		return nil
	}
	if len(img.Pix) != w.pixelsLen {
		w.pixels = js.Global().Get("Uint8Array").New(len(img.Pix))
		w.pixelsLen = len(img.Pix)
	}
	// CPU output is premultiplied; Canvas ImageData expects straight RGBA.
	pixels := make([]byte, len(img.Pix))
	copy(pixels, img.Pix)
	for i := 0; i < len(pixels); i += 4 {
		a := uint32(pixels[i+3])
		if a > 0 && a < 255 {
			for j := 0; j < 3; j++ {
				pixels[i+j] = uint8(min(255, uint32(pixels[i+j])*255/a))
			}
		}
	}
	js.CopyBytesToJS(w.pixels, pixels)
	w.host.Call("present", w.pixels, img.Rect.Dx(), img.Rect.Dy())
	return nil
}

var runner *ui.Runner
var win *browserWindow
var demo *demoState
var bindings []js.Func

func bind(name string, fn func([]js.Value) any) {
	f := js.FuncOf(func(_ js.Value, args []js.Value) (result any) {
		defer func() {
			if r := recover(); r != nil {
				if win != nil {
					win.host.Call("error", "Go preview failed; reload the demo to retry.")
				}
				result = false
			}
		}()
		return fn(args)
	})
	bindings = append(bindings, f)
	js.Global().Get("tangoPreview").Set(name, f)
}

func selectedTheme(name string, dark bool) *theme.Theme {
	var opts []theme.Option
	if dark {
		opts = append(opts, theme.Dark())
	}
	switch name {
	case "element":
		return theme.ElementPlus(opts...)
	case "shadcn":
		if dark {
			return theme.Shadcn(theme.Dark())
		}
		return theme.Shadcn()
	default:
		return theme.AntDesign(opts...)
	}
}

func main() {
	js.Global().Set("tangoPreview", js.Global().Get("Object").New())
	bind("pages", func(_ []js.Value) any { data, _ := json.Marshal(livePages()); return string(data) })
	bind("start", func(a []js.Value) any {
		name := a[1].String()
		isGallery := strings.HasPrefix(name, "gallery:")
		if isGallery {
			page, ok := gallery.Find(strings.TrimPrefix(name, "gallery:"))
			if !ok || page.Render == nil {
				a[0].Call("error", "Unknown component")
				return false
			}
		}

		if runner != nil {
			runner.Close()
		}
		demo = newDemo(name)
		height := float32(340)
		if isGallery {
			height = 560
		}
		win = &browserWindow{host: a[0], size: geom.Size{W: float32(a[2].Float()), H: height}, scale: float32(a[3].Float())}
		runner = ui.NewRunner(demo.page, ui.RunnerOptions{Theme: selectedTheme(a[4].String(), a[5].Bool()), NoAnimations: !isGallery})
		runner.Attach(win)
		win.Invalidate()
		return true
	})
	bind("frame", func(_ []js.Value) any {
		if runner != nil {
			if err := runner.Frame(); err != nil {
				win.host.Call("error", err.Error())
				return false
			}
			summary := ""
			if strings.HasPrefix(demo.component, "gallery:") {
				if page, ok := gallery.Find(strings.TrimPrefix(demo.component, "gallery:")); ok && page.Status != nil {
					summary = page.Status()
				}
			}
			win.host.Call("state", demo.name, demo.language, demo.level, demo.clicks, demo.enabled, demo.tab, summary)
		}
		return true
	})
	bind("theme", func(a []js.Value) any {
		if runner != nil {
			runner.SetTheme(selectedTheme(a[0].String(), a[1].Bool()))
		}
		return true
	})
	bind("resize", func(a []js.Value) any {
		if win != nil {
			win.size.W = float32(a[0].Float())
			win.scale = float32(a[1].Float())
			win.Invalidate()
		}
		return true
	})
	bind("font", func(a []js.Value) any {
		data := make([]byte, a[0].Get("byteLength").Int())
		js.CopyBytesToGo(data, a[0])
		if err := text.RegisterFont("Preview custom font", data); err != nil {
			return err.Error()
		}
		if win != nil {
			win.Invalidate()
		}
		return ""
	})
	bind("event", func(a []js.Value) any {
		if runner == nil {
			return false
		}
		e := a[0]
		kind := e.Get("kind").String()
		mods := input.Modifiers(e.Get("mods").Int())
		switch kind {
		case "pointer":
			runner.HandleEvent(input.Pointer{Kind: input.PointerKind(e.Get("phase").Int()), Pos: geom.Pt(float32(e.Get("x").Float()), float32(e.Get("y").Float())), Button: input.Button(e.Get("button").Int()), Mods: mods, Clicks: e.Get("clicks").Int()})
		case "touch":
			runner.HandleEvent(input.TouchEvent{ID: int64(e.Get("id").Int()), Phase: input.TouchPhase(e.Get("phase").Int()), Pos: geom.Pt(float32(e.Get("x").Float()), float32(e.Get("y").Float())), Pressure: float32(e.Get("pressure").Float()), Time: time.Now()})
		case "key":
			runner.HandleEvent(input.KeyEvent{Key: browserKey(e.Get("code").String()), Down: e.Get("down").Bool(), Mods: mods, Repeat: e.Get("repeat").Bool()})
		case "text":
			runner.HandleEvent(input.TextInput{Text: e.Get("text").String()})
		case "composition":
			s := e.Get("text").String()
			runner.HandleEvent(input.Composition{Text: s, Cursor: utf8.RuneCountInString(s)})
		case "scroll":
			runner.HandleEvent(input.Scroll{Pos: geom.Pt(float32(e.Get("x").Float()), float32(e.Get("y").Float())), DX: float32(e.Get("dx").Float()), DY: float32(e.Get("dy").Float()), Mods: mods})
		case "focus":
			runner.HandleEvent(input.WindowFocus{Focused: e.Get("focused").Bool()})
		case "clipboard":
			win.clipboard = e.Get("text").String()
		}
		return true
	})
	bind("close", func(_ []js.Value) any {
		if runner != nil {
			runner.Close()
			runner = nil
		}
		return true
	})
	js.Global().Call("dispatchEvent", js.Global().Get("Event").New("tango-preview-ready"))
	select {}
}
