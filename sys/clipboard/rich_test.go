package clipboard_test

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"path/filepath"
	"slices"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/clipboard"
)

func pngBytes(t *testing.T) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestImage(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	if img, err := clipboard.ReadImage(a); img != nil || err != nil {
		t.Fatalf("empty clipboard: %v %v", img, err)
	}
	p := pngBytes(t)
	if err := clipboard.WriteImage(a, p); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fb.ClipboardImage(), p) {
		t.Fatal("backend image differs")
	}
	got, err := clipboard.ReadImage(a)
	if err != nil || !bytes.Equal(got, p) {
		t.Fatalf("%v", err)
	}
	if err := clipboard.WriteImage(a, []byte("not a png")); !errors.Is(err, clipboard.ErrNotPNG) {
		t.Fatal(err)
	}
}

func TestFiles(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	if fs, err := clipboard.ReadFiles(a); fs != nil || err != nil {
		t.Fatalf("%v %v", fs, err)
	}
	fb.SetClipboardFiles([]string{"/a", "/b c"})
	fs, err := clipboard.ReadFiles(a)
	if err != nil || !slices.Equal(fs, []string{"/a", "/b c"}) {
		t.Fatalf("%v %v", fs, err)
	}
}

func TestWriteFiles(t *testing.T) {
	a, _ := faketest.Run(t, tango.Options{})
	abs, _ := filepath.Abs("x.txt")
	if err := clipboard.WriteFiles(a, []string{abs}); err != nil {
		t.Fatal(err)
	}
	fs, err := clipboard.ReadFiles(a)
	if err != nil || !slices.Equal(fs, []string{abs}) {
		t.Fatalf("%v %v", fs, err)
	}
	if err := clipboard.WriteFiles(a, []string{"rel"}); !errors.Is(err, clipboard.ErrNotAbsolute) {
		t.Fatal(err)
	}
}

func TestRichErrors(t *testing.T) {
	a, fb := faketest.NewApp(tango.Options{})
	fb.ClipboardErr = errors.New("locked")
	faketest.Start(t, a, fb)
	if _, err := clipboard.ReadImage(a); err != fb.ClipboardErr {
		t.Fatal(err)
	}
	if _, err := clipboard.ReadFiles(a); err != fb.ClipboardErr {
		t.Fatal(err)
	}
	m, _ := faketest.RunMinimal(t, tango.Options{})
	if _, err := clipboard.ReadImage(m); !errors.Is(err, clipboard.ErrUnsupported) {
		t.Fatal(err)
	}
	if err := clipboard.WriteImage(m, pngBytes(t)); !errors.Is(err, clipboard.ErrUnsupported) {
		t.Fatal(err)
	}
	q, qb := faketest.NewApp(tango.Options{})
	q.Quit()
	_ = q.Run()
	<-qb.Done()
	if _, err := clipboard.ReadFiles(q); !errors.Is(err, tango.ErrNotRunning) {
		t.Fatal(err)
	}
}
