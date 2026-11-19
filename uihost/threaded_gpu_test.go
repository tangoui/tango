//go:build darwin && !ios

package uihost

import (
	"context"
	"fmt"
	"image"
	"math"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/darwin"
	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/uithread"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

const task7GUIChild = "TANGO_TASK7_RENDER_CHILD"

func init() {
	if os.Getenv(task7GUIChild) != "" {
		runtime.LockOSThread()
	}
}

func TestMain(m *testing.M) {
	if mode := os.Getenv(task7GUIChild); mode != "" {
		if err := runThreadedRender(mode); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestThreadedMetalPresentsFromUIThread(t *testing.T)  { task7RenderTest(t, "gpu") }
func TestThreadedCPUPresentsFromUIThread(t *testing.T)    { task7RenderTest(t, "cpu") }
func TestThreadedNativePresentsFromUIThread(t *testing.T) { task7RenderTest(t, "native") }
func TestPromoteToMetalFromUIThread(t *testing.T)         { task7RenderTest(t, "promote") }
func TestFallbackToCPUFromUIThread(t *testing.T)          { task7RenderTest(t, "fallback") }

func task7RenderTest(t *testing.T, mode string) {
	t.Helper()
	if os.Getenv("TANGO_TEST_TASK7_GUI") != "1" || testing.Short() {
		t.Skip("requires coordinated real window")
	}
	if (mode == "native" || mode == "promote") && runtime.GOARCH != "arm64" {
		t.Skip("native renderer is arm64 only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), task7GUIChild+"="+mode)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

type task7Canvas struct {
	platform.Canvas
	platform.CanvasThreaded
	platform.CanvasThreadedRelease
	platform.CanvasMetal
	platform.CanvasAccessible
	capture func(*image.RGBA)
}

func (*task7Canvas) MainThreadAccessibility() {}

func (c *task7Canvas) UseCoreGraphicsLayer() (uintptr, error) {
	if n, ok := c.Canvas.(platform.CanvasCoreGraphics); ok {
		return n.UseCoreGraphicsLayer()
	}
	return 0, platform.ErrUnsupported
}
func (c *task7Canvas) Present(img *image.RGBA) error {
	err := c.Canvas.Present(img)
	if err == nil {
		c.capture(img)
	}
	return err
}

type task7Frame struct {
	thread *uithread.Thread
	color  [3]byte
}

func runThreadedRender(mode string) error {
	renderer := mode
	if mode == "promote" {
		renderer = ""
	}
	if mode == "fallback" {
		renderer = "gpu"
	}
	if err := os.Setenv("TANGO_RENDERER", renderer); err != nil {
		return err
	}
	app := tango.New(tango.Options{Name: "Tango Task 7 render test"})
	frames := make(chan task7Frame, 128)
	metalFrames := make(chan task7Frame, 128)
	nativeFrames := make(chan *uithread.Thread, 128)
	var capture func(uint32) ([3]byte, error)
	if mode == "promote" || mode == "fallback" {
		var err error
		capture, err = task7WindowCapture()
		if err != nil {
			return err
		}
	}
	var sink platform.CanvasSink
	orig := corehook.NewCanvasSurface
	corehook.NewCanvasSurface = func(a any, opts any, s platform.CanvasSink) (any, platform.Canvas, error) {
		surface, cv, err := orig(a, opts, s)
		if err != nil {
			return surface, cv, err
		}
		sink = s
		cv = darwin.ThreadedCanvas(cv)
		ct, ok := cv.(platform.CanvasThreaded)
		if !ok {
			return nil, nil, fmt.Errorf("Darwin adapter unavailable")
		}
		return surface, &task7Canvas{Canvas: cv, CanvasThreaded: ct, CanvasThreadedRelease: cv.(platform.CanvasThreadedRelease), CanvasMetal: cv.(platform.CanvasMetal), CanvasAccessible: cv.(platform.CanvasAccessible), capture: func(img *image.RGBA) {
			if img != nil {
				px := img.RGBAAt(img.Rect.Dx()/2, img.Rect.Dy()/2)
				frames <- task7Frame{uithread.Current(), [3]byte{px.R, px.G, px.B}}
			}
		}}, nil
	}
	origGPU := newPresenter
	newPresenter = func(target uintptr) (paint.Renderer, func(image.Point, float32) error, error) {
		r, p, err := origGPU(target)
		if err != nil {
			return r, p, err
		}
		return r, func(size image.Point, scale float32) error {
			if mode == "fallback" {
				return gpu.ErrDeviceLost
			}
			if err := p(size, scale); err != nil {
				return err
			}
			im, err := r.(interface{ ReadPixels() (*image.RGBA, error) }).ReadPixels()
			if err == nil {
				px := im.RGBAAt(im.Rect.Dx()/2, im.Rect.Dy()/2)
				metalFrames <- task7Frame{uithread.Current(), [3]byte{px.R, px.G, px.B}}
			}
			return err
		}, nil
	}
	if nativeNewPresenter != nil {
		origNative := nativeNewPresenter
		nativeNewPresenter = func(layer uintptr) (paint.Renderer, func(image.Point, float32) error, error) {
			r, p, err := origNative(layer)
			if err != nil {
				return r, p, err
			}
			return r, func(size image.Point, scale float32) error {
				if err := p(size, scale); err != nil {
					return err
				}
				nativeFrames <- uithread.Current()
				return nil
			}, nil
		}
	}
	colors := []theme.Color{{R: 240, G: 50, B: 30, A: 255}, {R: 35, G: 220, B: 75, A: 255}, {R: 30, G: 60, B: 240, A: 255}}
	result := make(chan error, 1)
	var n int // only UI thread reads/writes after Open
	app.OnReady(func() {
		thm := theme.AntDesign()
		thm.BgLayout = colors[0]
		r, s, err := Open(app, Options{Theme: ThemeFixed, Locale: LocaleFixed, Surface: tango.SurfaceOptions{Title: "Task 7 pixels", Width: 350, Height: 300}}, func(c *ui.Context) {
			g := sink.(*host).gpu // UI thread; its current path is the fixture's trigger
			if mode == "promote" && g != nil && g.native != nil && g.native.ready {
				n = 1
			}
			if mode == "fallback" && g != nil && g.lost() {
				n = 1
			}
			c.Theme.BgLayout = colors[n]
		}, ui.RunnerOptions{Theme: thm, NoAnimations: true})
		if err != nil {
			result <- err
			app.Quit()
			return
		}
		t := threadedOf(r)
		if t == nil {
			result <- fmt.Errorf("no UI thread")
			app.Quit()
			return
		}
		window := uint32(task7Window().Send(objc.RegisterName("windowNumber")))
		go func() {
			err := func() error {
				waitFrame := func(ch <-chan task7Frame, i int) error {
					want := [3]byte{colors[i].R, colors[i].G, colors[i].B}
					timer := time.NewTimer(2 * time.Second)
					defer timer.Stop()
					for {
						select {
						case f := <-ch:
							if f.thread != t.th {
								return fmt.Errorf("present off UI thread")
							}
							if matchesTask7(f.color, want) {
								return nil
							}
						case <-timer.C:
							return fmt.Errorf("no frame with pixel %v", want)
						}
					}
				}
				if mode == "native" || mode == "promote" {
					select {
					case actual := <-nativeFrames:
						if actual != t.th {
							return fmt.Errorf("native present off UI thread")
						}
					case <-time.After(2 * time.Second):
						return fmt.Errorf("no native frame")
					}
					if mode == "native" {
						rgb := tango.Await(app, task7LayerPixel)
						if !matchesTask7(rgb, [3]byte{240, 50, 30}) {
							return fmt.Errorf("renderInContext pixel=%v", rgb)
						}
						return nil
					}
					tango.Await(app, func() struct{} { sink.CanvasInput(input.KeyEvent{Key: input.KeyA, Down: true}); return struct{}{} })
				}
				ch := metalFrames
				if mode == "cpu" || mode == "fallback" {
					ch = frames
				}
				first := 0
				if mode == "promote" || mode == "fallback" {
					first = 1
				}
				if err := waitFrame(ch, first); err != nil {
					return err
				}
				if mode == "gpu" {
					for i := 1; i < len(colors); i++ {
						r.Post(func() { n = i })
						if err := waitFrame(ch, i); err != nil {
							return err
						}
					}
				}
				if mode == "cpu" {
					rgb := tango.Await(app, task7LayerPixel)
					if !matchesTask7(rgb, [3]byte{240, 50, 30}) {
						return fmt.Errorf("CPU layer pixel=%v", rgb)
					}
				}
				if capture != nil {
					for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
						rgb, err := capture(window)
						if err == nil && matchesTask7(rgb, [3]byte{35, 220, 75}) {
							return nil
						}
					}
					return fmt.Errorf("first switched frame did not reach window capture")
				}
				return nil
			}()
			result <- err
			app.Post(func() { s.Close(); app.Quit() })
		}()
	})
	if err := app.Run(); err != nil {
		return err
	}
	select {
	case err := <-result:
		return err
	default:
		return fmt.Errorf("app quit without test result")
	}
}

func matchesTask7(a, b [3]byte) bool {
	for i := range a {
		if d := int(a[i]) - int(b[i]); d < -6 || d > 6 {
			return false
		}
	}
	return true
}

type task7Rect struct{ X, Y, W, H float64 }

func task7Window() objc.ID {
	sel := objc.RegisterName
	return objc.ID(objc.GetClass("NSApplication")).Send(sel("sharedApplication")).Send(sel("windows")).Send(sel("firstObject"))
}

func task7Bitmap(draw func(uintptr)) [3]byte {
	lib, _ := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_NOW|purego.RTLD_LOCAL)
	var bitmap func(unsafe.Pointer, uintptr, uintptr, uintptr, uintptr, uintptr, uint32) uintptr
	var space func() uintptr
	var release, spaceRelease func(uintptr)
	purego.RegisterLibFunc(&bitmap, lib, "CGBitmapContextCreate")
	purego.RegisterLibFunc(&space, lib, "CGColorSpaceCreateDeviceRGB")
	purego.RegisterLibFunc(&release, lib, "CGContextRelease")
	purego.RegisterLibFunc(&spaceRelease, lib, "CGColorSpaceRelease")
	cs := space()
	defer spaceRelease(cs)
	var px [4]byte
	ctx := bitmap(unsafe.Pointer(&px[0]), 1, 1, 8, 4, cs, 1|4<<12)
	if ctx == 0 {
		return [3]byte{}
	}
	draw(ctx)
	release(ctx)
	return [3]byte{px[0], px[1], px[2]}
}

func task7LayerPixel() [3]byte {
	sel := objc.RegisterName
	layer := task7Window().Send(sel("contentView")).Send(sel("subviews")).Send(sel("lastObject")).Send(sel("layer"))
	lib, _ := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_NOW|purego.RTLD_LOCAL)
	var translate func(uintptr, float64, float64)
	purego.RegisterLibFunc(&translate, lib, "CGContextTranslateCTM")
	return task7Bitmap(func(ctx uintptr) { translate(ctx, -120, -120); layer.Send(sel("renderInContext:"), ctx) })
}

func task7WindowCapture() (func(uint32) ([3]byte, error), error) {
	lib, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil, err
	}
	var permission func() bool
	purego.RegisterLibFunc(&permission, lib, "CGPreflightScreenCaptureAccess")
	if !permission() {
		return nil, fmt.Errorf("Screen Recording permission required; not prompting")
	}
	var capture func(task7Rect, uint32, uint32, uint32) uintptr
	addr, err := purego.Dlsym(lib, "CGWindowListCreateImage")
	if err != nil {
		return nil, err
	}
	purego.RegisterFunc(&capture, addr)
	var release func(uintptr)
	var draw func(uintptr, task7Rect, uintptr)
	purego.RegisterLibFunc(&release, lib, "CGImageRelease")
	purego.RegisterLibFunc(&draw, lib, "CGContextDrawImage")
	return func(id uint32) ([3]byte, error) {
		// CGRectNull; include this window only; ignore framing/shadow.
		img := capture(task7Rect{X: math.Inf(1), Y: math.Inf(1)}, 1<<3, id, 1|1<<4)
		if img == 0 {
			return [3]byte{}, fmt.Errorf("CGWindowListCreateImage returned NULL")
		}
		defer release(img)
		return task7Bitmap(func(ctx uintptr) { draw(ctx, task7Rect{X: -10, Y: -10, W: 21, H: 21}, img) }), nil
	}, nil
}
