package ui

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/theme"
)

func p2dFile(t *testing.T, dir, name string, size int) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(strings.Repeat("x", size)), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// p2dWait runs frames until cond holds (background goroutines finish).
func p2dWait(t *testing.T, d *Driver, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
		d.Frame()
	}
	d.Settle()
}

func TestP2DUploadPickerChecks(t *testing.T) {
	p2dThemes(t, func(t *testing.T, th *theme.Theme) {
		dir := t.TempDir()
		png := p2dFile(t, dir, "a.png", 100)
		big := p2dFile(t, dir, "big.jpg", 5000)
		txt := p2dFile(t, dir, "notes.txt", 10)
		bad := p2dFile(t, dir, "virus.png", 10)
		var files []UploadFile
		var gotAccept string
		var gotMulti bool
		picker := func(accept string, multiple, dir bool) ([]string, error) {
			gotAccept, gotMulti = accept, multiple
			return []string{png, big, txt, bad}, nil
		}
		d := p2dDriver(th, func(c *Context) {
			Upload(c, &files).Accept(".png,image/*").MaxSize(1000).Multiple().Picker(picker).
				BeforeUpload(func(f UploadFile) error {
					if strings.HasPrefix(f.Name, "virus") {
						return errors.New("virus.png 未通过检查")
					}
					return nil
				})
		})
		d.EnableAccessibility()
		if err := d.ClickLabel(ZhCN.Upload.Button); err != nil {
			t.Fatal(err)
		}
		p2dWait(t, d, "picked files", func() bool { return len(files) > 0 })
		if gotAccept != ".png,image/*" || !gotMulti {
			t.Fatalf("picker args %q %v", gotAccept, gotMulti)
		}
		if len(files) != 1 || files[0].Name != "a.png" || files[0].Size != 100 || files[0].Path != png || files[0].Status != "done" {
			t.Fatalf("files %+v", files)
		}
		for _, msg := range []string{locFmt(ZhCN.Upload.TooLargeFmt, "big.jpg", "1000 B"), locFmt(ZhCN.Upload.BadTypeFmt, "notes.txt"), "virus.png 未通过检查"} {
			if _, ok := d.FindText(msg); !ok {
				t.Errorf("rejection %q not shown", msg)
			}
			if _, ok := d.A11yFind(a11y.RoleAlert, msg); !ok {
				t.Errorf("rejection %q not an alert", msg)
			}
		}
		if _, ok := d.A11yFind(a11y.RoleListItem, "a.png"); !ok {
			t.Error("list item has no name")
		}
	})
}

func TestP2DUploadDirectory(t *testing.T) {
	dir := t.TempDir()
	p2dFile(t, dir, "photos/a.png", 3)
	p2dFile(t, dir, "photos/sub/b.png", 4)
	var files []UploadFile
	var gotDir bool
	d := p2dDriver(theme.AntDesign(), func(c *Context) {
		Upload(c, &files).Directory().Picker(func(_ string, _, dir2 bool) ([]string, error) {
			gotDir = dir2
			return []string{filepath.Join(dir, "photos")}, nil
		})
	})
	if err := d.ClickLabel(ZhCN.Upload.Button); err != nil {
		t.Fatal(err)
	}
	p2dWait(t, d, "folder", func() bool { return len(files) == 2 })
	if !gotDir || files[0].Name != "photos/a.png" || files[1].Name != "photos/sub/b.png" {
		t.Fatalf("dir %v files %+v", gotDir, files)
	}
}

func TestP2DUploadDrop(t *testing.T) {
	p2dThemes(t, func(t *testing.T, th *theme.Theme) {
		dir := t.TempDir()
		a := p2dFile(t, dir, "drop.png", 7)
		txt := p2dFile(t, dir, "drop.txt", 7)
		var files []UploadFile
		var w *UploadW
		d := p2dDriver(th, func(c *Context) { w = Upload(c, &files).Dragger().Accept("image/*") })
		zr := d.r.c.rects[w.n.id]
		// Outside the dragger: ignored.
		d.DropFiles(geom.Pt(zr.X+zr.W/2, zr.Bottom()+40), a)
		d.Settle()
		if len(files) != 0 {
			t.Fatalf("drop outside added %+v", files)
		}
		d.DropFiles(zr.Center(), a, txt)
		d.Settle()
		if len(files) != 1 || files[0].Name != "drop.png" {
			t.Fatalf("dropped %+v", files)
		}
		if _, ok := d.FindText(locFmt(ZhCN.Upload.BadTypeFmt, "drop.txt")); !ok {
			t.Fatal("rejected drop not reported")
		}
	})
}

// p2dServer counts requests; the first fail requests answer 500.
type p2dServer struct {
	fail   int32
	calls  atomic.Int32
	mu     sync.Mutex
	got    []string // field name + file name + header + data
	slow   bool
	gate   chan struct{}
	gotLen int64
}

func (s *p2dServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n := s.calls.Add(1)
	if s.gate != nil {
		<-s.gate
	}
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	f, h, err := r.FormFile("upfile")
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	nb, _ := io.Copy(io.Discard, f)
	s.mu.Lock()
	s.got = append(s.got, h.Filename+"|"+r.Header.Get("X-Token")+"|"+r.FormValue("biz"))
	s.gotLen = nb
	s.mu.Unlock()
	if n <= s.fail {
		http.Error(w, "boom", 500)
		return
	}
	io.WriteString(w, `{"url":"/files/1"}`)
}

func TestP2DUploadActionProgress(t *testing.T) {
	p2dThemes(t, func(t *testing.T, th *theme.Theme) {
		dir := t.TempDir()
		const size = 32 << 20
		p := p2dFile(t, dir, "large.bin", size)
		srv := &p2dServer{gate: make(chan struct{})}
		ts := httptest.NewServer(srv)
		defer ts.Close()
		var files []UploadFile
		d := p2dDriver(th, func(c *Context) {
			Upload(c, &files).Action(ts.URL).FieldName("upfile").Headers(map[string]string{"X-Token": "t1"}).
				Data(map[string]string{"biz": "avatar"}).
				Picker(func(string, bool, bool) ([]string, error) { return []string{p}, nil })
		})
		d.EnableAccessibility()
		if err := d.ClickLabel(ZhCN.Upload.Button); err != nil {
			t.Fatal(err)
		}
		p2dWait(t, d, "uploading entry", func() bool { return len(files) == 1 })
		if files[0].Status != "uploading" {
			t.Fatalf("status %q", files[0].Status)
		}
		// The server holds the request: the client has sent part of the
		// file (socket buffers) but not all of it.
		p2dWait(t, d, "partial progress", func() bool { return files[0].Percent > 0 })
		if files[0].Percent >= 90 {
			t.Fatalf("progress %v before the server read", files[0].Percent)
		}
		item, ok := d.A11yFind(a11y.RoleListItem, "large.bin")
		if !ok || item.States&a11y.Busy == 0 || !item.HasRange {
			t.Fatalf("uploading item semantics %+v", item)
		}
		close(srv.gate)
		p2dWait(t, d, "upload done", func() bool { return files[0].Status == "done" })
		if files[0].Percent != 100 || string(files[0].Response) != `{"url":"/files/1"}` {
			t.Fatalf("done: %+v", files[0].Percent)
		}
		srv.mu.Lock()
		got, n := srv.got, srv.gotLen
		srv.mu.Unlock()
		if len(got) != 1 || got[0] != "large.bin|t1|avatar" || n != size {
			t.Fatalf("server got %v (%d bytes)", got, n)
		}
	})
}

func TestP2DUploadRetry(t *testing.T) {
	p2dThemes(t, func(t *testing.T, th *theme.Theme) {
		dir := t.TempDir()
		p := p2dFile(t, dir, "doc.pdf", 1000)
		// Automatic retries: two failures, then success.
		srv := &p2dServer{fail: 2}
		ts := httptest.NewServer(srv)
		defer ts.Close()
		var files []UploadFile
		d := p2dDriver(th, func(c *Context) {
			Upload(c, &files).Action(ts.URL).FieldName("upfile").Retries(2).
				Picker(func(string, bool, bool) ([]string, error) { return []string{p}, nil })
		})
		if err := d.ClickLabel(ZhCN.Upload.Button); err != nil {
			t.Fatal(err)
		}
		p2dWait(t, d, "retried upload", func() bool { return len(files) == 1 && files[0].Status == "done" })
		if srv.calls.Load() != 3 {
			t.Fatalf("calls %d", srv.calls.Load())
		}

		// No retries: the error shows a retry button.
		srv2 := &p2dServer{fail: 1}
		ts2 := httptest.NewServer(srv2)
		defer ts2.Close()
		var files2 []UploadFile
		d2 := p2dDriver(th, func(c *Context) {
			Upload(c, &files2).Action(ts2.URL).FieldName("upfile").
				Picker(func(string, bool, bool) ([]string, error) { return []string{p}, nil })
		})
		d2.EnableAccessibility()
		if err := d2.ClickLabel(ZhCN.Upload.Button); err != nil {
			t.Fatal(err)
		}
		p2dWait(t, d2, "failed upload", func() bool { return len(files2) == 1 && files2[0].Status == "error" })
		if files2[0].Err != "HTTP 500" {
			t.Fatalf("err %q", files2[0].Err)
		}
		item, ok := d2.A11yFind(a11y.RoleListItem, "doc.pdf")
		if !ok || !strings.Contains(item.Description, "HTTP 500") {
			t.Fatalf("error item %+v", item)
		}
		if err := d2.A11yPerform(a11y.RoleButton, ZhCN.Retry, a11y.Press, ""); err != nil {
			t.Fatal(err)
		}
		p2dWait(t, d2, "manual retry", func() bool { return files2[0].Status == "done" })
		if srv2.calls.Load() != 2 {
			t.Fatalf("calls %d", srv2.calls.Load())
		}
	})
}

func TestP2DUploadConfigDefaults(t *testing.T) {
	dir := t.TempDir()
	txt := p2dFile(t, dir, "a.txt", 1)
	var files []UploadFile
	d := p2dDriver(theme.AntDesign(), func(c *Context) {
		ConfigProvider(c).Defaults("Upload", Props{
			"accept": ".png",
			"picker": func(string, bool, bool) ([]string, error) { return []string{txt}, nil },
		}).Body(func() { Upload(c, &files) })
	})
	if err := d.ClickLabel(ZhCN.Upload.Button); err != nil {
		t.Fatal(err)
	}
	p2dWait(t, d, "rejection", func() bool {
		_, ok := d.FindText(locFmt(ZhCN.Upload.BadTypeFmt, "a.txt"))
		return ok
	})
	if len(files) != 0 {
		t.Fatalf("files %+v", files)
	}
}

func TestP2DAcceptMatch(t *testing.T) {
	for _, c := range []struct {
		accept, name string
		ok           bool
	}{
		{"", "x.exe", true},
		{".png,.jpg", "A.PNG", true},
		{".png,.jpg", "a.gif", false},
		{"image/*", "a.webp", true},
		{"image/*", "a.pdf", false},
		{"application/pdf", "a.pdf", true},
		{"*/*", "a", true},
	} {
		if got := acceptMatch(c.accept, c.name); got != c.ok {
			t.Errorf("acceptMatch(%q, %q) = %v", c.accept, c.name, got)
		}
	}
	if p := AcceptPatterns(".png, image/*"); len(p) < 3 || p[0] != "*.png" {
		t.Errorf("patterns %v", p)
	}
	if s := formatBytes(2 << 20); s != "2 MB" {
		t.Errorf("formatBytes %q", s)
	}
}
