package tango

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestDragFiles(t *testing.T) {
	a, fb := newTestApp(t)
	s, err := a.NewSurface(SurfaceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(f1, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	ops := make(chan DragOperation, 2)
	if err := s.DragFiles([]string{f1, dir + "/"}, []byte{1}, func(op DragOperation) { ops <- op }); err != nil {
		t.Fatal(err)
	}
	drags := fb.FileDrags()
	if len(drags) != 1 || !slices.Equal(drags[0].Paths, []string{f1, dir}) || drags[0].Surface != s.ID() || len(drags[0].Icon) != 1 {
		t.Fatalf("%+v", drags)
	}
	drags[0].Finish(fb, DragCopy)
	if op := <-ops; op != DragCopy {
		t.Fatal(op)
	}
	if drags[0].Finish(fb, DragMove) {
		t.Fatal("finished twice")
	}

	if err := s.DragFiles(nil, nil, nil); !errors.Is(err, ErrNoFiles) {
		t.Fatal(err)
	}
	if err := s.DragFiles([]string{"rel.txt"}, nil, nil); err == nil {
		t.Fatal("relative path accepted")
	}
	if err := s.DragFiles([]string{filepath.Join(dir, "missing")}, nil, nil); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	boom := errors.New("no drag")
	fb.SetDragErr(boom)
	if err := s.DragFiles([]string{f1}, nil, nil); err != boom {
		t.Fatal(err)
	}
}
