package macoslite

import (
	"errors"
	"runtime"
	"testing"
)

type nopView struct{}

func (nopView) Draw(*Frame, *Canvas) {}
func (nopView) Mouse(MouseEvent)     {}
func (nopView) Key(KeyEvent)         {}
func (nopView) Close()               {}

func TestNewValidates(t *testing.T) {
	valid := Options{Name: "Lite", Title: "Lite Counter", Width: 360, Height: 260}
	for name, o := range map[string]Options{
		"empty title": {Title: "", Width: 360, Height: 260},
		"zero width":  {Title: "x", Width: 0, Height: 260},
		"neg height":  {Title: "x", Width: 360, Height: -1},
	} {
		if _, err := New(o, nopView{}); !errors.Is(err, ErrInvalidOptions) {
			t.Errorf("%s: err = %v, want ErrInvalidOptions", name, err)
		}
	}
	if _, err := New(valid, nil); !errors.Is(err, ErrNilView) {
		t.Errorf("nil view: err = %v, want ErrNilView", err)
	}
	if runtime.GOOS == "darwin" {
		a, err := New(valid, nopView{})
		if err != nil || a == nil {
			t.Fatalf("valid: %v, %v", a, err)
		}
	}
}

func TestRectContains(t *testing.T) {
	r := Rect{X: 10, Y: 10, W: 20, H: 20}
	if !r.Contains(Point{10, 10}) || !r.Contains(Point{29.9, 29.9}) {
		t.Fatal("rect must contain its top-left and interior points")
	}
	if r.Contains(Point{30, 30}) {
		t.Fatal("rect must exclude its bottom-right edge")
	}
}
