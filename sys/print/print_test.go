package print_test

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/print"
)

func surface(t *testing.T, a *tango.App) *tango.Surface {
	t.Helper()
	s, err := a.NewSurface(tango.SurfaceOptions{URL: "/"})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPage(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	s := surface(t, a)
	if err := print.Page(s, print.Options{Title: "Invoice", Copies: 2}); err != nil {
		t.Fatal(err)
	}
	jobs := fb.PrintJobs()
	if len(jobs) != 1 || jobs[0].Surface != s.ID() || jobs[0].PDF != nil ||
		jobs[0].Opts != (platform.PrintOptions{Title: "Invoice", Copies: 2}) {
		t.Fatalf("%+v", jobs)
	}
	fb.SetPrintErr(print.ErrCanceled)
	if err := print.Page(s, print.Options{}); !errors.Is(err, print.ErrCanceled) {
		t.Fatal(err)
	}
	// From the main thread too.
	if err := tango.Await(a, func() error { return print.Page(s, print.Options{}) }); !errors.Is(err, print.ErrCanceled) {
		t.Fatal(err)
	}
	if err := print.Page(s, print.Options{Copies: -1}); err == nil {
		t.Fatal("negative copies accepted")
	}
	if err := print.Page(nil, print.Options{}); err == nil {
		t.Fatal("nil surface accepted")
	}
	surface(t, a) // keep a window open: closing the last one quits on some platforms
	s.Close()
	fb.SetPrintErr(nil)
	if err := print.Page(s, print.Options{}); !errors.Is(err, print.ErrUnsupported) {
		t.Fatalf("closed surface: %v", err)
	}
}

func TestPDFAndFile(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	s := surface(t, a)
	doc := []byte("%PDF-1.4\n%%EOF\n")
	if err := print.PDF(a, doc, print.Options{Parent: s, Copies: 3}); err != nil {
		t.Fatal(err)
	}
	if err := print.PDF(a, []byte("hello"), print.Options{}); err == nil {
		t.Fatal("non-PDF accepted")
	}
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "a.pdf")
	os.WriteFile(pdfPath, doc, 0o600)
	if err := print.File(a, pdfPath, print.Options{}); err != nil {
		t.Fatal(err)
	}
	pngPath := filepath.Join(dir, "pic.png")
	var buf bytes.Buffer
	png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 3)))
	os.WriteFile(pngPath, buf.Bytes(), 0o600)
	if err := print.File(a, pngPath, print.Options{}); err != nil {
		t.Fatal(err)
	}
	txt := filepath.Join(dir, "x.txt")
	os.WriteFile(txt, []byte("text"), 0o600)
	if err := print.File(a, txt, print.Options{}); err == nil {
		t.Fatal("text file accepted")
	}
	if err := print.File(a, filepath.Join(dir, "missing.pdf"), print.Options{}); err == nil {
		t.Fatal("missing file accepted")
	}

	jobs := fb.PrintJobs()
	if len(jobs) != 3 {
		t.Fatalf("%d jobs", len(jobs))
	}
	if jobs[0].Surface != s.ID() || !bytes.Equal(jobs[0].PDF, doc) || jobs[0].Opts.Copies != 3 {
		t.Fatalf("%+v", jobs[0])
	}
	if jobs[1].Surface != 0 || !bytes.Equal(jobs[1].PDF, doc) {
		t.Fatalf("%+v", jobs[1])
	}
	if !print.IsPDF(jobs[2].PDF) || jobs[2].Opts.Title != "pic.png" {
		t.Fatalf("image job: %q %+v", jobs[2].PDF[:min(len(jobs[2].PDF), 8)], jobs[2].Opts)
	}
}

func TestSavePDF(t *testing.T) {
	a, _ := faketest.Run(t, tango.Options{})
	s := surface(t, a)
	path := filepath.Join(t.TempDir(), "page.pdf")
	if err := print.SavePDF(s, path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !print.IsPDF(data) {
		t.Fatalf("%q %v", data, err)
	}
	if err := print.SavePDF(s, ""); err == nil {
		t.Fatal("empty path accepted")
	}
}

func TestUnsupportedAndQuit(t *testing.T) {
	m, _ := faketest.RunMinimal(t, tango.Options{})
	if err := print.PDF(m, []byte("%PDF-1.4"), print.Options{}); !errors.Is(err, print.ErrUnsupported) {
		t.Fatal(err)
	}
	if err := print.Page(surface(t, m), print.Options{}); !errors.Is(err, print.ErrUnsupported) {
		t.Fatal(err)
	}
	a, fb := faketest.NewApp(tango.Options{})
	a.Quit()
	_ = a.Run()
	<-fb.Done()
	if err := print.PDF(a, []byte("%PDF-1.4"), print.Options{}); !errors.Is(err, tango.ErrNotRunning) {
		t.Fatal(err)
	}
}

// TestImagePDF checks the document structure: every xref offset points at
// its object, and the image stream has the expected size.
func TestImagePDF(t *testing.T) {
	img := image.NewNRGBA(image.Rect(10, 20, 13, 22)) // 3x2, offset bounds
	img.Set(10, 20, color.NRGBA{255, 0, 0, 255})
	img.Set(11, 20, color.NRGBA{0, 0, 0, 0}) // transparent: white
	doc, err := print.ImagePDF(img, image.NewGray(image.Rect(0, 0, 96, 48)))
	if err != nil {
		t.Fatal(err)
	}
	if !print.IsPDF(doc) || !bytes.HasSuffix(doc, []byte("%%EOF\n")) {
		t.Fatal("header / trailer")
	}
	m := regexp.MustCompile(`startxref\n(\d+)\n`).FindSubmatch(doc)
	if m == nil {
		t.Fatal("no startxref")
	}
	xref, _ := strconv.Atoi(string(m[1]))
	if !bytes.HasPrefix(doc[xref:], []byte("xref\n0 9\n")) {
		t.Fatalf("xref at %d: %q", xref, doc[xref:xref+12])
	}
	entries := regexp.MustCompile(`(\d{10}) 00000 n \n`).FindAllSubmatch(doc[xref:], -1)
	if len(entries) != 8 {
		t.Fatalf("%d xref entries", len(entries))
	}
	for i, e := range entries {
		off, _ := strconv.Atoi(string(e[1]))
		want := strconv.Itoa(i+1) + " 0 obj\n"
		if !bytes.HasPrefix(doc[off:], []byte(want)) {
			t.Fatalf("object %d at %d: %q", i+1, off, doc[off:off+10])
		}
	}
	if !bytes.Contains(doc, []byte("/MediaBox [0 0 72.00 36.00]")) {
		t.Fatal("second page is not 96x48 px at 96 dpi")
	}
	if _, err := print.ImagePDF(); err == nil {
		t.Fatal("no images accepted")
	}
	if _, err := print.ImagePDF(image.NewRGBA(image.Rect(0, 0, 0, 5))); err == nil {
		t.Fatal("empty image accepted")
	}
}
