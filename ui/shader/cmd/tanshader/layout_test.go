package main

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testShader = `package lay

import "github.com/tangoui/tango/ui/shader/sl"

type Inst struct {
	Rect  sl.Vec4 ` + "`sl:\"loc=0\"`" + `
	P     sl.Vec2 ` + "`sl:\"loc=1\"`" + `
	Kind  uint32  ` + "`sl:\"loc=3\"`" + `
	Scale float32 ` + "`sl:\"loc=2\"`" + `
}

type Out struct {
	Pos sl.Vec4 ` + "`sl:\"position\"`" + `
}

type Globals struct {
	View sl.Vec2
	Pad  sl.Vec2
}

//sl:uniform 0
var U Globals

//sl:vertex
func vs(in Inst, vid uint32) Out {
	return Out{Pos: sl.V4(in.Rect.X+in.P.X, U.View.Y, in.Scale, float32(in.Kind))}
}

//sl:fragment
func fs(in Out) sl.Vec4 {
	return in.Pos
}
`

// typeCheck type-checks the .go files of dir as one package, returning
// the error messages.
func typeCheck(t *testing.T, dir string) []string {
	t.Helper()
	fset := token.NewFileSet()
	matches, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	var files []*ast.File
	for _, m := range matches {
		f, err := parser.ParseFile(fset, m, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	var errs []string
	conf := types.Config{Importer: importer.ForCompiler(fset, "source", nil), Error: func(err error) { errs = append(errs, err.Error()) }}
	conf.Check("lay", fset, files, nil)
	return errs
}

// TestLayoutFile: the generated layout file lists the vertex input's
// stride and attributes (by location, with Go offsets) and compiles with
// the structs it was generated from; once a struct changes, it no longer
// compiles.
func TestLayoutFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "lay.go")
	if err := os.WriteFile(src, []byte(testShader), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(dir, dir, ""); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "lay_layout.go"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{
		header,
		"package lay\n",
		"const LayInstStride = 32\n",
		`{0, 0, "f32", 4},  // Rect`,
		`{1, 16, "f32", 2}, // P`,
		`{2, 28, "f32", 1}, // Scale`,
		`{3, 24, "u32", 1}, // Kind`,
		"_ = uint(unsafe.Sizeof(Globals{}) - 16)",
		"_ = uint(16 - unsafe.Offsetof(Inst{}.P))",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("layout file lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"github.com`) {
		t.Errorf("same-package layout file imports the shader package:\n%s", got)
	}
	if errs := typeCheck(t, dir); len(errs) > 0 {
		t.Fatalf("generated layout does not compile:\n%s\n%s", strings.Join(errs, "\n"), got)
	}

	// Changing a struct without regenerating breaks the build.
	for _, c := range []struct{ name, old, new string }{
		{"field order", "\tKind  uint32", "\tPad   uint32\n\tKind  uint32"},
		{"uniform size", "\tPad  sl.Vec2\n", "\tPad  sl.Vec2\n\tMore float32\n"},
	} {
		changed := strings.Replace(testShader, c.old, c.new, 1)
		if changed == testShader {
			t.Fatalf("%s: edit did not apply", c.name)
		}
		if err := os.WriteFile(src, []byte(changed), 0o644); err != nil {
			t.Fatal(err)
		}
		errs := typeCheck(t, dir)
		if len(errs) == 0 || !strings.Contains(strings.Join(errs, "\n"), "overflows") {
			t.Errorf("%s: stale layout file still compiles (errors %q)", c.name, errs)
		}
	}
}

// TestLayoutFileOtherPackage: generated into another directory, the file
// imports the shader package by its module import path, qualifies its
// types and skips what it cannot reference.
func TestLayoutFileOtherPackage(t *testing.T) {
	out := t.TempDir()
	if err := run("../../msl/testdata/names", out, "gen"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "names_layout.go"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{
		"package gen\n",
		`"github.com/tangoui/tango/ui/shader/msl/testdata/names"`,
		"vertex input vertex (unexported type)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("layout file lacks %q:\n%s", want, got)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "names.metal")); err != nil {
		t.Error(err)
	}
	hlsl, err := os.ReadFile(filepath.Join(out, "names.hlsl"))
	if err != nil {
		t.Fatal(err)
	}
	if h := string(hlsl); !strings.HasPrefix(h, header) || strings.Count(h, "// Code generated") != 1 || !strings.Contains(h, "ps_main") {
		t.Errorf("names.hlsl lacks the single tanshader header or the entry points:\n%.300s", h)
	}
	for _, want := range []string{
		"type TextureDesc struct {",
		"var NamesTextures = []TextureDesc{",
		`{Binding: 0, Filter: "nearest"}, // texture`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("layout file lacks %q:\n%s", want, got)
		}
	}
}
