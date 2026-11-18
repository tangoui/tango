package text

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// gotextOnlyFiles are engine files that use go-text directly and carry a
// go-text build constraint; every other non-test file is shared by the
// engines and must not import go-text.
var gotextOnlyFiles = map[string]bool{
	"compact.go":               true,
	"hvar_specialize.go":       true,
	"variation_prune.go":       true,
	"table_resource.go":        true,
	"table_resource_darwin.go": true,
	"fontsxml.go":              true,
	"simple_numeric.go":        true,
	"coretext_shape_darwin.go": true,
	"engine_linux.go":          true,
	"engine_windows.go":        true,
	"engine_gotext_darwin.go":  true,
}

// TestSharedFilesImportNoGoText keeps go-text behind the engine
// primitives: only *_gotext.go files and the go-text engine files listed
// above may import github.com/go-text/...
func TestSharedFilesImportNoGoText(t *testing.T) {
	ents, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var bad []string
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") ||
			strings.HasSuffix(name, "_gotext.go") || gotextOnlyFiles[name] {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, im := range f.Imports {
			if p, _ := strconv.Unquote(im.Path.Value); strings.HasPrefix(p, "github.com/go-text/") {
				bad = append(bad, name)
				break
			}
		}
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Fatalf("shared files import go-text: %s", strings.Join(bad, ", "))
	}
}

// TestCoreTextBuildHasNoGoText checks that the macOS build of this package
// with -tags tango_coretext links no go-text package at all (go list -deps
// -tags tango_coretext, with the caller's GOFLAGS). Run only by that build
// on macOS; skipped otherwise.
func TestCoreTextBuildHasNoGoText(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	if !coreTextOnly {
		t.Skip("go-text build (run with -tags tango_coretext)")
	}
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go command")
	}
	cmd := exec.Command(gobin, "list", "-tags", "tango_coretext", "-deps", "-f", "{{.ImportPath}}", ".")
	cmd.Env = append(os.Environ(), "GOOS=darwin", "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, out)
	}
	var bad []string
	for _, p := range strings.Fields(string(out)) {
		if strings.HasPrefix(p, "github.com/go-text/") {
			bad = append(bad, p)
		}
	}
	if len(bad) > 0 {
		t.Fatalf("the tango_coretext darwin build links go-text: %s", strings.Join(bad, ", "))
	}
}
