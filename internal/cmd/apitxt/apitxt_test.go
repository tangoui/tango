package main

import (
	"bytes"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/mod/api/go")

const testMod = "testdata/mod"

// copyMod copies the test module to a temporary directory.
func copyMod(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir(testMod, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(testMod, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func testConfig(root string) (config, *bytes.Buffer, *bytes.Buffer) {
	var out, errOut bytes.Buffer
	return config{root: root, goos: []string{"linux", "darwin", "windows"}, out: &out, errOut: &errOut}, &out, &errOut
}

// TestGolden generates the test module's files and compares them with
// testdata/mod/api/go (go test -update rewrites them).
func TestGolden(t *testing.T) {
	tiers, err := readTiers(filepath.Join(testMod, "api", "tiers.txt"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := load(testMod, []string{"linux", "darwin", "windows"})
	if err != nil {
		t.Fatal(err)
	}
	files, err := generate(m, tiers)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(testMod, "api", "go")
	if *update {
		if err := writeFiles(dir, files); err != nil {
			t.Fatal(err)
		}
	}
	var diff bytes.Buffer
	drift, err := compareFiles(dir, files, &diff)
	if err != nil {
		t.Fatal(err)
	}
	if drift {
		t.Fatalf("golden files differ (go test -update rewrites them):\n%s", diff.String())
	}

	// The facts the format exists for, spelled out.
	want := map[string][]string{
		"m.txt": {
			// alias expansion of an internal type, with the alias name substituted
			"pkg example.com/m, type Event = internal/impl.Event",
			"pkg example.com/m, type Event struct, Kind Kind",
			"pkg example.com/m, type Event struct, URL string",
			"pkg example.com/m, method (Event) String() string",
			// promoted through an embedded internal type
			"pkg example.com/m, type App struct, ID int",
			"pkg example.com/m, method (*App) Close() error",
			// constants with values
			"pkg example.com/m, const PlaceTop Placement = 3",
			"pkg example.com/m, const Ratio untyped float = 1.5",
			"pkg example.com/m, const Name untyped string = \"m\"",
			// comparability and struct tags
			"pkg example.com/m, type Options struct, noncomparable",
			"pkg example.com/m, type Options struct, Title string `json:\"title\"`",
			"pkg example.com/m, type Sealed interface { Do, unexported methods }",
			"pkg example.com/m, func New(Options, ...string) *App",
			"pkg example.com/m, func Keys[K comparable, V any](map[K]V) []K",
		},
		"ui.txt": {
			"pkg example.com/m/ui, prop \"Button\" \"size\" Size",
			"pkg example.com/m/ui, prop \"Button\" \"ghost\" bool",
			"pkg example.com/m/ui, prop \"Card\" \"title\" string",
			"pkg example.com/m/ui, prop \"Card\" \"extra\" any",
		},
		"plat.txt": {
			"pkg example.com/m/plat, func Common()",
			"pkg example.com/m/plat, func OnlyLinux() [linux]",
			"pkg example.com/m/plat, func Unix() [darwin,linux]",
			"pkg example.com/m/plat, const Sep untyped string = \"/\" [darwin,linux]",
			"pkg example.com/m/plat, const Sep untyped string = \"\\\\\" [windows]",
		},
	}
	for name, lines := range want {
		have := map[string]bool{}
		for _, l := range files[name] {
			have[l] = true
		}
		for _, l := range lines {
			if !have[l] {
				t.Errorf("%s: missing line %s", name, l)
			}
		}
	}
	for _, l := range files["m.txt"] {
		if strings.Contains(l, "Point struct, noncomparable") || strings.Contains(l, "secret") || strings.Contains(l, "note") {
			t.Errorf("unexpected line %s", l)
		}
	}
	for _, l := range files["ui.txt"] {
		if strings.Contains(l, "skipped") {
			t.Errorf("a computed component name produced a prop line: %s", l)
		}
	}
	if _, ok := files["examples_demo.txt"]; ok {
		t.Error("excluded package has a golden file")
	}
}

func TestCheck(t *testing.T) {
	root := copyMod(t)
	cfg, out, errOut := testConfig(root)
	if code := run(cfg, false); code != 0 {
		t.Fatalf("check on the committed golden files: exit %d\n%s", code, errOut)
	}
	if !strings.Contains(out.String(), "up to date") {
		t.Errorf("stdout = %q", out)
	}
	// Phase 0 warnings: printed, not fatal.
	for _, s := range []string{
		"exp (experimental)",
		"m.UseUI -> ui.Context (not in api/stable/ui.txt)",
		"ui.Prop -> ui.Context (not in api/stable/ui.txt)",
	} {
		if !strings.Contains(errOut.String(), s) {
			t.Errorf("warnings lack %q:\n%s", s, errOut)
		}
	}
	cfg.strict = true
	if code := run(cfg, false); code != 1 {
		t.Errorf("-strict with warnings: exit %d, want 1", code)
	}
}

func TestCheckDrift(t *testing.T) {
	root := copyMod(t)
	golden := filepath.Join(root, "api", "go", "m.txt")

	// An API change without regenerating.
	src := filepath.Join(root, "m.go")
	data, _ := os.ReadFile(src)
	os.WriteFile(src, append(data, []byte("\nfunc Added() {}\n")...), 0o644)
	cfg, _, errOut := testConfig(root)
	if code := run(cfg, false); code != 1 {
		t.Fatalf("drift: exit %d, want 1\n%s", code, errOut)
	}
	for _, s := range []string{"api/go/m.txt: +1 -0 lines", "+ pkg example.com/m, func Added()", "apitxt -w"} {
		if !strings.Contains(errOut.String(), s) {
			t.Errorf("drift report lacks %q:\n%s", s, errOut)
		}
	}

	// -w repairs it.
	cfg, _, errOut = testConfig(root)
	if code := run(cfg, true); code != 0 {
		t.Fatalf("-w: exit %d\n%s", code, errOut)
	}
	if b, _ := os.ReadFile(golden); !strings.Contains(string(b), "func Added()") {
		t.Error("-w did not add the new function")
	}
	cfg, _, errOut = testConfig(root)
	if code := run(cfg, false); code != 0 {
		t.Fatalf("check after -w: exit %d\n%s", code, errOut)
	}

	// A stale file fails, and -w removes it.
	stale := filepath.Join(root, "api", "go", "gone.txt")
	os.WriteFile(stale, []byte("pkg example.com/m/gone, func F()\n"), 0o644)
	cfg, _, errOut = testConfig(root)
	if code := run(cfg, false); code != 1 || !strings.Contains(errOut.String(), "gone.txt: stale") {
		t.Errorf("stale file: exit %d\n%s", code, errOut)
	}
	cfg, _, _ = testConfig(root)
	run(cfg, true)
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("-w kept the stale file")
	}
}

func TestCheckErrors(t *testing.T) {
	root := copyMod(t)
	cfg, _, _ := testConfig(root)
	run(cfg, true)

	// An unclassified package and an allowlist entry that does not exist.
	os.MkdirAll(filepath.Join(root, "newpkg"), 0o755)
	os.WriteFile(filepath.Join(root, "newpkg", "n.go"), []byte("// Package newpkg is new.\npackage newpkg\n\nfunc F() {}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "api", "stable", "ui.txt"), []byte("Prop\nContext.ConfigProp\nGone\nContext.Missing\n"), 0o644)
	os.WriteFile(filepath.Join(root, "api", "stable", "plat.txt"), nil, 0o644)
	run(cfg, true) // regenerate for newpkg so only the checks fail

	cfg, _, errOut := testConfig(root)
	if code := run(cfg, false); code != 1 {
		t.Fatalf("exit %d, want 1\n%s", code, errOut)
	}
	for _, s := range []string{
		"package newpkg is not classified",
		"api/stable/ui.txt:3: Gone is not an exported symbol of ui",
		"api/stable/ui.txt:4: Context.Missing is not an exported symbol of ui",
		"api/stable/plat.txt does not belong to a split package",
	} {
		if !strings.Contains(errOut.String(), s) {
			t.Errorf("errors lack %q:\n%s", s, errOut)
		}
	}
	if strings.Contains(errOut.String(), "Context.ConfigProp is not") {
		t.Errorf("an existing member was reported:\n%s", errOut)
	}
	// Context has an allowlisted member, so references to it are no longer leaks.
	if strings.Contains(errOut.String(), "-> ui.Context") {
		t.Errorf("leak reported for a partly allowlisted type:\n%s", errOut)
	}
}

func TestTiers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tiers.txt")
	os.WriteFile(path, []byte(`
. stable   # root
ui split
ui/... experimental
ui/a11y stable
ui/editor/... experimental
sys/... stable
`), 0o644)
	ts, err := readTiers(path)
	if err != nil {
		t.Fatal(err)
	}
	for rel, want := range map[string]string{
		"":              tierStable,
		"ui":            tierSplit,
		"ui/chart":      tierExperimental,
		"ui/a11y":       tierStable,
		"ui/a11y/x":     tierExperimental,
		"ui/editor/buf": tierExperimental,
		"sys":           tierStable,
		"sys/tray":      tierStable,
		"asset":         "",
	} {
		if got, _ := ts.lookup(rel); got != want {
			t.Errorf("lookup(%q) = %q, want %q", rel, got, want)
		}
	}
	for _, bad := range []string{"ui fancy\n", "ui stable\nui experimental\n", "ui/... split\n", "ui\n"} {
		os.WriteFile(path, []byte(bad), 0o644)
		if _, err := readTiers(path); err == nil {
			t.Errorf("readTiers(%q) succeeded", bad)
		}
	}
}

func TestParseAPIDiffSymbol(t *testing.T) {
	const mod = "example.com/m"
	for line, want := range map[string][3]string{
		"- ./ui.Tooltip: changed from a to b  [linux]":        {"ui", "Tooltip", ""},
		"- ./ui.(*Label).Color: changed  [linux,darwin]":      {"ui", "Label", "Color"},
		"- ./ui.Label.Color: changed":                         {"ui", "Label", "Color"},
		"- ./ui.(*ProTableW[T]).Rows: removed":                {"ui", "ProTableW", "Rows"},
		"- ./ui.Pair[K, V].Key: removed":                      {"ui", "Pair", "Key"},
		"- ./ui/theme.Options.Dark: removed":                  {"ui/theme", "Options", "Dark"},
		"- ./runtime-js/runtimejs.Script: removed":            {"runtime-js/runtimejs", "Script", ""},
		"- package example.com/m/ui/foo: removed":             {"ui/foo", "", ""},
		"- AppEvent: old is comparable, new is not  [darwin]": {"", "AppEvent", ""},
		"- (*App).Close: removed":                             {"", "App", "Close"},
		"- SurfaceOptions.Title: changed from string to int":  {"", "SurfaceOptions", "Title"},
	} {
		rel, top, member := parseAPIDiffSymbol(mod, line)
		if got := [3]string{rel, top, member}; got != want {
			t.Errorf("parseAPIDiffSymbol(%q) = %q, want %q", line, got, want)
		}
	}
}

func TestClassify(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "api", "stable"), 0o755)
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/m\n"), 0o644)
	os.WriteFile(filepath.Join(root, "api", "tiers.txt"), []byte(". stable\nasset stable\nui split\nui/... experimental\nuihost split\n"), 0o644)
	os.WriteFile(filepath.Join(root, "api", "stable", "ui.txt"), []byte("# core\nButton\nLabel.Color\n"), 0o644)
	in := strings.Join([]string{
		"- ./asset.DevProxy: changed  [linux]",
		"- ./ui.Button: changed  [linux]",
		"- ./ui.(*ButtonW).Ghost: removed  [linux]",
		"- ./ui.(*Label).Color: changed  [linux]",
		"- ./ui.(*Label).Type: changed  [linux]",
		"- ./ui.Label: old is comparable, new is not  [linux]",
		"- ./ui/chart.Line: removed  [linux]",
		"- ./uihost.Open: changed  [linux]",
		"- ./newpkg.F: removed  [linux]",
		"- DevProxy: changed  [linux]",
		"- package example.com/m/ui/chart: removed",
		"- package example.com/m/uihost: removed",
		"ignored line",
	}, "\n")
	var out, errOut bytes.Buffer
	cfg := config{root: root, out: &out, errOut: &errOut}
	if code := runClassify(cfg, strings.NewReader(in)); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	got := strings.Split(strings.TrimSpace(out.String()), "\n")
	want := []string{
		"stable\t- ./asset.DevProxy: changed  [linux]",
		"stable\t- ./ui.Button: changed  [linux]",
		"experimental\t- ./ui.(*ButtonW).Ghost: removed  [linux]",
		"stable\t- ./ui.(*Label).Color: changed  [linux]",
		"experimental\t- ./ui.(*Label).Type: changed  [linux]",
		"stable\t- ./ui.Label: old is comparable, new is not  [linux]",
		"experimental\t- ./ui/chart.Line: removed  [linux]",
		"experimental\t- ./uihost.Open: changed  [linux]",
		"stable\t- ./newpkg.F: removed  [linux]",
		"stable\t- DevProxy: changed  [linux]",
		"experimental\t- package example.com/m/ui/chart: removed",
		"experimental\t- package example.com/m/uihost: removed",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("classify:\n got %q\nwant %q", got, want)
	}
}
