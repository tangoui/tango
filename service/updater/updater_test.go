package updater

import (
	"bytes"
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---- a release server ----

type server struct {
	t        *testing.T
	srv      *httptest.Server
	pub      string
	priv     string
	mu       sync.Mutex
	files    map[string][]byte
	envelope []byte
	hits     map[string]int
}

func newServer(t *testing.T) *server {
	t.Helper()
	pub, priv, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	s := &server{t: t, pub: pub, priv: priv, files: map[string][]byte{}, hits: map[string]int{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/app/latest.json", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		w.Write(s.envelope)
	})
	mux.HandleFunc("/app/files/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/app/files/")
		s.mu.Lock()
		b, ok := s.files[name]
		s.hits[name]++
		s.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(b)
	})
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	return s
}

// put serves b and returns its manifest entry (with a relative URL).
func (s *server) put(name string, b []byte) File {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[name] = b
	p := filepath.Join(s.t.TempDir(), name)
	os.WriteFile(p, b, 0o644)
	f, err := FileInfo(p, "files/"+name)
	if err != nil {
		s.t.Fatal(err)
	}
	return f
}

func (s *server) publish(m *Manifest) {
	s.t.Helper()
	env, err := SignManifest(s.priv, m)
	if err != nil {
		s.t.Fatal(err)
	}
	s.mu.Lock()
	s.envelope = env
	s.mu.Unlock()
}

func (s *server) hitCount(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits[name]
}

func (s *server) updater(version string) *Updater {
	return &Updater{
		ManifestURL: s.srv.URL + "/app/latest.json",
		PublicKey:   s.pub,
		Version:     version,
		Platform:    "testos-testarch",
		Client:      s.srv.Client(),
		StateDir:    s.t.TempDir(),
	}
}

var pubDate = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

func manifest(version, format string, full File) *Manifest {
	return &Manifest{
		App: "com.example.notes", Version: version, PubDate: pubDate, Notes: "notes",
		Platforms: map[string]Asset{"testos-testarch": {Format: format, File: full}},
	}
}

func randomBytes(r *rand.Rand, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(r.Uint32())
	}
	return b
}

// ---- manifest signing and anti-rollback ----

func TestManifestSignature(t *testing.T) {
	pub, priv, _ := GenerateKey()
	m := manifest("1.2.0", FormatExe, File{URL: "a.exe", Size: 3, SHA256: strings.Repeat("ab", 32)})
	env, err := SignManifest(priv, m)
	if err != nil {
		t.Fatal(err)
	}
	got, err := VerifyManifest(env, pub)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "1.2.0" || got.Platforms["testos-testarch"].URL != "a.exe" {
		t.Fatalf("%+v", got)
	}

	// Any change to the signed bytes — a URL, a digest, the version —
	// breaks the signature, even if the JSON stays valid.
	for _, edit := range [][2]string{{`"1.2.0"`, `"9.9.9"`}, {`a.exe`, `b.exe`}, {`"abab`, `"cdab`}} {
		bad := bytes.Replace(env, []byte(edit[0]), []byte(edit[1]), 1)
		if bytes.Equal(bad, env) {
			t.Fatalf("edit %q did not apply", edit)
		}
		if _, err := VerifyManifest(bad, pub); !errors.Is(err, ErrBadSignature) {
			t.Errorf("edit %q: %v", edit, err)
		}
	}
	other, otherPriv, _ := GenerateKey()
	if _, err := VerifyManifest(env, other); !errors.Is(err, ErrBadSignature) {
		t.Errorf("foreign key: %v", err)
	}
	// Key rotation: either key verifies.
	env2, _ := SignManifest(otherPriv, m)
	if _, err := VerifyManifest(env2, pub, other); err != nil {
		t.Errorf("rotated key: %v", err)
	}
	// An unsigned manifest is refused.
	plain := []byte(`{"version":"1.2.0","pub_date":"2026-11-01T12:00:00Z","platforms":{}}`)
	if _, err := VerifyManifest(plain, pub); err == nil {
		t.Error("unsigned manifest accepted")
	}
	// Invalid manifests are refused at signing.
	for _, bad := range []*Manifest{
		{Version: "x", PubDate: pubDate, Platforms: m.Platforms},
		{Version: "1.0.0", Platforms: m.Platforms},
		{Version: "1.0.0", PubDate: pubDate, Platforms: map[string]Asset{"p": {Format: "zip", File: m.Platforms["testos-testarch"].File}}},
		{Version: "1.0.0", PubDate: pubDate, Platforms: map[string]Asset{"p": {Format: FormatApp, File: m.Platforms["testos-testarch"].File}}}, // no tree_sha256
		{Version: "1.0.0", PubDate: pubDate, Platforms: map[string]Asset{"p": {Format: FormatNSIS, File: m.Platforms["testos-testarch"].File,
			Patches: []PatchAsset{{From: "0.9.0", File: m.Platforms["testos-testarch"].File}}}}},
	} {
		if _, err := SignManifest(priv, bad); err == nil {
			t.Errorf("signed invalid manifest %+v", bad)
		}
	}
	if k, _ := PublicKeyOf(priv); k != pub {
		t.Error("PublicKeyOf")
	}
}

func TestCheckRefusesDowngrades(t *testing.T) {
	ctx := context.Background()
	s := newServer(t)
	full := s.put("App-1.2.0.exe", []byte("v1.2.0"))
	s.publish(manifest("1.2.0", FormatExe, full))

	u := s.updater("1.0.0")
	rel, err := u.Check(ctx)
	if err != nil || rel == nil || rel.Version != "1.2.0" || rel.Full.URL != s.srv.URL+"/app/files/App-1.2.0.exe" {
		t.Fatalf("Check = %+v, %v", rel, err)
	}
	// Same version: up to date.
	if rel, err := s.updater("1.2.0").Check(ctx); rel != nil || err != nil {
		t.Fatalf("same version: %v, %v", rel, err)
	}
	// Older than the running version: refused.
	if _, err := s.updater("1.3.0").Check(ctx); !errors.Is(err, ErrDowngrade) {
		t.Fatalf("older manifest: %v", err)
	}
	// Older than MinVersion: refused.
	u2 := s.updater("1.0.0")
	u2.MinVersion = "1.5.0"
	if _, err := u2.Check(ctx); !errors.Is(err, ErrDowngrade) {
		t.Fatalf("MinVersion: %v", err)
	}
	// Replay: once 1.2.0 was seen, an old signed 1.1.0 manifest (still
	// newer than the running 1.0.0) is refused.
	s.publish(manifest("1.1.0", FormatExe, full))
	if _, err := u.Check(ctx); !errors.Is(err, ErrDowngrade) {
		t.Fatalf("replayed manifest: %v", err)
	}
	// A manifest for another app is refused.
	s.publish(manifest("1.3.0", FormatExe, full))
	u.AppID = "com.example.other"
	if _, err := u.Check(ctx); err == nil || !strings.Contains(err.Error(), "com.example.other") {
		t.Fatalf("app id: %v", err)
	}
	u.AppID = ""
	// A forged manifest (signed by another key) is refused.
	_, evil, _ := GenerateKey()
	env, _ := SignManifest(evil, manifest("9.0.0", FormatExe, full))
	s.mu.Lock()
	s.envelope = env
	s.mu.Unlock()
	if _, err := u.Check(ctx); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("forged manifest: %v", err)
	}
	// Missing platform, bad keys and versions.
	s.publish(manifest("2.0.0", FormatExe, full))
	u.Platform = "plan9-mips"
	if _, err := u.Check(ctx); err == nil || !strings.Contains(err.Error(), "plan9-mips") {
		t.Fatalf("missing platform: %v", err)
	}
	u.Platform = "testos-testarch"
	for _, mut := range []func(*Updater){
		func(u *Updater) { u.Version = "garbage" },
		func(u *Updater) { u.PublicKey = "nope" },
		func(u *Updater) { u.ManifestURL = s.srv.URL + "/missing.json" },
	} {
		c := *u
		mut(&c)
		if _, err := c.Check(ctx); err == nil {
			t.Errorf("accepted %+v", c)
		}
	}
	m := manifest("2.0.0", FormatExe, full)
	a := m.Platforms["testos-testarch"]
	a.URL = "file:///etc/passwd"
	m.Platforms["testos-testarch"] = a
	s.publish(m)
	if _, err := u.Check(ctx); err == nil {
		t.Fatal("file: URL accepted")
	}
}

// ---- file patches ----

// fileRelease publishes newBody as 2.0.0 with a patch from 1.0.0 and
// returns an Updater at 1.0.0 whose target holds oldBody.
func fileRelease(t *testing.T, oldBody, newBody []byte) (*server, *Updater, []byte) {
	s := newServer(t)
	patch, err := Diff(oldBody, newBody)
	if err != nil {
		t.Fatal(err)
	}
	m := manifest("2.0.0", FormatExe, s.put("App-2.0.0.exe", newBody))
	a := m.Platforms["testos-testarch"]
	a.Patches = []PatchAsset{{From: "1.0.0", File: s.put("App-1.0.0-to-2.0.0.patch", patch)}}
	m.Platforms["testos-testarch"] = a
	s.publish(m)
	u := s.updater("1.0.0")
	u.Target = filepath.Join(t.TempDir(), "App.exe")
	os.WriteFile(u.Target, oldBody, 0o755)
	return s, u, patch
}

func downloadAndCheck(t *testing.T, u *Updater, want []byte) (*Payload, []Event) {
	t.Helper()
	var mu sync.Mutex
	var events []Event
	u.OnEvent = func(e Event) { mu.Lock(); events = append(events, e); mu.Unlock() }
	ctx := context.Background()
	rel, err := u.Check(ctx)
	if err != nil || rel == nil {
		t.Fatalf("Check = %v, %v", rel, err)
	}
	p, err := u.Download(ctx, rel, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(p.Path); !bytes.Equal(got, want) {
		t.Fatal("downloaded release differs")
	}
	return p, events
}

func kinds(events []Event) []EventKind {
	var k []EventKind
	for _, e := range events {
		if len(k) == 0 || k[len(k)-1] != e.Kind {
			k = append(k, e.Kind)
		}
	}
	return k
}

func TestPatchRandomBinary(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 8))
	oldBody := randomBytes(r, 1<<20)
	newBody := slices.Clone(oldBody)
	for range 200 {
		newBody[r.IntN(len(newBody))] = byte(r.Uint32())
	}
	newBody = slices.Insert(newBody, 300000, randomBytes(r, 20000)...)
	newBody = slices.Delete(newBody, 700000, 710000)

	s, u, patch := fileRelease(t, oldBody, newBody)
	p, events := downloadAndCheck(t, u, newBody)
	if !p.Patched {
		t.Fatal("full file downloaded instead of the patch")
	}
	if s.hitCount("App-2.0.0.exe") != 0 {
		t.Fatal("full file fetched")
	}
	want := []EventKind{EventChecking, EventAvailable, EventDownloading, EventProgress, EventVerified}
	if got := kinds(events); !slices.Equal(got, want) {
		t.Fatalf("events %v, want %v", got, want)
	}
	ratio := float64(len(patch)) / float64(len(newBody))
	t.Logf("random 1 MiB binary, 200 edits + 20 KB insert + 10 KB delete: full %d bytes, patch %d bytes (%.2f%%)", len(newBody), len(patch), 100*ratio)
	if ratio > 0.1 {
		t.Errorf("patch is %.1f%% of the full file", 100*ratio)
	}
}

func TestPatchFallsBackToFull(t *testing.T) {
	r := rand.New(rand.NewPCG(9, 10))
	oldBody := randomBytes(r, 100000)
	newBody := append(slices.Clone(oldBody), "v2"...)

	// The installed file is not the one the patch was made from.
	s, u, _ := fileRelease(t, oldBody, newBody)
	os.WriteFile(u.Target, append([]byte("locally modified"), oldBody...), 0o755)
	p, events := downloadAndCheck(t, u, newBody)
	if p.Patched || s.hitCount("App-2.0.0.exe") != 1 {
		t.Fatalf("patched %v, full fetched %d times", p.Patched, s.hitCount("App-2.0.0.exe"))
	}
	if !slices.Contains(kinds(events), EventPatchFailed) {
		t.Fatalf("no patch-failed event: %v", kinds(events))
	}

	// The patch on the server is corrupt (its digest no longer matches).
	s, u, patch := fileRelease(t, oldBody, newBody)
	bad := slices.Clone(patch)
	bad[len(bad)/2] ^= 1
	s.mu.Lock()
	s.files["App-1.0.0-to-2.0.0.patch"] = bad
	s.mu.Unlock()
	if p, _ := downloadAndCheck(t, u, newBody); p.Patched {
		t.Fatal("corrupt patch used")
	}

	// The full file is tampered with too: Download fails, leaving nothing.
	s.mu.Lock()
	s.files["App-2.0.0.exe"] = []byte("evil")
	s.mu.Unlock()
	rel, _ := u.Check(context.Background())
	dir := t.TempDir()
	if _, err := u.Download(context.Background(), rel, dir); err == nil {
		t.Fatal("tampered full file accepted")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("files left: %v", entries)
	}
}

// buildGo builds a small Go program; v2 has more code, as a real release
// would, so addresses throughout the binary shift.
func buildGo(t *testing.T, v2 bool) []byte {
	t.Helper()
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go command")
	}
	dir := t.TempDir()
	src := `package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
)

var version = "1.0.0"

func main() {
	fmt.Println("hello", version, strings.ToUpper(os.Args[0]))
	if len(os.Args) > 5 {
		http.ListenAndServe(":0", nil)
	}
	extra()
}
`
	extra := "package main\n\nfunc extra() {}\n"
	if v2 {
		src = strings.Replace(src, `"1.0.0"`, `"2.0.0"`, 1)
		extra = `package main

import (
	"encoding/json"
	"fmt"
	"sort"
)

func extra() {
	m := map[string]int{"a": 1, "b": 2}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	b, _ := json.Marshal(keys)
	fmt.Println(string(b))
}
`
	}
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module sample\n\ngo 1.22\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644)
	os.WriteFile(filepath.Join(dir, "extra.go"), []byte(extra), 0o644)
	out := filepath.Join(dir, "sample")
	cmd := exec.Command(gobin, "build", "-trimpath", "-ldflags=-s -w", "-o", out, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, b)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPatchGoExecutable(t *testing.T) {
	if testing.Short() {
		t.Skip("builds Go programs")
	}
	oldBody, newBody := buildGo(t, false), buildGo(t, true)
	_, u, patch := fileRelease(t, oldBody, newBody)
	p, _ := downloadAndCheck(t, u, newBody)
	if !p.Patched {
		t.Fatal("patch not used")
	}
	ratio := float64(len(patch)) / float64(len(newBody))
	t.Logf("Go executable (%s): old %d, new %d bytes, patch %d bytes (%.2f%% of the full file)", runtime.GOOS+"/"+runtime.GOARCH, len(oldBody), len(newBody), len(patch), 100*ratio)
	if ratio > 0.25 {
		t.Errorf("patch is %.1f%% of the full file", 100*ratio)
	}
}

// ---- tree patches ----

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if target, ok := strings.CutPrefix(body, "->"); ok {
			if err := os.Symlink(target, p); err != nil {
				t.Fatal(err)
			}
			continue
		}
		mode := os.FileMode(0o644)
		if strings.Contains(name, "MacOS/") {
			mode = 0o755
		}
		if err := os.WriteFile(p, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
}

func bundleFiles(exe []byte, version string) map[string]string {
	return map[string]string{
		"Contents/Info.plist":                              "<plist>" + version + "</plist>",
		"Contents/MacOS/Notes":                             string(exe),
		"Contents/Resources/icon.icns":                     strings.Repeat("icon", 2000),
		"Contents/Frameworks/F.framework/Versions/A/F":     "lib",
		"Contents/Frameworks/F.framework/Versions/Current": "->A",
		"Contents/Frameworks/F.framework/F":                "->Versions/Current/F",
	}
}

func TestTreePatch(t *testing.T) {
	r := rand.New(rand.NewPCG(11, 12))
	exe := randomBytes(r, 300000)
	exe2 := slices.Clone(exe)
	for range 50 {
		exe2[r.IntN(len(exe2))]++
	}
	oldApp := filepath.Join(t.TempDir(), "Notes.app")
	newApp := filepath.Join(t.TempDir(), "Notes.app")
	writeTree(t, oldApp, bundleFiles(exe, "1.0.0"))
	nf := bundleFiles(exe2, "2.0.0")
	nf["Contents/Resources/new.txt"] = "added"
	delete(nf, "Contents/Resources/icon.icns")
	nf["Contents/Resources/moved.icns"] = strings.Repeat("icon", 2000) // renamed: copied
	writeTree(t, newApp, nf)

	var patch bytes.Buffer
	if err := DiffTree(oldApp, newApp, &patch); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "Notes-2.0.0.app.tar.gz")
	if err := PackTree(newApp, archive); err != nil {
		t.Fatal(err)
	}
	full, _ := os.ReadFile(archive)
	digest, _ := TreeDigest(newApp)

	s := newServer(t)
	m := manifest("2.0.0", FormatApp, s.put("Notes-2.0.0.app.tar.gz", full))
	a := m.Platforms["testos-testarch"]
	a.TreeSHA256 = digest
	a.Patches = []PatchAsset{{From: "1.0.0", File: s.put("Notes-1.0.0-to-2.0.0.patch", patch.Bytes())}}
	m.Platforms["testos-testarch"] = a
	s.publish(m)
	t.Logf("tree: full archive %d bytes, tree patch %d bytes (%.2f%%)", len(full), patch.Len(), 100*float64(patch.Len())/float64(len(full)))

	u := s.updater("1.0.0")
	u.Target = oldApp
	ctx := context.Background()
	rel, err := u.Check(ctx)
	if err != nil {
		t.Fatal(err)
	}
	p, err := u.Download(ctx, rel, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !p.Patched {
		t.Fatal("tree patch not used")
	}
	if got, _ := TreeDigest(p.Path); got != digest {
		t.Fatal("patched tree differs")
	}
	// Windows stores (and reads back) the link target with backslashes.
	if l, err := os.Readlink(filepath.Join(p.Path, "Contents/Frameworks/F.framework/F")); err != nil || filepath.ToSlash(l) != "Versions/Current/F" {
		t.Fatalf("symlink: %q %v", l, err)
	}
	if fi, _ := os.Stat(filepath.Join(p.Path, "Contents/MacOS/Notes")); runtime.GOOS != "windows" && fi.Mode().Perm()&0o100 == 0 {
		t.Fatal("executable bit lost")
	}

	// Staging the patched tree and the full archive both yield the bundle.
	for _, pl := range []*Payload{p, {Release: rel, Path: archive}} {
		st, err := stage(pl, oldApp)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := TreeDigest(st.repl); got != digest {
			t.Fatal("staged tree differs")
		}
		if filepath.Dir(st.work) != filepath.Dir(oldApp) {
			t.Fatal("work dir not next to the target")
		}
		os.RemoveAll(st.work)
	}

	// A locally modified installation fails the patch; Download falls
	// back to the archive.
	os.WriteFile(filepath.Join(oldApp, "Contents/Resources/icon.icns"), []byte("changed"), 0o644)
	p, err = u.Download(ctx, rel, t.TempDir())
	if err != nil || p.Patched {
		t.Fatalf("fallback: %+v, %v", p, err)
	}
}

func TestTreePatchRejectsEscapes(t *testing.T) {
	old := t.TempDir()
	os.WriteFile(filepath.Join(old, "a"), []byte("x"), 0o644)
	newDir := t.TempDir()
	os.WriteFile(filepath.Join(newDir, "a"), []byte("x"), 0o644)
	var buf bytes.Buffer
	if err := DiffTree(old, newDir, &buf); err != nil {
		t.Fatal(err)
	}
	// Hand-made index entries that escape.
	for _, e := range []treeEntry{
		{Path: "../evil", Type: "file", Op: "data", Blob: "b"},
		{Path: "l", Type: "symlink", Link: "../../etc"},
		{Path: "c", Type: "file", Op: "copy", From: "../outside", SHA256: strings.Repeat("0", 64)},
	} {
		outDir := filepath.Join(t.TempDir(), "out")
		os.Mkdir(outDir, 0o755)
		oldRoot, _ := os.OpenRoot(old)
		root, _ := os.OpenRoot(outDir)
		if err := patchEntry(oldRoot, root, e, map[string][]byte{"b": []byte("x")}); err == nil {
			t.Errorf("accepted %+v", e)
		}
		oldRoot.Close()
		root.Close()
	}
}

// ---- helper, confirmation and rollback ----

// exitedPid returns the pid of a process that has exited.
func exitedPid(t *testing.T) int {
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return cmd.Process.Pid
}

type fakeOps struct {
	mu      sync.Mutex
	started [][]string
	ran     [][]string
	onStart func(cmd []string)
	runErr  error
}

func (f *fakeOps) ops() helperOps {
	return helperOps{
		wait: func(int) {},
		start: func(c []string) error {
			f.mu.Lock()
			f.started = append(f.started, c)
			f.mu.Unlock()
			if f.onStart != nil {
				f.onStart(c)
			}
			return nil
		},
		run: func(c []string) error {
			f.ran = append(f.ran, c)
			return f.runErr
		},
		alive: func(int) bool { return false },
		sleep: func(time.Duration) { time.Sleep(time.Millisecond) },
	}
}

// exeJob lays out a Windows-style exe replacement.
func exeJob(t *testing.T) *helperJob {
	dir := t.TempDir()
	target := filepath.Join(dir, "App.exe")
	os.WriteFile(target, []byte("old"), 0o755)
	work := filepath.Join(dir, ".tango-update-1")
	os.MkdirAll(filepath.Join(work, "new"), 0o755)
	repl := filepath.Join(work, "new", "App.exe")
	os.WriteFile(repl, []byte("new"), 0o755)
	return &helperJob{Format: FormatExe, Target: target, Repl: repl, Work: work, State: t.TempDir(), Timeout: 1, Launch: []string{target}}
}

func TestRunJobConfirmed(t *testing.T) {
	j := exeJob(t)
	os.WriteFile(filepath.Join(j.State, statePending), []byte("{}"), 0o600)
	f := &fakeOps{onStart: func([]string) {
		// The new version starts and confirms.
		if b, _ := os.ReadFile(j.Target); string(b) == "new" {
			os.WriteFile(filepath.Join(j.State, stateConfirmed), nil, 0o600)
		}
	}}
	if err := runJob(j, f.ops()); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(j.Target); string(b) != "new" {
		t.Fatalf("target = %q", b)
	}
	for _, p := range []string{j.Work, filepath.Join(j.State, statePending), filepath.Join(j.State, stateConfirmed)} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s left", p)
		}
	}
}

func TestRunJobRollsBack(t *testing.T) {
	// No confirmation within the timeout.
	j := exeJob(t)
	f := &fakeOps{}
	if err := runJob(j, f.ops()); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(j.Target); string(b) != "old" {
		t.Fatalf("not rolled back: %q", b)
	}
	if len(f.started) != 2 {
		t.Fatalf("started %v, want new then old", f.started)
	}
	if b, _ := os.ReadFile(filepath.Join(j.State, stateRolledBack)); !strings.Contains(string(b), "did not confirm") {
		t.Fatalf("reason %q", b)
	}

	// The new version asks for a rollback.
	j = exeJob(t)
	j.Timeout = 30
	f = &fakeOps{onStart: func([]string) {
		os.WriteFile(filepath.Join(j.State, stateRollback), []byte("12345"), 0o600)
	}}
	runJob(j, f.ops())
	if b, _ := os.ReadFile(filepath.Join(j.State, stateRolledBack)); !strings.Contains(string(b), "asked for a rollback") {
		t.Fatalf("reason %q", b)
	}

	// The replacement is missing: nothing changes, the old app restarts.
	j = exeJob(t)
	os.Remove(j.Repl)
	f = &fakeOps{}
	runJob(j, f.ops())
	if b, _ := os.ReadFile(j.Target); string(b) != "old" || len(f.started) != 1 {
		t.Fatalf("target %q, started %v", b, f.started)
	}
}

func TestRunJobInstaller(t *testing.T) {
	j := exeJob(t)
	j.Format = FormatNSIS
	j.Repl = filepath.Join(j.Work, "new", "App-2.0.0-setup.exe")
	os.WriteFile(j.Repl, []byte("installer"), 0o755)
	f := &fakeOps{}
	f.onStart = func([]string) { os.WriteFile(filepath.Join(j.State, stateConfirmed), nil, 0o600) }
	ops := f.ops()
	ops.run = func(c []string) error {
		f.ran = append(f.ran, c)
		return os.WriteFile(j.Target, []byte("installed"), 0o755) // the installer writes the new exe
	}
	if err := runJob(j, ops); err != nil {
		t.Fatal(err)
	}
	want := []string{j.Repl, "/S", "/D=" + filepath.Dir(j.Target)}
	if len(f.ran) != 1 || !slices.Equal(f.ran[0], want) {
		t.Fatalf("ran %v, want %v", f.ran, want)
	}
	if b, _ := os.ReadFile(j.Target); string(b) != "installed" {
		t.Fatalf("target %q", b)
	}

	// A failing installer leaves the old executable in place.
	j = exeJob(t)
	j.Format = FormatNSIS
	f = &fakeOps{runErr: errors.New("exit status 2")}
	runJob(j, f.ops())
	if b, _ := os.ReadFile(j.Target); string(b) != "old" {
		t.Fatalf("target %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(j.State, stateRolledBack)); !strings.Contains(string(b), "install failed") {
		t.Fatalf("reason %q", b)
	}
}

func TestInstallerCommand(t *testing.T) {
	if name, args, err := installerCommand(`C:\t\Setup.EXE`, `C:\Apps\Notes`, nil); err != nil || name != `C:\t\Setup.EXE` || strings.Join(args, " ") != `/S /D=C:\Apps\Notes` {
		t.Errorf("exe: %s %v %v", name, args, err)
	}
	if name, args, err := installerCommand(`C:\t\n.msi`, `C:\x`, []string{"ALLUSERS=2"}); err != nil || name != "msiexec" || strings.Join(args, " ") != `/i C:\t\n.msi /qn /norestart ALLUSERS=2` {
		t.Errorf("msi: %s %v %v", name, args, err)
	}
	if _, _, err := installerCommand("x.zip", "", nil); err == nil {
		t.Error("zip accepted as installer")
	}
}

func TestConfirm(t *testing.T) {
	u := &Updater{Version: "2.0.0", StateDir: t.TempDir()}
	if o, err := u.Confirm(); err != nil || o.Status != StatusNone {
		t.Fatalf("no update: %+v %v", o, err)
	}
	work := t.TempDir()
	writeJSON(filepath.Join(u.StateDir, statePending), pending{From: "1.0.0", To: "2.0.0", Work: work, Time: time.Now(), Timeout: 60})
	if err := (&Updater{Version: "2.0.0", StateDir: u.StateDir}).Rollback(); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(u.StateDir, stateRollback))
	o, err := u.Confirm()
	if err != nil || o.Status != StatusUpdated || o.From != "1.0.0" {
		t.Fatalf("new version: %+v %v", o, err)
	}
	if _, err := os.Stat(filepath.Join(u.StateDir, stateConfirmed)); err != nil {
		t.Fatal("not confirmed")
	}
	if err := u.Rollback(); err == nil {
		t.Fatal("rollback after confirmation")
	}

	// The old version, before the helper swapped: pending.
	old := &Updater{Version: "1.0.0", StateDir: t.TempDir()}
	writeJSON(filepath.Join(old.StateDir, statePending), pending{From: "1.0.0", To: "2.0.0", Time: time.Now(), Timeout: 60})
	if o, _ := old.Confirm(); o.Status != StatusPending {
		t.Fatalf("old version: %+v", o)
	}
	// After the helper rolled back.
	os.WriteFile(filepath.Join(old.StateDir, stateRolledBack), []byte("the new version did not confirm within 60s\n"), 0o600)
	if o, _ := old.Confirm(); o.Status != StatusRolledBack || !strings.Contains(o.Reason, "did not confirm") {
		t.Fatalf("rolled back: %+v", o)
	}
	if o, _ := old.Confirm(); o.Status != StatusNone {
		t.Fatalf("reported twice: %+v", o)
	}
}

func TestTargetFor(t *testing.T) {
	j := filepath.Join
	cases := []struct {
		goos, format, exe, appimage, want string
	}{
		{"darwin", FormatApp, j("/Applications", "Notes.app", "Contents", "MacOS", "Notes"), "", j("/Applications", "Notes.app")},
		{"linux", FormatAppImage, "/tmp/.mount_x/usr/bin/notes", "/home/a/Notes.AppImage", "/home/a/Notes.AppImage"},
		{"linux", FormatDir, j("/opt", "notes", "notes"), "", j("/opt", "notes")},
		{"windows", FormatExe, `C:\Notes\Notes.exe`, "", `C:\Notes\Notes.exe`},
		{"windows", FormatNSIS, `C:\Notes\Notes.exe`, "", `C:\Notes\Notes.exe`},
	}
	for _, c := range cases {
		if got, err := targetFor(c.goos, c.format, c.exe, c.appimage); err != nil || got != c.want {
			t.Errorf("targetFor(%s, %s) = %q, %v", c.goos, c.format, got, err)
		}
	}
	for _, c := range [][3]string{
		{"darwin", FormatApp, "/usr/local/bin/notes"},
		{"linux", FormatAppImage, "/usr/bin/notes"},
		{"darwin", FormatExe, "/x"},
		{"ios", FormatApp, "/x.app/Contents/MacOS/x"},
	} {
		if _, err := targetFor(c[0], c[1], c[2], ""); !errors.Is(err, ErrUnsupported) {
			t.Errorf("targetFor%v: %v", c, err)
		}
	}
}

func TestStageNotWritable(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("needs Unix permissions as a normal user")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "Notes.AppImage")
	os.WriteFile(target, []byte("old"), 0o755)
	src := filepath.Join(t.TempDir(), "new.AppImage")
	os.WriteFile(src, []byte("new"), 0o755)
	os.Chmod(dir, 0o555)
	defer os.Chmod(dir, 0o755)
	_, err := stage(&Payload{Release: &Release{Format: FormatAppImage}, Path: src}, target)
	if !errors.Is(err, ErrNotWritable) || !strings.Contains(err.Error(), dir) {
		t.Fatalf("stage = %v", err)
	}
	if err := checkLocation("/private/var/folders/x/AppTranslocation/ABC/d/Notes.app"); !errors.Is(err, ErrNotWritable) {
		t.Fatalf("translocated: %v", err)
	}
}
