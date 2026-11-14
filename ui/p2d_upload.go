package ui

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
)

// upRequest configures the built-in upload request (Action).
type upRequest struct {
	action  string
	method  string
	headers map[string]string
	data    map[string]string
	field   string
	retries int
	client  *http.Client
	// custom replaces the built-in request (CustomRequest).
	custom func(ctx context.Context, f UploadFile, progress func(pct float64)) ([]byte, error)
}

// active reports whether added files are uploaded (Action or
// CustomRequest).
func (r upRequest) active() bool { return r.action != "" || r.custom != nil }

// upState is the retained state of an Upload: picker results arriving
// from a goroutine, running requests and the last rejections.
type upState struct {
	mu      sync.Mutex
	picked  [][]string
	pickErr string
	picking bool
	jobs    map[int64]*upJob
	rejects []string
}

// upJob is one request; the goroutine writes it, the UI thread copies it
// into the UploadFile every build.
type upJob struct {
	mu      sync.Mutex
	sent    int64
	total   int64
	done    bool
	failed  bool
	err     string
	resp    []byte
	cancel  context.CancelFunc
	attempt int
}

var upUID atomic.Int64

// Accept limits the files Picker offers and the files accepted from the
// picker or a drop: comma-separated extensions and MIME types
// (".png,.jpg,image/*", like the HTML accept attribute). Other files are
// rejected with a message.
func (w *UploadW) Accept(s string) *UploadW { w.accept = s; return w }

// MaxSize rejects files larger than n bytes with a message (0 = no limit).
func (w *UploadW) MaxSize(n int64) *UploadW { w.maxSize = n; return w }

// BeforeUpload runs for every file about to be added (after Accept and
// MaxSize); a non-nil error rejects it and is shown as the message.
func (w *UploadW) BeforeUpload(fn func(f UploadFile) error) *UploadW { w.before = fn; return w }

// Picker sets the file chooser the trigger opens (uihost.FilePicker for
// the native dialog). It runs on its own goroutine and returns the chosen
// paths (nil, nil when cancelled); accept is Accept, multiple is Multiple,
// dir is Directory. The files are then checked and added to the list (and
// uploaded with Action). OnPick still runs, for hosts that add files
// themselves.
func (w *UploadW) Picker(fn func(accept string, multiple, dir bool) ([]string, error)) *UploadW {
	w.picker = fn
	return w
}

// Multiple lets the picker choose several files.
func (w *UploadW) Multiple() *UploadW { w.multiple = true; return w }

// Directory picks (and accepts dropped) folders and adds every file in
// them, named by their path inside the folder (Ant directory). The picker
// must support choosing folders (uihost.FilePicker does).
func (w *UploadW) Directory() *UploadW { w.dir = true; return w }

// Action uploads every added file to url (multipart/form-data POST, field
// "file"), showing its progress; Headers, Data, FieldName, Method,
// Retries and Client tune the request. A reply with status >= 400 or a
// network error marks the file "error" with a retry button.
func (w *UploadW) Action(url string) *UploadW { w.req.action = url; return w }

// CustomRequest uploads every added file with fn instead of the built-in
// HTTP request (Ant customRequest): fn runs on its own goroutine, reports
// progress (0..100) and returns the reply body or an error. The file shows
// "uploading", then "done" or "error" with a retry button, like Action;
// Retries also applies. ctx is cancelled when the file is removed.
//
// Without Action and CustomRequest the Upload only collects files: they
// are added with Status "done" (meaning accepted locally) and handed to
// the application through Added; an application that uploads them itself
// sets Status / Percent / Err on the bound list.
func (w *UploadW) CustomRequest(fn func(ctx context.Context, f UploadFile, progress func(pct float64)) ([]byte, error)) *UploadW {
	w.req.custom = fn
	return w
}

// Headers sets request headers of Action.
func (w *UploadW) Headers(h map[string]string) *UploadW { w.req.headers = h; return w }

// Data adds form fields to the Action request.
func (w *UploadW) Data(d map[string]string) *UploadW { w.req.data = d; return w }

// FieldName sets the form field of the file (default "file").
func (w *UploadW) FieldName(s string) *UploadW { w.req.field = s; return w }

// Method sets the request method (default POST).
func (w *UploadW) Method(m string) *UploadW { w.req.method = m; return w }

// Retries retries a failed request up to n more times before showing the
// error.
func (w *UploadW) Retries(n int) *UploadW { w.req.retries = n; return w }

// Client sets the HTTP client of Action (default http.DefaultClient).
func (w *UploadW) Client(cl *http.Client) *UploadW { w.req.client = cl; return w }

// AutoUpload(false) holds the files added with Action or CustomRequest
// in the list with Status "ready" instead of uploading them at once
// (Element auto-upload=false, Ant beforeUpload returning false); Submit
// uploads them. Without Action / CustomRequest files are accepted locally
// either way.
func (w *UploadW) AutoUpload(on bool) *UploadW { w.manual = !on; return w }

// Submit uploads every "ready" file of the list (see AutoUpload) with
// Action or CustomRequest, e.g. when a separate "upload" button is
// clicked. It returns how many uploads it started.
func (w *UploadW) Submit() int {
	if !w.req.active() || w.isDisabled() {
		return 0
	}
	n := 0
	for i := range *w.files {
		f := &(*w.files)[i]
		if f.Status != "ready" {
			continue
		}
		if f.UID == 0 {
			f.UID = upUID.Add(1)
		}
		f.Status, f.Percent, f.Err = "uploading", 0, ""
		w.startJob(*f)
		n++
	}
	if n > 0 {
		w.c.invalid = true
	}
	return n
}

// Added returns the files the Upload added this frame (picked or dropped,
// after the checks).
func (w *UploadW) Added() []UploadFile { w.z.run(); return w.added }

// Rejections returns the messages about files that were not added (wrong
// type, too large, BeforeUpload errors), shown under the trigger until the
// next pick or drop.
func (w *UploadW) Rejections() []string {
	w.z.run()
	w.st.mu.Lock()
	defer w.st.mu.Unlock()
	return append([]string(nil), w.st.rejects...)
}

// configDefaults applies ConfigProvider Defaults("Upload", ...): "accept",
// "action" (string), "maxSize" (int64 bytes), "maxCount" (int),
// "multiple", "directory", "disabled", "autoUpload" (bool), "listType"
// ("text" | "picture" | "picture-card" | "picture-circle"), "headers"
// (map[string]string) and
// "picker" (func(accept string, multiple, dir bool) ([]string, error)).
func (w *UploadW) configDefaults() {
	c := w.c
	w.accept = propString(c, "Upload", "accept", w.accept)
	w.req.action = propString(c, "Upload", "action", w.req.action)
	w.maxSize = propInt64(c, "Upload", "maxSize", w.maxSize)
	w.maxCount = int(propInt64(c, "Upload", "maxCount", int64(w.maxCount)))
	w.multiple = propBool(c, "Upload", "multiple", w.multiple)
	w.dir = propBool(c, "Upload", "directory", w.dir)
	w.disabled = propBool(c, "Upload", "disabled", w.disabled)
	switch propString(c, "Upload", "listType", "") {
	case "picture-card", "picture-circle":
		w.kind = upCard
	case "picture":
		w.kind = upPicture
	}
	if !propBool(c, "Upload", "autoUpload", true) {
		w.manual = true
	}
	w.req.headers = Prop(c, "Upload", "headers", w.req.headers)
	w.picker = Prop(c, "Upload", "picker", w.picker)
}

// startPick runs the Picker on a goroutine; the result is added on the
// next build.
func (w *UploadW) startPick() {
	st, fn := w.st, w.picker
	if fn == nil {
		return
	}
	st.mu.Lock()
	if st.picking {
		st.mu.Unlock()
		return
	}
	st.picking = true
	st.mu.Unlock()
	accept, multiple, dir := w.accept, w.multiple, w.dir
	wake := w.c.Invalidator()
	go func() {
		paths, err := fn(accept, multiple, dir)
		st.mu.Lock()
		st.picking = false
		if err != nil {
			st.pickErr = err.Error()
		} else if len(paths) > 0 {
			st.picked = append(st.picked, paths)
		}
		st.mu.Unlock()
		wake()
	}()
}

// sync runs at the start of every build: adds picked files and copies
// request progress into the list.
func (w *UploadW) sync() {
	st := w.st
	st.mu.Lock()
	picked, perr := st.picked, st.pickErr
	st.picked, st.pickErr = nil, ""
	st.mu.Unlock()
	if perr != "" {
		w.setRejects([]string{perr})
	}
	for _, p := range picked {
		w.addPaths(p)
	}
	if len(st.jobs) == 0 {
		return
	}
	for i := range *w.files {
		f := &(*w.files)[i]
		j := st.jobs[f.UID]
		if f.UID == 0 || j == nil {
			continue
		}
		j.mu.Lock()
		switch {
		case j.done:
			f.Status, f.Percent, f.Response, f.Err = "done", 100, j.resp, ""
			delete(st.jobs, f.UID)
			w.c.Announce(fmt.Sprintf(w.loc.Upload.DoneFmt, f.Name), a11y.LivePolite)
		case j.failed:
			f.Status, f.Err = "error", j.err
			delete(st.jobs, f.UID)
			w.c.Announce(fmt.Sprintf(w.loc.Upload.FailedFmt, f.Name), a11y.LiveAssertive)
		default:
			f.Status = "uploading"
			if j.total > 0 {
				f.Percent = min(99, float64(j.sent)*100/float64(j.total))
			}
		}
		j.mu.Unlock()
	}
}

func (w *UploadW) setRejects(msgs []string) {
	w.st.mu.Lock()
	w.st.rejects = msgs
	w.st.mu.Unlock()
}

// rejections shows the messages about files that were not added.
func (w *UploadW) rejections() {
	w.st.mu.Lock()
	msgs := append([]string(nil), w.st.rejects...)
	w.st.mu.Unlock()
	if len(msgs) == 0 {
		return
	}
	c, tk := w.c, w.tk
	Column(c, func() {
		for i, m := range msgs {
			Key(c, i, func() {
				t := Text(c, m).Color(tk.ErrorColor).Size(12).LineHeight(20)
				t.n.setRole(a11y.RoleAlert)
			})
		}
	}, layout.Margin(geom.Insets{Top: 4}))
}

// addPaths checks files from the picker or a drop and appends the
// accepted ones (uploading them with Action).
func (w *UploadW) addPaths(paths []string) {
	if w.isDisabled() {
		return
	}
	loc := w.loc
	var rejects []string
	for _, e := range w.expand(paths, &rejects) {
		if w.full() {
			rejects = append(rejects, fmt.Sprintf(loc.Upload.TooManyFmt, e.name))
			continue
		}
		f := UploadFile{Name: e.name, Size: e.size, Path: e.path, Status: "done"}
		if !acceptMatch(w.accept, e.name) {
			rejects = append(rejects, fmt.Sprintf(loc.Upload.BadTypeFmt, e.name))
			continue
		}
		if w.maxSize > 0 && e.size > w.maxSize {
			rejects = append(rejects, fmt.Sprintf(loc.Upload.TooLargeFmt, e.name, formatBytes(w.maxSize)))
			continue
		}
		if w.before != nil {
			if err := w.before(f); err != nil {
				rejects = append(rejects, err.Error())
				continue
			}
		}
		f.UID = upUID.Add(1)
		upload := w.req.active() && !w.manual
		switch {
		case upload:
			f.Status, f.Percent = "uploading", 0
		case w.req.active():
			f.Status = "ready" // waits for Submit
		}
		*w.files = append(*w.files, f)
		w.added = append(w.added, f)
		if upload {
			w.startJob(f)
		}
	}
	w.setRejects(rejects)
	w.c.invalid = true
}

type upEntry struct {
	path, name string
	size       int64
}

// expand stats the paths, walking folders when Directory is set.
func (w *UploadW) expand(paths []string, rejects *[]string) []upEntry {
	var out []upEntry
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil {
			*rejects = append(*rejects, err.Error())
			continue
		}
		if !fi.IsDir() {
			out = append(out, upEntry{path: p, name: filepath.Base(p), size: fi.Size()})
			continue
		}
		if !w.dir {
			*rejects = append(*rejects, fmt.Sprintf(w.loc.Upload.IsDirFmt, filepath.Base(p)))
			continue
		}
		root := filepath.Dir(p)
		_ = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			out = append(out, upEntry{path: path, name: filepath.ToSlash(rel), size: info.Size()})
			return nil
		})
	}
	return out
}

func (w *UploadW) canRetry(f UploadFile) bool {
	return f.Status == "error" && f.Path != "" && f.UID != 0 && w.req.active()
}

// retry uploads file i again.
func (w *UploadW) retry(i int) {
	if w.isDisabled() || i < 0 || i >= len(*w.files) {
		return
	}
	f := &(*w.files)[i]
	if !w.canRetry(*f) {
		return
	}
	f.Status, f.Percent, f.Err = "uploading", 0, ""
	w.startJob(*f)
	w.c.invalid = true
}

func (w *UploadW) cancelJob(uid int64) {
	if uid == 0 || w.st.jobs == nil {
		return
	}
	if j := w.st.jobs[uid]; j != nil {
		j.cancel()
		delete(w.st.jobs, uid)
	}
}

// startJob uploads f on a goroutine; progress is read back by sync.
func (w *UploadW) startJob(f UploadFile) {
	st := w.st
	if st.jobs == nil {
		st.jobs = map[int64]*upJob{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	j := &upJob{total: f.Size, cancel: cancel}
	st.jobs[f.UID] = j
	req := w.req
	wake := w.c.Invalidator()
	go func() {
		defer cancel()
		var err error
		var resp []byte
		for attempt := 0; attempt <= req.retries; attempt++ {
			j.mu.Lock()
			j.attempt, j.sent = attempt, 0
			j.mu.Unlock()
			if req.custom != nil {
				resp, err = upCustom(ctx, req, f, j, wake)
			} else {
				resp, err = upSend(ctx, req, f, j, wake)
			}
			if err == nil || ctx.Err() != nil {
				break
			}
		}
		j.mu.Lock()
		if err != nil {
			j.failed, j.err = true, err.Error()
		} else {
			j.done, j.resp = true, resp
		}
		j.mu.Unlock()
		wake()
	}()
}

// upCustom runs one CustomRequest attempt, mapping its percent progress
// onto the job's byte counters.
func upCustom(ctx context.Context, req upRequest, f UploadFile, j *upJob, wake func()) ([]byte, error) {
	last := -1
	return req.custom(ctx, f, func(pct float64) {
		pct = max(0, min(100, pct))
		j.mu.Lock()
		j.total, j.sent = 100, int64(pct)
		j.mu.Unlock()
		if int(pct) != last {
			last = int(pct)
			wake()
		}
	})
}

// upSend performs one multipart request, counting the file bytes the
// client has consumed.
func upSend(ctx context.Context, req upRequest, f UploadFile, j *upJob, wake func()) ([]byte, error) {
	file, err := os.Open(f.Path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	field := req.field
	if field == "" {
		field = "file"
	}
	go func() {
		var err error
		defer func() { pw.CloseWithError(err) }()
		for k, v := range req.data {
			if err = mw.WriteField(k, v); err != nil {
				return
			}
		}
		var part io.Writer
		if part, err = mw.CreateFormFile(field, filepath.Base(f.Name)); err != nil {
			return
		}
		lastPct := -1
		_, err = io.Copy(part, &upCounter{r: file, onRead: func(n int) {
			j.mu.Lock()
			j.sent += int64(n)
			pct := -1
			if j.total > 0 {
				pct = int(j.sent * 100 / j.total)
			}
			j.mu.Unlock()
			if pct != lastPct {
				lastPct = pct
				wake()
			}
		}})
		if err == nil {
			err = mw.Close()
		}
	}()
	method := req.method
	if method == "" {
		method = http.MethodPost
	}
	hr, err := http.NewRequestWithContext(ctx, method, req.action, pr)
	if err != nil {
		pr.CloseWithError(err)
		return nil, err
	}
	hr.Header.Set("Content-Type", mw.FormDataContentType())
	for k, v := range req.headers {
		hr.Header.Set(k, v)
	}
	cl := req.client
	if cl == nil {
		cl = http.DefaultClient
	}
	res, err := cl.Do(hr)
	if err != nil {
		pr.CloseWithError(err)
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		return body, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	return body, nil
}

type upCounter struct {
	r      io.Reader
	onRead func(n int)
}

func (u *upCounter) Read(p []byte) (int, error) {
	n, err := u.r.Read(p)
	if n > 0 {
		u.onRead(n)
	}
	return n, err
}

// acceptMatch reports whether name passes an accept list (".png,image/*").
func acceptMatch(accept, name string) bool {
	accept = strings.TrimSpace(accept)
	if accept == "" {
		return true
	}
	ext := strings.ToLower(filepath.Ext(name))
	typ := upMimeOf(ext)
	for _, tok := range strings.Split(accept, ",") {
		tok = strings.ToLower(strings.TrimSpace(tok))
		switch {
		case tok == "":
		case tok == "*" || tok == "*/*":
			return true
		case strings.HasPrefix(tok, "."):
			if ext == tok {
				return true
			}
		case strings.HasSuffix(tok, "/*"):
			if typ != "" && strings.HasPrefix(typ, tok[:len(tok)-1]) {
				return true
			}
		default:
			if typ == tok {
				return true
			}
		}
	}
	return false
}

// upMimeOf returns the MIME type of an extension ("" when unknown),
// without parameters. A few common types are built in so the result does
// not depend on the system's MIME tables.
func upMimeOf(ext string) string {
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".bmp":
		return "image/bmp"
	case ".heic":
		return "image/heic"
	case ".mp4":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".pdf":
		return "application/pdf"
	case ".txt":
		return "text/plain"
	}
	t := mime.TypeByExtension(ext)
	if i := strings.IndexByte(t, ';'); i >= 0 {
		t = t[:i]
	}
	return strings.ToLower(strings.TrimSpace(t))
}

// AcceptPatterns turns an accept list into file-dialog patterns ("*.png");
// MIME wildcards expand to common extensions. Hosts use it to build the
// native dialog's filter (uihost.FilePicker).
func AcceptPatterns(accept string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(exts ...string) {
		for _, e := range exts {
			p := "*" + e
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	for _, tok := range strings.Split(accept, ",") {
		tok = strings.ToLower(strings.TrimSpace(tok))
		switch {
		case tok == "":
		case strings.HasPrefix(tok, "."):
			add(tok)
		case tok == "image/*":
			add(".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg", ".heic")
		case tok == "video/*":
			add(".mp4", ".mov", ".webm", ".mkv", ".avi")
		case tok == "audio/*":
			add(".mp3", ".wav", ".m4a", ".flac", ".ogg")
		default:
			exts, _ := mime.ExtensionsByType(tok)
			add(exts...)
		}
	}
	return out
}

// formatBytes formats a size: 512 B, 2 KB, 1.5 MB, 2 GB.
func formatBytes(n int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	f := float64(n)
	i := 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	if i == 0 || f >= 100 || f == float64(int64(f)) {
		return strconv.FormatInt(int64(f+0.5), 10) + " " + units[i]
	}
	return strconv.FormatFloat(f, 'f', 1, 64) + " " + units[i]
}
