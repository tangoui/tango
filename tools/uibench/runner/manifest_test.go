package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeTree is a uibench directory with the files the manifest writer
// reads: mygo/go.mod and go.sum, third_party/nsgo with its version file.
func fakeTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, s string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range []string{"tango", "runner", "proto"} {
		write(d+"/go.mod", "module x\n")
	}
	write("mygo/go.mod", "module tango.local/uibench/mygo\n\ngo 1.27.3\n\nrequire (\n\tgithub.com/egoist/mygo v0.9.0\n)\n")
	write("mygo/go.sum", "github.com/egoist/mygo v0.9.0 h1:zGeZ=\ngithub.com/egoist/mygo v0.9.0/go.mod h1:Hw9s=\n")
	write("nsgo-src/go.mod", "module github.com/nsgo-dev/nsgo\n")
	write("nsgo-src/a.go", "package nsgo\n")
	write("nsgo-src/sub/b.go", "package sub\n")
	if err := os.MkdirAll(filepath.Join(root, "third_party"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "nsgo-src"), filepath.Join(root, "third_party", "nsgo")); err != nil {
		t.Fatal(err)
	}
	hash, err := goTreeSHA1(filepath.Join(root, "nsgo-src"))
	if err != nil {
		t.Fatal(err)
	}
	write("third_party/nsgo.version", "source "+filepath.Join(root, "nsgo-src")+"\nchangelog Unreleased\ngo-tree-sha1 "+hash+"\n")
	return root
}

// fakeBuild stands in for the go and git commands: go build writes a
// binary whose content names its directory, go version -m prints the
// toolchain version[name], git reports commit abc123 and the status dirty.
type fakeBuild struct {
	version map[string]string // binary name -> go version it was "built" with
	fail    map[string]bool   // binary name -> go build fails
	dirty   string
	deps    map[string]string // module -> "version sum" the binaries link (default mygo v0.9.0 h1:zGeZ=)
	calls   []string
	envs    [][]string
}

func (f *fakeBuild) run(dir string, env []string, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, dir+": "+name+" "+strings.Join(args, " "))
	f.envs = append(f.envs, env)
	switch {
	case name == "git" && len(args) > 0 && args[0] == "rev-parse":
		return []byte("abc123\n"), nil
	case name == "git" && len(args) > 0 && args[0] == "status":
		return []byte(f.dirty), nil
	case name == "go" && args[0] == "build":
		out := args[indexOf(args, "-o")+1]
		if f.fail[filepath.Base(out)] {
			return []byte("build failed: needs cgo"), errors.New("exit status 1")
		}
		return nil, os.WriteFile(out, []byte("binary of "+filepath.Base(dir)), 0o755)
	case name == "go" && args[0] == "version":
		p := args[len(args)-1]
		dep := ""
		if filepath.Base(p) == "mygo" {
			v, s, _ := strings.Cut(f.dep("github.com/egoist/mygo"), " ")
			dep = "\tdep\tgithub.com/egoist/mygo\t" + v + "\t" + s + "\n"
		}
		return []byte(fmt.Sprintf("%s: %s\n\tpath\tx\n%s\tbuild\tCGO_ENABLED=0\n", p, f.versionOf(filepath.Base(p)), dep)), nil
	}
	return nil, fmt.Errorf("unexpected %s %v", name, args)
}

func (f *fakeBuild) dep(mod string) string {
	if v, ok := f.deps[mod]; ok {
		return v
	}
	return "v0.9.0 h1:zGeZ="
}

func (f *fakeBuild) versionOf(bin string) string {
	if v, ok := f.version[bin]; ok {
		return v
	}
	return pinnedToolchain
}

// inspect reports what debug/buildinfo and debug/macho would.
func (f *fakeBuild) inspect(path string) (binInfo, error) {
	if _, err := os.Stat(path); err != nil {
		return binInfo{}, err
	}
	name := filepath.Base(path)
	info := binInfo{GoVersion: f.versionOf(name), Settings: map[string]string{
		"-buildmode": "exe", "-compiler": "gc", "-trimpath": "true",
		"CGO_ENABLED": "0", "GOOS": "darwin", "GOARCH": "arm64", "GOARM64": "v8.0",
	}}
	if name == "runner" {
		info.Symtab, info.DWARF = true, true
	}
	if name == "mygo" {
		info.Deps = map[string]string{"github.com/egoist/mygo": f.dep("github.com/egoist/mygo")}
	}
	return info, nil
}

func indexOf(xs []string, s string) int {
	for i, x := range xs {
		if x == s {
			return i
		}
	}
	return -1
}

func newFakeBuilder(t *testing.T, f *fakeBuild) (builder, string) {
	root := fakeTree(t)
	return builder{root: root, bin: filepath.Join(root, "bin"), self: pinnedToolchain,
		run: f.run, inspect: f.inspect, now: func() string { return "2026-11-17T07:00:00+08:00" }}, root
}

func TestBuildWritesManifest(t *testing.T) {
	f := &fakeBuild{dirty: " M tools/uibench/runner/main.go\n"}
	b, root := newFakeBuilder(t, f)
	m, err := b.build()
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Binaries) != 4 {
		t.Fatalf("binaries %d, want tango mygo nsgo runner", len(m.Binaries))
	}
	for _, e := range m.Binaries {
		if e.GoVersion != pinnedToolchain || e.SHA256 == "" || !strings.Contains(e.GoVersionM, "path\tx") {
			t.Errorf("%s: %+v", e.Name, e)
		}
		env := e.Build.Env
		if env.GOTOOLCHAIN != "go1.27.3" || env.CGO_ENABLED != "0" || env.GOOS != "darwin" || env.GOARCH != "arm64" || env.GOWORK != "off" {
			t.Errorf("%s env %+v", e.Name, env)
		}
		want := "go build -trimpath -ldflags -s -w -o"
		if e.Name == "runner" {
			want = "go build -trimpath -o"
		}
		if !strings.HasPrefix(strings.Join(e.Build.Args, " "), want) {
			t.Errorf("%s command %q, want prefix %q", e.Name, e.Build.Args, want)
		}
	}
	src := map[string]Source{}
	for _, e := range m.Binaries {
		src[e.Name] = e.Source
	}
	if s := src["tango"]; s.Commit != "abc123" || !s.Dirty || len(s.DirtyFiles) != 1 {
		t.Errorf("tango source %+v", s)
	}
	if s := src["mygo"]; s.Module != "github.com/egoist/mygo" || s.Version != "v0.9.0" || s.Sum != "h1:zGeZ=" {
		t.Errorf("mygo source %+v", s)
	}
	if s := src["nsgo"]; s.Version != "Unreleased" || len(s.TreeSHA1) != 40 {
		t.Errorf("nsgo source %+v", s)
	}
	// Every go command ran with exactly the pinned environment.
	for i, c := range f.calls {
		if !strings.Contains(c, ": go ") {
			continue
		}
		env := strings.Join(f.envs[i], "\n")
		for _, kv := range []string{"GOTOOLCHAIN=go1.27.3", "CGO_ENABLED=0", "GOOS=darwin", "GOARCH=arm64", "GOWORK=off"} {
			if !strings.Contains("\n"+env+"\n", "\n"+kv+"\n") {
				t.Errorf("%s: env lacks %s", c, kv)
			}
		}
	}
	// The manifest on disk is the one returned, and it verifies.
	if _, err := verifyManifest(b.bin, filepath.Join(b.bin, "runner"), f.inspect); err != nil {
		t.Fatalf("fresh manifest does not verify: %v", err)
	}
	_ = root
}

// No CGO_ENABLED=1 fallback: a framework that fails without cgo fails the
// build, and no manifest is left behind.
func TestBuildFailsLoudlyWithoutFallback(t *testing.T) {
	f := &fakeBuild{fail: map[string]bool{"mygo": true}}
	b, _ := newFakeBuilder(t, f)
	if err := os.MkdirAll(b.bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b.bin, manifestName), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := b.build(); err == nil || !strings.Contains(err.Error(), "mygo") {
		t.Fatalf("build err %v, want a mygo failure", err)
	}
	for _, c := range f.calls {
		if strings.Contains(c, "CGO_ENABLED=1") {
			t.Fatalf("retried with cgo: %s", c)
		}
	}
	for i, env := range f.envs {
		if strings.Contains(strings.Join(env, "\n"), "CGO_ENABLED=1") {
			t.Fatalf("call %s ran with CGO_ENABLED=1", f.calls[i])
		}
	}
	if _, err := os.Stat(filepath.Join(b.bin, manifestName)); !os.IsNotExist(err) {
		t.Fatalf("stale manifest left after a failed build: %v", err)
	}
}

// A binary built by another toolchain (GOTOOLCHAIN auto-upgraded for one
// framework) fails the build.
func TestBuildRefusesOtherToolchain(t *testing.T) {
	f := &fakeBuild{version: map[string]string{"nsgo": "go1.27.4"}}
	b, _ := newFakeBuilder(t, f)
	if _, err := b.build(); err == nil || !strings.Contains(err.Error(), "go1.27.4") {
		t.Fatalf("build err %v, want a toolchain mismatch", err)
	}
	b2, _ := newFakeBuilder(t, &fakeBuild{})
	b2.self = "go1.27.2"
	if _, err := b2.build(); err == nil || !strings.Contains(err.Error(), "go1.27.2") {
		t.Fatalf("builder on go1.27.2: err %v, want refusal", err)
	}
}

// A third_party/nsgo.version whose tree hash no longer matches the linked
// source fails the build: the manifest would name the wrong source.
func TestBuildRefusesStaleNSGoVersion(t *testing.T) {
	f := &fakeBuild{}
	b, root := newFakeBuilder(t, f)
	if err := os.WriteFile(filepath.Join(root, "nsgo-src", "c.go"), []byte("package nsgo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := b.build(); err == nil || !strings.Contains(err.Error(), "nsgo.version") {
		t.Fatalf("build err %v, want stale nsgo.version", err)
	}
}

// builtTree runs the fake build and returns the builder and fake.
func builtTree(t *testing.T) (builder, *fakeBuild) {
	t.Helper()
	f := &fakeBuild{}
	b, _ := newFakeBuilder(t, f)
	if _, err := b.build(); err != nil {
		t.Fatal(err)
	}
	return b, f
}

func TestVerifyRefusesMissingManifest(t *testing.T) {
	b, f := builtTree(t)
	if err := os.Remove(filepath.Join(b.bin, manifestName)); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyManifest(b.bin, filepath.Join(b.bin, "runner"), f.inspect); err == nil || !strings.Contains(err.Error(), "build.sh") {
		t.Fatalf("missing manifest: err %v, want refusal naming build.sh", err)
	}
}

func TestVerifyRefusesTamperedBinary(t *testing.T) {
	b, f := builtTree(t)
	if err := os.WriteFile(filepath.Join(b.bin, "mygo"), []byte("rebuilt by hand"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyManifest(b.bin, filepath.Join(b.bin, "runner"), f.inspect); err == nil || !strings.Contains(err.Error(), "mygo") || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("tampered mygo: err %v, want SHA-256 refusal", err)
	}
}

// The running runner must be the one the manifest lists (not `go run .`
// under whatever toolchain is installed).
func TestVerifyRefusesOtherRunner(t *testing.T) {
	b, f := builtTree(t)
	other := filepath.Join(t.TempDir(), "runner")
	if err := os.WriteFile(other, []byte("go run build"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyManifest(b.bin, other, f.inspect); err == nil || !strings.Contains(err.Error(), "runner") {
		t.Fatalf("other runner: err %v, want refusal", err)
	}
}

// editManifest rewrites bin/manifest.json through edit.
func editManifest(t *testing.T, bin string, edit func(*Manifest)) {
	t.Helper()
	p := filepath.Join(bin, manifestName)
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	edit(&m)
	raw, _ = json.Marshal(m)
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyRefusesMismatchedToolchain(t *testing.T) {
	// The binary itself was built by another toolchain.
	b, f := builtTree(t)
	f.version = map[string]string{"tango": "go1.27.2"}
	if _, err := verifyManifest(b.bin, filepath.Join(b.bin, "runner"), f.inspect); err == nil || !strings.Contains(err.Error(), "go1.27.2") {
		t.Fatalf("tango built by go1.27.2: err %v, want refusal", err)
	}
	// The manifest says one framework was built differently.
	b, f = builtTree(t)
	editManifest(t, b.bin, func(m *Manifest) {
		for i := range m.Binaries {
			if m.Binaries[i].Name == "nsgo" {
				m.Binaries[i].GoVersion = "go1.27.4"
				m.Binaries[i].Build.Env.GOTOOLCHAIN = "go1.27.4"
			}
		}
	})
	if _, err := verifyManifest(b.bin, filepath.Join(b.bin, "runner"), f.inspect); err == nil || !strings.Contains(err.Error(), "go1.27.4") {
		t.Fatalf("manifest nsgo go1.27.4: err %v, want refusal", err)
	}
}

func TestVerifyRefusesMismatchedCGOAndFlags(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*BinaryEntry)
		want string
	}{
		{"cgo", func(e *BinaryEntry) { e.Build.Env.CGO_ENABLED = "1"; e.Settings["CGO_ENABLED"] = "1" }, "CGO_ENABLED"},
		{"goarch", func(e *BinaryEntry) { e.Build.Env.GOARCH = "amd64"; e.Settings["GOARCH"] = "amd64" }, "GOARCH"},
		{"trimpath", func(e *BinaryEntry) { delete(e.Settings, "-trimpath") }, "-trimpath"},
		{"ldflags", func(e *BinaryEntry) { e.Build.Args = []string{"build", "-trimpath", "-o", "x", "."} }, "-ldflags"},
		{"stripped", func(e *BinaryEntry) { e.DWARF = true }, "DWARF"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, f := builtTree(t)
			editManifest(t, b.bin, func(m *Manifest) {
				for i := range m.Binaries {
					if m.Binaries[i].Name == "mygo" {
						tc.edit(&m.Binaries[i])
					}
				}
			})
			if _, err := verifyManifest(b.bin, filepath.Join(b.bin, "runner"), f.inspect); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err %v, want refusal naming %s", err, tc.want)
			}
		})
	}
}

// The tree hash is setup.sh's: shasum of the sorted ./path list of .go
// files, then shasum of that listing.
func TestGoTreeSHA1MatchesSetupSh(t *testing.T) {
	dir := t.TempDir()
	for rel, s := range map[string]string{"a.go": "package a\n", "z/b.go": "package b\n", "README": "x"} {
		p := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(s), 0o644)
	}
	got, err := goTreeSHA1(dir)
	if err != nil {
		t.Fatal(err)
	}
	// (cd dir && find . -type f -name '*.go' | LC_ALL=C sort | xargs shasum | shasum)
	const want = "68a7a1430b99c197ddb219c0d9e528acf6700dcf"
	if got != want {
		t.Fatalf("goTreeSHA1 = %s, want %s", got, want)
	}
}

// buildinfo of a real Go binary (this test binary) reads as binInfo.
func TestInspectRealBinary(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	info, err := inspectBinary(exe)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(info.GoVersion, "go1.") || info.Settings["GOOS"] == "" {
		t.Fatalf("inspect %s: %+v", exe, info)
	}
}
