package main

import (
	"bufio"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/tangoui/tango/ui/shader/ir"
)

// layoutParams says where the layout file goes and how it names the
// shader package's types.
type layoutParams struct {
	base string // output base name ("quad")
	pkg  string // Go package of the generated file
	qual string // qualifier of the shader package's types ("shaders." or "")
	imp  string // import path of the shader package ("" in the same package)
}

// layoutFile returns the gofmt'ed <base>_layout.go.
func layoutFile(m *ir.Module, p layoutParams) ([]byte, error) {
	if len(m.Vertex.Params) != 1 {
		return nil, errors.New("vertex function has no input struct")
	}
	ref, ok := m.Vertex.Params[0].Type.(ir.StructRef)
	if !ok {
		return nil, errors.New("vertex input is not a struct")
	}
	in := ref.S
	prefix := exportName(p.base) + exportName(in.Name)
	typ := p.qual + in.Name

	var b strings.Builder
	b.WriteString(header + "\n")
	fmt.Fprintf(&b, "package %s\n\n", p.pkg)
	if p.imp != "" {
		fmt.Fprintf(&b, "import (\n\t\"unsafe\"\n\n\t%q\n)\n\n", p.imp)
	} else {
		b.WriteString("import \"unsafe\"\n\n")
	}
	fmt.Fprintf(&b, "// Host buffer layout of %s.metal and %s.hlsl, taken from the Go\n", p.base, p.base)
	b.WriteString("// structs they were generated from. The constants at the end of the\n")
	b.WriteString("// file stop the build when a struct no longer has this layout: run go\n// generate.\n\n")

	fmt.Fprintf(&b, "// %sStride is the size in bytes of %s:\n", prefix, typ)
	b.WriteString("// one element of the per-instance vertex buffer.\n")
	fmt.Fprintf(&b, "const %sStride = %d\n\n", prefix, in.Size)

	fields := append([]ir.Field(nil), in.Fields...)
	sort.Slice(fields, func(i, j int) bool { return fields[i].Attr.Location < fields[j].Attr.Location })
	fmt.Fprintf(&b, "// %sAttrs are the vertex attributes of %s\n", prefix, typ)
	b.WriteString("// by location: byte offset in the struct, scalar kind (\"f32\", \"i32\",\n// \"u32\") and component count.\n")
	fmt.Fprintf(&b, "var %sAttrs = [...]struct {\n\tLocation, Offset int\n\tKind string\n\tN int\n}{\n", prefix)
	for _, f := range fields {
		kind, n, err := attrFormat(f.Type)
		if err != nil {
			return nil, fmt.Errorf("vertex input field %s: %w", f.Name, err)
		}
		fmt.Fprintf(&b, "\t{%d, %d, %q, %d}, // %s\n", f.Attr.Location, f.Offset, kind, n, f.Name)
	}
	b.WriteString("}\n\n")

	texs := append([]*ir.TextureVar(nil), m.Textures...)
	sort.Slice(texs, func(i, j int) bool { return texs[i].Binding < texs[j].Binding })
	b.WriteString("// TextureDesc describes a texture binding of the shader: the Direct3D 11\n")
	b.WriteString("// device binds a clamp-to-edge sampler with this filter (\"nearest\" or\n")
	b.WriteString("// \"linear\") at register s<Binding> next to the texture at t<Binding>.\n")
	b.WriteString("type TextureDesc struct {\n\tBinding int\n\tFilter  string\n}\n\n")
	fmt.Fprintf(&b, "// %sTextures are the textures of %s.hlsl by binding.\n", exportName(p.base), p.base)
	fmt.Fprintf(&b, "var %sTextures = []TextureDesc{\n", exportName(p.base))
	for _, t := range texs {
		fmt.Fprintf(&b, "\t{Binding: %d, Filter: %q}, // %s\n", t.Binding, t.Filter, t.Name)
	}
	b.WriteString("}\n\n")

	var asserts, skipped []string
	assertStruct := func(s *ir.Struct, what string) {
		if p.qual != "" && !ast.IsExported(s.Name) {
			skipped = append(skipped, fmt.Sprintf("%s %s (unexported type)", what, s.Name))
			return
		}
		lit := p.qual + s.Name + "{}"
		asserts = append(asserts, fmt.Sprintf("// %s %s%s: %d bytes", what, p.qual, s.Name, s.Size))
		asserts = append(asserts, eq(fmt.Sprintf("unsafe.Sizeof(%s)", lit), s.Size)...)
		for _, f := range s.Fields {
			if p.qual != "" && !ast.IsExported(f.Name) {
				skipped = append(skipped, fmt.Sprintf("%s.%s (unexported field)", s.Name, f.Name))
				continue
			}
			asserts = append(asserts, eq(fmt.Sprintf("unsafe.Offsetof(%s.%s)", lit, f.Name), f.Offset)...)
		}
	}
	assertStruct(in, "vertex input")
	for _, u := range m.Uniforms {
		assertStruct(u.Type, fmt.Sprintf("uniform %s (buffer %d):", u.Name, u.Binding))
	}
	if len(skipped) > 0 {
		b.WriteString("// Not asserted (not accessible from this package):\n")
		for _, s := range skipped {
			fmt.Fprintf(&b, "//   - %s\n", s)
		}
		b.WriteString("\n")
	}
	b.WriteString("// Each pair fails to compile (constant overflow) unless both sides are\n// equal.\nconst (\n")
	for _, a := range asserts {
		fmt.Fprintf(&b, "\t%s\n", a)
	}
	b.WriteString(")\n")
	return format.Source([]byte(b.String()))
}

// eq returns two constant declarations that compile only when the uintptr
// constant expression x equals v.
func eq(x string, v int) []string {
	return []string{fmt.Sprintf("_ = uint(%s - %d)", x, v), fmt.Sprintf("_ = uint(%d - %s)", v, x)}
}

// attrFormat returns the scalar kind and component count of a vertex
// attribute type.
func attrFormat(t ir.Type) (string, int, error) {
	n := 1
	if v, ok := t.(ir.Vector); ok {
		t, n = v.Elem, v.N
	}
	switch t {
	case ir.F32:
		return "f32", n, nil
	case ir.I32:
		return "i32", n, nil
	case ir.U32:
		return "u32", n, nil
	}
	return "", 0, fmt.Errorf("type %v has no vertex format", t)
}

// exportName turns a file or type name into an exported Go identifier
// part: "quad" → "Quad", "rrect_fill" → "RrectFill".
func exportName(s string) string {
	var b strings.Builder
	up := true
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			up = true
			continue
		}
		if up {
			r = unicode.ToUpper(r)
			up = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

// outPackage returns the package name of the non-test .go files in dir
// other than tanshader's own output, or the directory's base name.
func outPackage(dir string) (string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return "", err
	}
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || strings.HasSuffix(f, "_layout.go") {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, parser.PackageClauseOnly)
		if err != nil {
			return "", err
		}
		return af.Name.Name, nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	return strings.ToLower(exportName(filepath.Base(abs))), nil
}

func sameDir(a, b string) (bool, error) {
	aa, err := filepath.Abs(a)
	if err != nil {
		return false, err
	}
	bb, err := filepath.Abs(b)
	if err != nil {
		return false, err
	}
	return filepath.Clean(aa) == filepath.Clean(bb), nil
}

// importPath returns the import path of the package in dir, from the
// module path of the nearest go.mod above it.
func importPath(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for d := abs; ; d = filepath.Dir(d) {
		if mod, ok, err := modulePath(filepath.Join(d, "go.mod")); err != nil {
			return "", err
		} else if ok {
			rel, err := filepath.Rel(d, abs)
			if err != nil {
				return "", err
			}
			if rel == "." {
				return mod, nil
			}
			return mod + "/" + filepath.ToSlash(rel), nil
		}
		if filepath.Dir(d) == d {
			return "", fmt.Errorf("no go.mod above %s", abs)
		}
	}
}

// modulePath reads the module directive of a go.mod file; ok is false
// when the file does not exist.
func modulePath(path string) (mod string, ok bool, err error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if rest, found := strings.CutPrefix(line, "module"); found && (rest == "" || rest[0] == ' ' || rest[0] == '\t') {
			mod = strings.Trim(strings.TrimSpace(rest), `"`)
			if mod != "" {
				return mod, true, nil
			}
		}
	}
	if err := sc.Err(); err != nil {
		return "", false, err
	}
	return "", false, fmt.Errorf("%s: no module directive", path)
}
