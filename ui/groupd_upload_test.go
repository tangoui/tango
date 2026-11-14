package ui

import (
	"context"
	"image"
	"sync"
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/theme"
)

func TestUploadManualSubmit(t *testing.T) {
	dir := t.TempDir()
	a := p2dFile(t, dir, "a.txt", 10)
	b := p2dFile(t, dir, "b.txt", 20)
	var files []UploadFile
	var calls []string
	var mu sync.Mutex
	submit := false
	started := -1
	d := p2dDriver(theme.AntDesign(), func(c *Context) {
		w := Upload(c, &files).AutoUpload(false).Multiple().
			Picker(func(string, bool, bool) ([]string, error) { return []string{a, b}, nil }).
			CustomRequest(func(ctx context.Context, f UploadFile, progress func(float64)) ([]byte, error) {
				mu.Lock()
				calls = append(calls, f.Name)
				mu.Unlock()
				progress(100)
				return []byte("ok"), nil
			})
		if submit {
			submit = false
			started = w.Submit()
		}
	})
	if err := d.ClickLabel(ZhCN.Upload.Button); err != nil {
		t.Fatal(err)
	}
	p2dWait(t, d, "queued files", func() bool { return len(files) == 2 })
	for _, f := range files {
		if f.Status != "ready" {
			t.Fatalf("queued file status %q", f.Status)
		}
	}
	if len(calls) != 0 {
		t.Fatalf("uploaded before Submit: %v", calls)
	}
	submit = true
	d.Frame()
	if started != 2 {
		t.Fatalf("Submit started %d", started)
	}
	p2dWait(t, d, "uploads done", func() bool { return files[0].Status == "done" && files[1].Status == "done" })
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 2 {
		t.Fatalf("calls %v", calls)
	}
	// Nothing left to submit.
	submit = true
	d.Frame()
	if started != 0 {
		t.Fatalf("second Submit started %d", started)
	}
}

func TestUploadPictureList(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn()} {
		files := []UploadFile{
			{Name: "photo.png", Status: "done", Thumb: img},
			{Name: "doc.pdf", Status: "uploading", Percent: 40},
			{Name: "bad.png", Status: "error"},
		}
		previewed := -1
		removed := -1
		d := p2dDriver(th, func(c *Context) {
			w := Upload(c, &files).ListType(UploadListPicture)
			if i := w.Previewed(); i >= 0 {
				previewed = i
			}
			if i := w.Removed(); i >= 0 {
				removed = i
			}
		})
		d.EnableAccessibility()
		d.Settle()
		tk := th.Components.Upload
		r, ok := d.Find("photo.png")
		if !ok {
			t.Fatalf("%s: picture row missing", th.Name)
		}
		if r.H != tk.PictureItemHeight {
			t.Fatalf("%s: row height %v, want %v", th.Name, r.H, tk.PictureItemHeight)
		}
		item, ok := d.A11yFind(a11y.RoleListItem, "doc.pdf")
		if !ok || item.States&a11y.Busy == 0 {
			t.Fatalf("%s: uploading item %+v", th.Name, item)
		}
		if t2bCount(d, tk.ErrorColor) == 0 {
			t.Fatalf("%s: failed file not in the error color", th.Name)
		}
		if err := d.ClickLabel(ZhCN.Upload.Preview); err != nil {
			t.Fatal(err)
		}
		if previewed < 0 {
			t.Fatalf("%s: preview %d", th.Name, previewed)
		}
		if err := d.ClickLabel(ZhCN.RemoveItem("photo.png")); err != nil {
			t.Fatal(err)
		}
		if removed != 0 || len(files) != 2 {
			t.Fatalf("%s: remove %d, %d files", th.Name, removed, len(files))
		}
	}
	// ConfigProvider default listType "picture".
	var files []UploadFile
	var w *UploadW
	d := driver(func(c *Context) {
		ConfigProvider(c).Defaults("Upload", map[string]any{"listType": "picture", "autoUpload": false}).Body(func() {
			w = Upload(c, &files)
		})
	})
	d.Settle()
	if w.kind != upPicture || !w.manual {
		t.Fatalf("defaults: kind %d manual %v", w.kind, w.manual)
	}
}
