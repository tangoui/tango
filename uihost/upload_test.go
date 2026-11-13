package uihost_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/uihost"
)

func TestFilePicker(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	var got platform.FileOptions
	fb.OnOpenDialog = func(_ platform.SurfaceID, o platform.FileOptions) ([]string, error) {
		got = o
		return []string{"/tmp/a.png", "/tmp/b.png"}, nil
	}
	pick := uihost.FilePicker(a)
	paths, err := pick(".png,image/*", true, false)
	if err != nil || len(paths) != 2 {
		t.Fatalf("paths %v err %v", paths, err)
	}
	if !got.Multiple || got.PickFolders || len(got.Filters) != 1 || got.Filters[0].Patterns[0] != "*.png" {
		t.Fatalf("dialog options %+v", got)
	}
	// Folder pick, no filter.
	if _, err := pick("", false, true); err != nil || !got.PickFolders || len(got.Filters) != 0 {
		t.Fatalf("folder options %+v err %v", got, err)
	}
	// Cancelling is not an error.
	fb.OnOpenDialog = func(platform.SurfaceID, platform.FileOptions) ([]string, error) {
		return nil, platform.ErrCanceled
	}
	if paths, err := pick("", false, false); paths != nil || err != nil {
		t.Fatalf("cancel: %v %v", paths, err)
	}
}

func TestAcceptDropsToUpload(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	dir := t.TempDir()
	p := filepath.Join(dir, "drop.png")
	if err := os.WriteFile(p, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	var files []ui.UploadFile
	r, s, cv := open(t, a, fb, 400, 300, func(c *ui.Context) {
		ui.Upload(c, &files).Dragger()
	})
	tango.Await(a, func() struct{} { uihost.AcceptDrops(r, s); return struct{}{} })
	for cv.FlushFrame() {
	}
	if !fb.DropFiles(s.ID(), []string{p}, 200, 60) {
		t.Fatal("drop not delivered")
	}
	for cv.FlushFrame() {
	}
	n := tango.Await(a, func() int { return len(files) })
	if n != 1 {
		t.Fatalf("files after drop: %d", n)
	}
	name := tango.Await(a, func() string { return files[0].Name })
	if name != "drop.png" {
		t.Fatalf("name %q", name)
	}
}
