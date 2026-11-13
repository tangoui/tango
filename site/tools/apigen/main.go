// Command apigen writes the API reference of the Tango documentation site:
// one Markdown page per public package of the module (exported constants,
// variables, functions, types and methods with their doc comments) plus an
// index. It uses only the standard library (go/build, go/parser, go/doc).
//
//	go run ./site/tools/apigen -root . -out site/reference/api
//
// Files are selected with the build constraints of -goos/-goarch (default
// darwin/arm64, cgo on), so platform variants of the same declaration do not
// show up twice. Packages under internal/, examples/, site/, testdata and
// separate modules (cmd/tan) are skipped.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/doc"
	"go/doc/comment"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const module = "github.com/tangoui/tango"

func main() {
	root := flag.String("root", ".", "module root `dir` (where go.mod is)")
	out := flag.String("out", "site/reference/api", "output `dir`")
	goos := flag.String("goos", "darwin", "GOOS used to select files")
	goarch := flag.String("goarch", "arm64", "GOARCH used to select files")
	flag.Parse()
	log.SetFlags(0)
	log.SetPrefix("apigen: ")

	ctx := build.Default
	ctx.GOOS, ctx.GOARCH, ctx.CgoEnabled = *goos, *goarch, true

	dirs, err := packageDirs(*root)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.RemoveAll(*out); err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}

	var index []entry
	for _, rel := range dirs {
		e, ok, err := writePackage(&ctx, *root, rel, *out)
		if err != nil {
			log.Fatalf("%s: %v", rel, err)
		}
		if ok {
			index = append(index, e)
		}
	}
	if err := writeIndex(*out, index, *goos, *goarch); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %d package pages to %s", len(index), *out)
}

// entry is one package in the index.
type entry struct {
	ImportPath, Page, Synopsis, Group string
}

// packageDirs lists directories (relative, slash-separated) that may hold
// public packages of the root module.
func packageDirs(root string) ([]string, error) {
	var dirs []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		name := d.Name()
		if rel != "." {
			if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" ||
				name == "node_modules" || name == "internal" {
				return filepath.SkipDir
			}
			switch rel {
			case "examples", "site", "build", "bin", "cmd", "mobile", "docs", "scripts":
				return filepath.SkipDir
			}
			// Nested modules are documented on their own.
			if _, err := os.Stat(filepath.Join(p, "go.mod")); err == nil {
				return filepath.SkipDir
			}
		}
		dirs = append(dirs, rel)
		return nil
	})
	sort.Strings(dirs)
	return dirs, err
}

// writePackage documents the package in dir rel; ok is false when the
// directory holds no package for the selected platform or a command.
func writePackage(ctx *build.Context, root, rel, out string) (entry, bool, error) {
	dir := filepath.Join(root, filepath.FromSlash(rel))
	bp, err := ctx.ImportDir(dir, 0)
	if err != nil {
		if _, noGo := err.(*build.NoGoError); noGo {
			return entry{}, false, nil
		}
		// Packages whose files all exclude this platform (mobile-only
		// helpers) are skipped rather than failing the run.
		if strings.Contains(err.Error(), "build constraints exclude all Go files") {
			return entry{}, false, nil
		}
		return entry{}, false, err
	}
	if bp.Name == "main" {
		return entry{}, false, nil
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, name := range bp.GoFiles {
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ParseComments)
		if err != nil {
			return entry{}, false, err
		}
		files = append(files, f)
	}
	importPath := module
	if rel != "." {
		importPath = module + "/" + rel
	}
	pkg, err := doc.NewFromFiles(fset, files, importPath)
	if err != nil {
		return entry{}, false, err
	}

	page := pageName(rel)
	md := render(fset, pkg, importPath)
	if err := os.WriteFile(filepath.Join(out, page+".md"), md, 0o644); err != nil {
		return entry{}, false, err
	}
	group := "核心"
	switch {
	case strings.HasPrefix(rel, "sys/"):
		group = "系统能力 sys/"
	case strings.HasPrefix(rel, "service"):
		group = "服务 service/"
	case rel == "ui" || strings.HasPrefix(rel, "ui/") || rel == "uihost":
		group = "自绘 UI"
	}
	return entry{ImportPath: importPath, Page: page, Synopsis: pkg.Synopsis(pkg.Doc), Group: group}, true, nil
}

// pageName turns "sys/clipboard" into "sys-clipboard" and "." into "tango".
func pageName(rel string) string {
	if rel == "." {
		return "tango"
	}
	return strings.ReplaceAll(rel, "/", "-")
}

// render writes one package page.
func render(fset *token.FileSet, pkg *doc.Package, importPath string) []byte {
	var b bytes.Buffer
	pr := pkg.Printer()
	pr.HeadingLevel = 3
	pr.DocLinkURL = func(l *comment.DocLink) string {
		ip := l.ImportPath
		if ip == "" {
			ip = importPath
		}
		// Anchors keep Go's case, as on pkg.go.dev: #Updater, #Updater.Check.
		anchor := l.Name
		if l.Recv != "" {
			anchor = l.Recv + "." + l.Name
		}
		if strings.HasPrefix(ip, module) {
			rel := strings.TrimPrefix(strings.TrimPrefix(ip, module), "/")
			u := "./" + pageName(cmpOr(rel, "."))
			if anchor != "" {
				u += "#" + anchor
			}
			return u
		}
		u := "https://pkg.go.dev/" + ip
		if l.Name != "" {
			u += "#" + l.Name
			if l.Recv != "" {
				u = "https://pkg.go.dev/" + ip + "#" + l.Recv + "." + l.Name
			}
		}
		return u
	}
	docMD := func(text string) string {
		return string(pr.Markdown(pkg.Parser().Parse(text)))
	}

	fmt.Fprintf(&b, "---\ntitle: %s\noutline: [2, 3]\neditLink: false\n---\n\n", path.Base(importPath))
	fmt.Fprintf(&b, "# %s\n\n", path.Base(importPath))
	fmt.Fprintf(&b, "<div v-pre>\n\n")
	fmt.Fprintf(&b, "```go\nimport %q\n```\n\n", importPath)
	fmt.Fprintf(&b, "> 本页由 `site/tools/apigen` 从源码注释生成（按 darwin 构建标签选文件），请勿手改。\n\n")
	b.WriteString(docMD(pkg.Doc))
	b.WriteString("\n")

	if len(pkg.Consts) > 0 {
		b.WriteString("## 常量\n\n")
		for _, v := range pkg.Consts {
			writeValue(&b, fset, v, docMD)
		}
	}
	if len(pkg.Vars) > 0 {
		b.WriteString("## 变量\n\n")
		for _, v := range pkg.Vars {
			writeValue(&b, fset, v, docMD)
		}
	}
	if len(pkg.Funcs) > 0 {
		b.WriteString("## 函数\n\n")
		for _, f := range pkg.Funcs {
			writeFunc(&b, fset, f, "", docMD)
		}
	}
	if len(pkg.Types) > 0 {
		b.WriteString("## 类型\n\n")
		for _, t := range pkg.Types {
			fmt.Fprintf(&b, "### %s {#%s}\n\n", t.Name, t.Name)
			writeCode(&b, fset, t.Decl)
			b.WriteString(docMD(t.Doc))
			b.WriteString("\n")
			for _, v := range t.Consts {
				writeValue(&b, fset, v, docMD)
			}
			for _, v := range t.Vars {
				writeValue(&b, fset, v, docMD)
			}
			for _, f := range t.Funcs {
				writeFunc(&b, fset, f, "", docMD)
			}
			for _, m := range t.Methods {
				writeFunc(&b, fset, m, t.Name, docMD)
			}
		}
	}
	b.WriteString("\n</div>\n")
	return b.Bytes()
}

func writeValue(b *bytes.Buffer, fset *token.FileSet, v *doc.Value, docMD func(string) string) {
	writeCode(b, fset, v.Decl)
	if v.Doc != "" {
		b.WriteString(docMD(v.Doc))
		b.WriteString("\n")
	}
}

func writeFunc(b *bytes.Buffer, fset *token.FileSet, f *doc.Func, recv string, docMD func(string) string) {
	name, anchor := f.Name, f.Name
	if recv != "" {
		name = "(" + f.Recv + ") " + f.Name
		anchor = recv + "." + f.Name
		fmt.Fprintf(b, "#### `func %s` {#%s}\n\n", name, anchor)
	} else {
		fmt.Fprintf(b, "### `func %s` {#%s}\n\n", name, anchor)
	}
	// Print the signature without the body.
	d := *f.Decl
	d.Body = nil
	d.Doc = nil
	writeCode(b, fset, &d)
	b.WriteString(docMD(f.Doc))
	b.WriteString("\n")
}

// writeCode prints a declaration without its doc comment as a Go block.
func writeCode(b *bytes.Buffer, fset *token.FileSet, node any) {
	if g, ok := node.(*ast.GenDecl); ok {
		c := *g
		c.Doc = nil
		node = &c
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, node); err != nil {
		fmt.Fprintf(&buf, "// %v", err)
	}
	fmt.Fprintf(b, "```go\n%s\n```\n\n", buf.String())
}

func writeIndex(out string, entries []entry, goos, goarch string) error {
	var b bytes.Buffer
	b.WriteString("---\ntitle: API 参考\neditLink: false\n---\n\n# API 参考\n\n")
	fmt.Fprintf(&b, "每个公开包一页，由 `npm run docs:api`（`site/tools/apigen`，标准库 `go/doc`）从源码注释生成，按 %s/%s 的构建标签选文件。也可以用 `go doc` 在本地查看同样的内容。\n\n", goos, goarch)
	b.WriteString("::: tip 目录搬迁\n能力包位于 `github.com/tangoui/tango/sys/<名字>`，更新器位于 `github.com/tangoui/tango/service/updater`。\n:::\n\n")
	groups := []string{"核心", "系统能力 sys/", "服务 service/", "自绘 UI"}
	for _, g := range groups {
		fmt.Fprintf(&b, "## %s\n\n| 包 | 说明 |\n|---|---|\n", g)
		for _, e := range entries {
			if e.Group != g {
				continue
			}
			syn := strings.ReplaceAll(e.Synopsis, "|", "\\|")
			syn = strings.ReplaceAll(syn, "{{", "{ {")
			fmt.Fprintf(&b, "| [`%s`](./%s) | %s |\n", strings.TrimPrefix(e.ImportPath, module+"/"), e.Page, syn)
		}
		b.WriteString("\n")
	}
	return os.WriteFile(filepath.Join(out, "index.md"), b.Bytes(), 0o644)
}

func cmpOr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
