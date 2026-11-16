package main

// The build manifest (spec §5): build.sh runs `go run . -build`, which
// builds the three scene programs and the runner with one pinned
// toolchain and environment and writes bin/manifest.json. The runner
// refuses to measure unless that manifest verifies, and copies it into
// every results file.

import (
	"bufio"
	"crypto/sha1"
	"crypto/sha256"
	"debug/buildinfo"
	"debug/macho"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// The one toolchain and target all four binaries are built with. Raising
// the toolchain means changing it here (and in build.sh) and rebuilding and
// re-measuring all three frameworks; never one alone.
const (
	pinnedToolchain = "go1.27.3"
	pinnedCGO       = "0"
	pinnedGOOS      = "darwin"
	pinnedGOARCH    = "arm64"
	manifestName    = "manifest.json"
	manifestSchema  = 1
)

// scenePrograms are the framework programs, built identically.
var scenePrograms = []string{"tango", "mygo", "nsgo"}

// releaseFlags are the scene programs' build flags; the runner gets
// -trimpath only.
var releaseFlags = []string{"-trimpath", "-ldflags", "-s -w"}

// Manifest is bin/manifest.json.
type Manifest struct {
	Schema    int           `json:"schema"`
	Created   string        `json:"created"`
	Toolchain string        `json:"toolchain"`
	Env       BuildEnv      `json:"env"`
	Binaries  []BinaryEntry `json:"binaries"`
}

// BuildEnv is the environment every go command of the build ran with
// (the rest of the caller's environment, minus GOFLAGS, is inherited).
type BuildEnv struct {
	GOTOOLCHAIN string `json:"GOTOOLCHAIN"`
	CGO_ENABLED string `json:"CGO_ENABLED"`
	GOOS        string `json:"GOOS"`
	GOARCH      string `json:"GOARCH"`
	GOWORK      string `json:"GOWORK"`
	GOFLAGS     string `json:"GOFLAGS"`
}

func pinnedEnv() BuildEnv {
	return BuildEnv{GOTOOLCHAIN: pinnedToolchain, CGO_ENABLED: pinnedCGO, GOOS: pinnedGOOS, GOARCH: pinnedGOARCH, GOWORK: "off"}
}

func (e BuildEnv) list() []string {
	return []string{"GOTOOLCHAIN=" + e.GOTOOLCHAIN, "CGO_ENABLED=" + e.CGO_ENABLED, "GOOS=" + e.GOOS,
		"GOARCH=" + e.GOARCH, "GOWORK=" + e.GOWORK, "GOFLAGS=" + e.GOFLAGS}
}

// BinaryEntry is one built program.
type BinaryEntry struct {
	Name      string       `json:"name"` // tango, mygo, nsgo, runner
	File      string       `json:"file"` // in bin/
	Source    Source       `json:"source"`
	Build     BuildCommand `json:"build"`
	SHA256    string       `json:"sha256"`
	GoVersion string       `json:"go_version"`
	// Settings are the binary's build settings (debug/buildinfo). Go omits
	// -ldflags from them under -trimpath, so Symtab and DWARF stand for
	// "-s -w": whether the Mach-O still has defined symbols / a __DWARF segment.
	Settings   map[string]string `json:"settings"`
	Symtab     bool              `json:"symtab"`
	DWARF      bool              `json:"dwarf"`
	GoVersionM string            `json:"go_version_m"` // `go version -m <bin>`, verbatim
}

// Source names the framework source a binary was built from.
type Source struct {
	Kind string `json:"kind"` // git, go.sum, nsgo.version
	// git (Tango and the runner, from this repository)
	Commit     string   `json:"commit,omitempty"`
	Dirty      bool     `json:"dirty,omitempty"`
	DirtyFiles []string `json:"dirty_files,omitempty"`
	// go.sum (MyGo, from the module cache)
	Module string `json:"module,omitempty"`
	Sum    string `json:"sum,omitempty"`
	// nsgo.version (NSGo, a linked source tree that is not a git checkout).
	// TreeSHA1 is setup.sh's hash of the .go files; go.mod and go.sum,
	// which it leaves out, are hashed here ("absent" when missing).
	Path        string `json:"path,omitempty"`
	TreeSHA1    string `json:"go_tree_sha1,omitempty"`
	GoModSHA256 string `json:"go_mod_sha256,omitempty"`
	GoSumSHA256 string `json:"go_sum_sha256,omitempty"`
	// Version: MyGo's module version, NSGo's CHANGELOG heading.
	Version string `json:"version,omitempty"`
}

// BuildCommand is the command exactly as run.
type BuildCommand struct {
	Dir  string   `json:"dir"` // relative to tools/uibench
	Args []string `json:"args"`
	Env  BuildEnv `json:"env"`
	Line string   `json:"line"` // the same, as one shell line
}

// binInfo is what a binary says about its own build.
type binInfo struct {
	GoVersion     string
	Settings      map[string]string
	Deps          map[string]string // module path -> "version sum"
	Symtab, DWARF bool
}

// inspectBinary reads a binary's build info and whether it is stripped.
func inspectBinary(path string) (binInfo, error) {
	bi, err := buildinfo.ReadFile(path)
	if err != nil {
		return binInfo{}, err
	}
	info := binInfo{GoVersion: bi.GoVersion, Settings: map[string]string{}, Deps: map[string]string{}}
	for _, s := range bi.Settings {
		info.Settings[s.Key] = s.Value
	}
	for _, d := range bi.Deps {
		info.Deps[d.Path] = d.Version + " " + d.Sum
	}
	f, err := macho.Open(path)
	if err != nil {
		return info, nil // not Mach-O (a test binary on another OS): no strip facts
	}
	defer f.Close()
	if f.Symtab != nil {
		for _, s := range f.Symtab.Syms {
			if s.Sect != 0 { // defined here, not an import from libSystem
				info.Symtab = true
				break
			}
		}
	}
	info.DWARF = f.Segment("__DWARF") != nil
	return info, nil
}

// builder builds the four binaries and writes the manifest.
type builder struct {
	root, bin string // tools/uibench and its bin/
	self      string // the toolchain the builder itself runs on
	run       func(dir string, env []string, name string, args ...string) ([]byte, error)
	inspect   func(path string) (binInfo, error)
	now       func() string
}

func newBuilder(root string) builder {
	return builder{root: root, bin: filepath.Join(root, "bin"), self: runtime.Version(),
		run: func(dir string, env []string, name string, args ...string) ([]byte, error) {
			c := exec.Command(name, args...)
			c.Dir, c.Env = dir, env
			return c.CombinedOutput()
		},
		inspect: inspectBinary,
		now:     func() string { return time.Now().Format(time.RFC3339) },
	}
}

// buildEnviron is the caller's environment with the pinned variables
// replacing whatever it had for them.
func buildEnviron(parent []string, e BuildEnv) []string {
	var out []string
	for _, kv := range parent {
		k, _, _ := strings.Cut(kv, "=")
		switch k {
		case "GOTOOLCHAIN", "CGO_ENABLED", "GOOS", "GOARCH", "GOWORK", "GOFLAGS":
			continue
		}
		out = append(out, kv)
	}
	return append(out, e.list()...)
}

// build builds tango, mygo, nsgo and the runner into bin/ and writes
// bin/manifest.json. Any failure stops the build and leaves no manifest:
// there is no CGO_ENABLED=1 fallback and no other toolchain.
func (b builder) build() (Manifest, error) {
	if b.self != pinnedToolchain {
		return Manifest{}, fmt.Errorf("the builder runs on %s, not %s: run it through build.sh (GOTOOLCHAIN=%s)", b.self, pinnedToolchain, pinnedToolchain)
	}
	if err := os.MkdirAll(b.bin, 0o755); err != nil {
		return Manifest{}, err
	}
	mpath := filepath.Join(b.bin, manifestName)
	if err := os.Remove(mpath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Manifest{}, err
	}
	_ = os.Remove(filepath.Join(b.bin, "build-info.txt")) // the old, unverified record

	env := pinnedEnv()
	environ := buildEnviron(os.Environ(), env)
	repo, err := b.gitSource()
	if err != nil {
		return Manifest{}, err
	}
	mygo, err := mygoSource(filepath.Join(b.root, "mygo"))
	if err != nil {
		return Manifest{}, err
	}
	nsgo, err := nsgoSource(filepath.Join(b.root, "third_party"))
	if err != nil {
		return Manifest{}, err
	}
	sources := map[string]Source{"tango": repo, "mygo": mygo, "nsgo": nsgo, "runner": repo}

	m := Manifest{Schema: manifestSchema, Created: b.now(), Toolchain: pinnedToolchain, Env: env}
	for _, name := range append(append([]string{}, scenePrograms...), "runner") {
		out := filepath.Join(b.bin, name)
		if err := os.Remove(out); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return Manifest{}, err
		}
		args := []string{"build", "-trimpath"}
		if name != "runner" {
			args = append([]string{"build"}, releaseFlags...)
		}
		args = append(args, "-o", out, ".")
		dir := filepath.Join(b.root, name)
		if o, err := b.run(dir, environ, "go", args...); err != nil {
			return Manifest{}, fmt.Errorf("%s: go %s failed with %s (no fallback): %v\n%s",
				name, strings.Join(args, " "), strings.Join(env.list(), " "), err, o)
		}
		e := BinaryEntry{Name: name, File: name, Source: sources[name],
			Build: BuildCommand{Dir: name, Args: append([]string{"go"}, relArgs(args, b.bin)...), Env: env}}
		e.Build.Line = "cd " + name + " && " + strings.Join(env.list(), " ") + " " + shellJoin(e.Build.Args)
		info, err := b.inspect(out)
		if err != nil {
			return Manifest{}, fmt.Errorf("%s: read build info: %v", name, err)
		}
		e.GoVersion, e.Settings, e.Symtab, e.DWARF = info.GoVersion, info.Settings, info.Symtab, info.DWARF
		vm, err := b.run(b.root, environ, "go", "version", "-m", out)
		if err != nil {
			return Manifest{}, fmt.Errorf("%s: go version -m: %v\n%s", name, err, vm)
		}
		e.GoVersionM = string(vm)
		first, _, _ := strings.Cut(e.GoVersionM, "\n")
		if v := strings.TrimSpace(first[strings.LastIndex(first, ":")+1:]); v != pinnedToolchain {
			return Manifest{}, fmt.Errorf("%s: go version -m says %s, want %s (did GOTOOLCHAIN switch toolchains?)", name, v, pinnedToolchain)
		}
		if e.SHA256, err = fileSHA256(out); err != nil {
			return Manifest{}, err
		}
		if err := checkEntry(e, info); err != nil {
			return Manifest{}, err
		}
		m.Binaries = append(m.Binaries, e)
	}
	if err := consistent(m); err != nil {
		return Manifest{}, err
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return Manifest{}, err
	}
	tmp := mpath + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return Manifest{}, err
	}
	return m, os.Rename(tmp, mpath)
}

// relArgs writes the -o path relative to tools/uibench, so the manifest
// does not depend on where the checkout is.
func relArgs(args []string, bin string) []string {
	out := append([]string{}, args...)
	for i := range out {
		if i > 0 && out[i-1] == "-o" {
			out[i] = "../bin/" + filepath.Base(out[i])
		}
	}
	return out
}

func shellJoin(args []string) string {
	q := make([]string, len(args))
	for i, a := range args {
		if a == "" || strings.ContainsAny(a, " \t\"'$") {
			a = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
		q[i] = a
	}
	return strings.Join(q, " ")
}

// gitSource is this repository's commit and whether the work tree differs
// from it (untracked files included: they can be built in).
func (b builder) gitSource() (Source, error) {
	rev, err := b.run(b.root, nil, "git", "rev-parse", "HEAD")
	if err != nil {
		return Source{}, fmt.Errorf("git rev-parse: %v\n%s", err, rev)
	}
	st, err := b.run(b.root, nil, "git", "status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return Source{}, fmt.Errorf("git status: %v\n%s", err, st)
	}
	s := Source{Kind: "git", Commit: strings.TrimSpace(string(rev))}
	for _, l := range strings.Split(string(st), "\n") {
		if strings.TrimSpace(l) != "" {
			s.DirtyFiles = append(s.DirtyFiles, strings.TrimSpace(l))
		}
	}
	s.Dirty = len(s.DirtyFiles) > 0
	return s, nil
}

// mygoSource reads the MyGo version mygo/go.mod requires and its go.sum line.
func mygoSource(dir string) (Source, error) {
	const mod = "github.com/egoist/mygo"
	gomod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return Source{}, err
	}
	var version string
	for _, l := range strings.Split(string(gomod), "\n") {
		f := strings.Fields(strings.TrimPrefix(strings.TrimSpace(l), "require "))
		if len(f) >= 2 && f[0] == mod {
			version = f[1]
		}
	}
	if version == "" {
		return Source{}, fmt.Errorf("mygo/go.mod does not require %s", mod)
	}
	sum, err := os.ReadFile(filepath.Join(dir, "go.sum"))
	if err != nil {
		return Source{}, err
	}
	for _, l := range strings.Split(string(sum), "\n") {
		if f := strings.Fields(l); len(f) == 3 && f[0] == mod && f[1] == version {
			return Source{Kind: "go.sum", Module: mod, Version: version, Sum: f[2]}, nil
		}
	}
	return Source{}, fmt.Errorf("mygo/go.sum has no %s %s", mod, version)
}

// nsgoSource reads third_party/nsgo.version (setup.sh) and checks its tree
// hash against the linked source, which may have changed since.
func nsgoSource(dir string) (Source, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "nsgo.version"))
	if err != nil {
		return Source{}, fmt.Errorf("%v (run setup.sh)", err)
	}
	s := Source{Kind: "nsgo.version"}
	for _, l := range strings.Split(string(raw), "\n") {
		k, v, _ := strings.Cut(strings.TrimSpace(l), " ")
		switch k {
		case "source":
			s.Path = v
		case "changelog":
			s.Version = v
		case "go-tree-sha1":
			s.TreeSHA1 = v
		}
	}
	if s.TreeSHA1 == "" {
		return Source{}, errors.New("third_party/nsgo.version has no go-tree-sha1 (run setup.sh)")
	}
	src, err := filepath.EvalSymlinks(filepath.Join(dir, "nsgo"))
	if err != nil {
		return Source{}, err
	}
	now, err := goTreeSHA1(src)
	if err != nil {
		return Source{}, err
	}
	if now != s.TreeSHA1 {
		return Source{}, fmt.Errorf("third_party/nsgo.version says go-tree-sha1 %s but the linked source hashes to %s: rerun setup.sh", s.TreeSHA1, now)
	}
	for _, f := range []struct {
		name string
		dst  *string
	}{{"go.mod", &s.GoModSHA256}, {"go.sum", &s.GoSumSHA256}} {
		sum, err := fileSHA256(filepath.Join(src, f.name))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			sum = "absent"
		case err != nil:
			return Source{}, err
		}
		*f.dst = sum
	}
	return s, nil
}

// goTreeSHA1 is setup.sh's hash: `find . -type f -name '*.go' | LC_ALL=C
// sort | xargs shasum | shasum` in dir.
func goTreeSHA1(dir string) (string, error) {
	var paths []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() && strings.HasSuffix(d.Name(), ".go") {
			rel, _ := filepath.Rel(dir, p)
			paths = append(paths, "./"+filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths) // byte order, as LC_ALL=C sort
	list := sha1.New()
	for _, p := range paths {
		f, err := os.Open(filepath.Join(dir, filepath.FromSlash(p)))
		if err != nil {
			return "", err
		}
		h := sha1.New()
		_, err = io.Copy(h, bufio.NewReader(f))
		f.Close()
		if err != nil {
			return "", err
		}
		fmt.Fprintf(list, "%s  %s\n", hex.EncodeToString(h.Sum(nil)), p)
	}
	return hex.EncodeToString(list.Sum(nil)), nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// checkEntry checks one entry against the pins and against what the
// binary itself says.
func checkEntry(e BinaryEntry, info binInfo) error {
	var errs []error
	bad := func(format string, a ...any) { errs = append(errs, fmt.Errorf(e.Name+": "+format, a...)) }
	if e.File != e.Name || strings.ContainsAny(e.File, `/\`) {
		bad("manifest file %q, want %q (the file the runner launches)", e.File, e.Name)
	}
	if e.GoVersion != pinnedToolchain || info.GoVersion != pinnedToolchain {
		bad("built with %s (manifest %s), want %s", info.GoVersion, e.GoVersion, pinnedToolchain)
	}
	if e.Build.Env != pinnedEnv() {
		bad("build environment %v, want %v", e.Build.Env.list(), pinnedEnv().list())
	}
	for k, want := range map[string]string{"CGO_ENABLED": pinnedCGO, "GOOS": pinnedGOOS, "GOARCH": pinnedGOARCH, "-trimpath": "true"} {
		if got := e.Settings[k]; got != want {
			bad("manifest setting %s=%q, want %q", k, got, want)
		}
		if got := info.Settings[k]; got != want {
			bad("binary setting %s=%q, want %q", k, got, want)
		}
	}
	if e.Name != "runner" {
		if !hasSeq(e.Build.Args, releaseFlags) {
			bad("build command %q lacks -trimpath -ldflags \"-s -w\"", e.Build.Args)
		}
		if e.Symtab || info.Symtab {
			bad("has a symbol table: not built with -ldflags \"-s -w\"")
		}
		if e.DWARF || info.DWARF {
			bad("has DWARF: not built with -ldflags \"-s -w\"")
		}
	}
	if e.Source.Module != "" {
		want := e.Source.Version + " " + e.Source.Sum
		if got := info.Deps[e.Source.Module]; got != want {
			bad("binary links %s %q, manifest source says %q", e.Source.Module, got, want)
		}
		if got := goVersionMDep(e.GoVersionM, e.Source.Module); got != want {
			bad("go version -m lists %s %q, manifest source says %q", e.Source.Module, got, want)
		}
	}
	return errors.Join(errs...)
}

// goVersionMDep is "version sum" of module's dep line in `go version -m`
// output, or "" when it has none.
func goVersionMDep(out, module string) string {
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) >= 3 && f[0] == "dep" && f[1] == module {
			sum := ""
			if len(f) >= 4 {
				sum = f[3]
			}
			return f[2] + " " + sum
		}
	}
	return ""
}

func hasSeq(xs, seq []string) bool {
	for i := 0; i+len(seq) <= len(xs); i++ {
		ok := true
		for j := range seq {
			if xs[i+j] != seq[j] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// consistent checks that the three scene programs were built alike: the
// same toolchain, environment, flags and build settings (vcs stamps aside).
func consistent(m Manifest) error {
	var ref *BinaryEntry
	var errs []error
	for _, name := range scenePrograms {
		e := m.entry(name)
		if e == nil {
			errs = append(errs, fmt.Errorf("manifest has no %s", name))
			continue
		}
		if ref == nil {
			ref = e
			continue
		}
		if e.GoVersion != ref.GoVersion {
			errs = append(errs, fmt.Errorf("toolchain differs: %s %s, %s %s", ref.Name, ref.GoVersion, e.Name, e.GoVersion))
		}
		if e.Build.Env != ref.Build.Env {
			errs = append(errs, fmt.Errorf("build environment differs: %s %v, %s %v", ref.Name, ref.Build.Env.list(), e.Name, e.Build.Env.list()))
		}
		if a, b := flagsOf(ref.Build.Args), flagsOf(e.Build.Args); a != b {
			errs = append(errs, fmt.Errorf("build flags differ: %s %q, %s %q", ref.Name, a, e.Name, b))
		}
		if e.Symtab != ref.Symtab || e.DWARF != ref.DWARF {
			errs = append(errs, fmt.Errorf("stripping differs: %s symtab=%t dwarf=%t, %s symtab=%t dwarf=%t", ref.Name, ref.Symtab, ref.DWARF, e.Name, e.Symtab, e.DWARF))
		}
		keys := map[string]bool{}
		for k := range ref.Settings {
			keys[k] = true
		}
		for k := range e.Settings {
			keys[k] = true
		}
		for k := range keys {
			if strings.HasPrefix(k, "vcs") {
				continue
			}
			if ref.Settings[k] != e.Settings[k] {
				errs = append(errs, fmt.Errorf("build setting %s differs: %s %q, %s %q", k, ref.Name, ref.Settings[k], e.Name, e.Settings[k]))
			}
		}
	}
	return errors.Join(errs...)
}

// flagsOf is a build command without its output path.
func flagsOf(args []string) string {
	var out []string
	for i := 0; i < len(args); i++ {
		if args[i] == "-o" {
			i++
			continue
		}
		out = append(out, args[i])
	}
	return strings.Join(out, " ")
}

func (m Manifest) entry(name string) *BinaryEntry {
	for i := range m.Binaries {
		if m.Binaries[i].Name == name {
			return &m.Binaries[i]
		}
	}
	return nil
}

// verifyManifest reads bin/manifest.json and refuses (an error) unless
// every binary, the running runner (self) included, is the one the
// manifest lists, built with the pinned toolchain and target, and the
// three scene programs were built alike.
func verifyManifest(bin, self string, inspect func(string) (binInfo, error)) (Manifest, error) {
	var m Manifest
	raw, err := os.ReadFile(filepath.Join(bin, manifestName))
	if err != nil {
		return m, fmt.Errorf("no verified build: %v (run build.sh)", err)
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return m, fmt.Errorf("%s: %v (run build.sh)", manifestName, err)
	}
	var errs []error
	if m.Schema != manifestSchema {
		errs = append(errs, fmt.Errorf("manifest schema %d, want %d", m.Schema, manifestSchema))
	}
	if m.Toolchain != pinnedToolchain || m.Env != pinnedEnv() {
		errs = append(errs, fmt.Errorf("manifest toolchain %s %v, want %s %v", m.Toolchain, m.Env.list(), pinnedToolchain, pinnedEnv().list()))
	}
	for _, name := range append(append([]string{}, scenePrograms...), "runner") {
		e := m.entry(name)
		if e == nil {
			errs = append(errs, fmt.Errorf("manifest has no %s", name))
			continue
		}
		paths := []string{filepath.Join(bin, e.File)}
		if name == "runner" && self != "" { // "": -unverified-runner
			paths = append(paths, self)
		}
		for _, p := range paths {
			sum, err := fileSHA256(p)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %v", name, err))
				continue
			}
			if sum != e.SHA256 {
				what := "stale or modified since build.sh"
				if p == self {
					what = "the running runner is not bin/runner from build.sh (use ../bin/runner, not go run)"
				}
				errs = append(errs, fmt.Errorf("%s: SHA-256 %s does not match the manifest's %s: %s", name, short(sum), short(e.SHA256), what))
			}
		}
		info, err := inspect(paths[0])
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: read build info: %v", name, err))
			continue
		}
		if err := checkEntry(*e, info); err != nil {
			errs = append(errs, err)
		}
	}
	if err := consistent(m); err != nil {
		errs = append(errs, err)
	}
	if err := errors.Join(errs...); err != nil {
		return m, fmt.Errorf("refusing to measure, the build does not verify:\n%w", err)
	}
	return m, nil
}

// dirty reports whether any binary was built from a work tree that
// differed from its commit.
func (m Manifest) dirty() bool {
	for _, e := range m.Binaries {
		if e.Source.Dirty {
			return true
		}
	}
	return false
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// buildLine is a round's one-line build summary: the manifest entry with
// sha, the SHA-256 of the binary actually launched.
func (m Manifest) buildLine(name, sha string) string {
	e := m.entry(name)
	if e == nil {
		return ""
	}
	line := fmt.Sprintf("%s %s %s/%s %s sha256=%s", e.GoVersion, "CGO_ENABLED="+e.Build.Env.CGO_ENABLED,
		e.Build.Env.GOOS, e.Build.Env.GOARCH, flagsOf(e.Build.Args), sha)
	if e.Source.Dirty {
		line += " (dirty build)"
	}
	return line
}
