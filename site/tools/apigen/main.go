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
//
// Experimental symbols get a 「实验性」 badge: packages are classified by
// api/tiers.txt and, for split packages, by the stable-symbol allowlists
// api/stable/<pkg>.txt (docs/release.md, "API 稳定性").
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

	tiers, err := readTiers(*root)
	if err != nil {
		log.Fatal(err)
	}
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
		e, ok, err := writePackage(&ctx, tiers, *root, rel, *out)
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
	Tier                              string
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
func writePackage(ctx *build.Context, tiers *tierSet, root, rel, out string) (entry, bool, error) {
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

	st := stability{tier: tiers.lookup(rel)}
	if st.tier == tierExclude {
		return entry{}, false, nil
	}
	if st.tier == tierSplit {
		if st.allow, err = readAllowlist(root, rel); err != nil {
			return entry{}, false, err
		}
	}

	page := pageName(rel)
	md := render(fset, pkg, importPath, rel, st)
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
	return entry{ImportPath: importPath, Page: page, Synopsis: pkg.Synopsis(pkg.Doc), Group: group, Tier: st.tier}, true, nil
}

// pageName turns "sys/clipboard" into "sys-clipboard" and "." into "tango".
func pageName(rel string) string {
	if rel == "." {
		return "tango"
	}
	return strings.ReplaceAll(rel, "/", "-")
}

// render writes one package page.
func render(fset *token.FileSet, pkg *doc.Package, importPath, rel string, st stability) []byte {
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
	switch st.tier {
	case tierExperimental:
		fmt.Fprintf(&b, "# %s %s\n\n", path.Base(importPath), badge)
		b.WriteString("::: warning 实验性\n本包属于实验层：任何 0.x 版本都可能有不兼容变化（`docs/release.md`「API 稳定性」）。\n:::\n\n")
	case tierSplit:
		fmt.Fprintf(&b, "# %s\n\n", path.Base(importPath))
		fmt.Fprintf(&b, "::: info 分层\n只有 `api/stable/%s.txt` 列出的符号受兼容性承诺约束。标有「实验性」的符号，以及类型下「实验性成员」列出的字段，任何 0.x 版本都可能有不兼容变化（`docs/release.md`「API 稳定性」）。\n:::\n\n", strings.ReplaceAll(rel, "/", "_"))
	default:
		fmt.Fprintf(&b, "# %s\n\n", path.Base(importPath))
	}
	fmt.Fprintf(&b, "<div v-pre>\n\n")
	fmt.Fprintf(&b, "```go\nimport %q\n```\n\n", importPath)
	fmt.Fprintf(&b, "> 本页由 `site/tools/apigen` 从源码注释生成（按 darwin 构建标签选文件），请勿手改。\n\n")
	b.WriteString(docMD(pkg.Doc))
	b.WriteString("\n")

	mark := st.tier == tierSplit
	value := func(v *doc.Value) {
		writeValue(&b, fset, v, docMD)
		if mark {
			writeValueMark(&b, v, st)
		}
	}
	fn := func(f *doc.Func, recv string) {
		exp := mark && st.experimental(f.Name)
		if recv != "" {
			exp = mark && st.memberExperimental(recv, f.Name)
		}
		writeFunc(&b, fset, f, recv, exp, docMD)
	}
	if len(pkg.Consts) > 0 {
		b.WriteString("## 常量\n\n")
		for _, v := range pkg.Consts {
			value(v)
		}
	}
	if len(pkg.Vars) > 0 {
		b.WriteString("## 变量\n\n")
		for _, v := range pkg.Vars {
			value(v)
		}
	}
	if len(pkg.Funcs) > 0 {
		b.WriteString("## 函数\n\n")
		for _, f := range pkg.Funcs {
			fn(f, "")
		}
	}
	if len(pkg.Types) > 0 {
		b.WriteString("## 类型\n\n")
		for _, t := range pkg.Types {
			title := t.Name
			if mark && st.experimental(t.Name) {
				title += " " + badge
			}
			fmt.Fprintf(&b, "### %s {#%s}\n\n", title, t.Name)
			writeCode(&b, fset, t.Decl)
			b.WriteString(docMD(t.Doc))
			b.WriteString("\n")
			if mark && st.partial(t.Name) {
				writeFieldMark(&b, t, st)
			}
			for _, v := range t.Consts {
				value(v)
			}
			for _, v := range t.Vars {
				value(v)
			}
			for _, f := range t.Funcs {
				fn(f, "")
			}
			for _, m := range t.Methods {
				fn(m, t.Name)
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

// writeValueMark notes the experimental names of a const/var group on a
// split package page.
func writeValueMark(b *bytes.Buffer, v *doc.Value, st stability) {
	var exp []string
	for _, n := range v.Names {
		if token.IsExported(n) && st.experimental(n) {
			exp = append(exp, "`"+n+"`")
		}
	}
	switch {
	case len(exp) == 0:
	case len(exp) == len(v.Names):
		b.WriteString(badge + "\n\n")
	default:
		fmt.Fprintf(b, "%s %s\n\n", badge, strings.Join(exp, "、"))
	}
}

// writeFieldMark lists the experimental fields of a type whose stable
// members are listed one by one (methods carry their own badge).
func writeFieldMark(b *bytes.Buffer, t *doc.Type, st stability) {
	var exp []string
	for _, spec := range t.Decl.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok || ts.Name.Name != t.Name {
			continue
		}
		s, ok := ts.Type.(*ast.StructType)
		if !ok {
			continue
		}
		for _, f := range s.Fields.List {
			names := f.Names
			if len(names) == 0 { // embedded: the field is named after its type
				if id := embeddedName(f.Type); id != nil {
					names = []*ast.Ident{id}
				}
			}
			for _, n := range names {
				if n.IsExported() && st.memberExperimental(t.Name, n.Name) {
					exp = append(exp, "`"+n.Name+"`")
				}
			}
		}
	}
	b.WriteString("只有部分成员是稳定的：未标「实验性」的方法")
	if len(exp) > 0 {
		fmt.Fprintf(b, "和下列以外的字段受兼容性承诺约束。\n\n%s 实验性成员：%s\n\n", badge, strings.Join(exp, "、"))
	} else {
		b.WriteString("和全部字段受兼容性承诺约束。\n\n")
	}
}

func embeddedName(x ast.Expr) *ast.Ident {
	switch x := x.(type) {
	case *ast.Ident:
		return x
	case *ast.StarExpr:
		return embeddedName(x.X)
	case *ast.SelectorExpr:
		return x.Sel
	case *ast.IndexExpr:
		return embeddedName(x.X)
	case *ast.IndexListExpr:
		return embeddedName(x.X)
	}
	return nil
}

func writeFunc(b *bytes.Buffer, fset *token.FileSet, f *doc.Func, recv string, experimental bool, docMD func(string) string) {
	name, anchor := f.Name, f.Name
	mark := ""
	if experimental {
		mark = " " + badge
	}
	if recv != "" {
		name = "(" + f.Recv + ") " + f.Name
		anchor = recv + "." + f.Name
		fmt.Fprintf(b, "#### `func %s`%s {#%s}\n\n", name, mark, anchor)
	} else {
		fmt.Fprintf(b, "### `func %s`%s {#%s}\n\n", name, mark, anchor)
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
	b.WriteString("标「实验性」的包任何 0.x 版本都可能有不兼容变化；标「部分实验性」的包只有 `api/stable/<包>.txt` 列出的符号是稳定层，页面里逐个标出实验性符号（`docs/release.md`「API 稳定性」）。\n\n")
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
			name := fmt.Sprintf("[`%s`](./%s)", strings.TrimPrefix(e.ImportPath, module+"/"), e.Page)
			switch e.Tier {
			case tierExperimental:
				name += " " + badge
			case tierSplit:
				name += ` <span class="VPBadge info" title="Only the symbols in api/stable are stable">部分实验性</span>`
			}
			fmt.Fprintf(&b, "| %s | %s |\n", name, syn)
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
