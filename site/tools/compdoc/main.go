// Command compdoc extracts the usage documentation of the component pages of
// the Tango documentation site: for every component of the live catalog it
// reports the public entry points (constructor signatures and doc comments),
// the builder types they return with all their exported methods, the
// exported fields of struct types taken as parameters, and the Go source of
// every gallery demo of the component. It uses only the standard library
// (go/build, go/parser, go/ast, go/doc).
//
//	go run ./site/tools/compdoc -root . < catalog.json > component-docs.json
//
// The input is the component catalog written by site/tools/build-playground.mjs
// ({components: [{id, names, demos}], demos: [{name, title, source, line}]});
// the output is described by the output type below. Files are selected with
// the darwin/arm64 build constraints, as site/tools/apigen does, so the
// signatures match the API reference pages.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/doc"
	"go/doc/comment"
	"go/format"
	"go/parser"
	"go/scanner"
	"go/token"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
)

const module = "github.com/tangoui/tango"

// Package qualifiers used in catalog names ("chart.Line", "theme.HighContrast").
var qualifiers = map[string]string{
	"ui":     "ui",
	"chart":  "ui/chart",
	"mobile": "ui/mobile",
	"editor": "ui/editor",
	"theme":  "ui/theme",
	"input":  "ui/input",
}

type input struct {
	Components []struct {
		ID    string   `json:"id"`
		Names []string `json:"names"`
		Demos []string `json:"demos"`
	} `json:"components"`
	Demos []struct {
		Name   string `json:"name"`
		Title  string `json:"title"`
		Source string `json:"source"`
		Line   int    `json:"line"`
	} `json:"demos"`
}

type output struct {
	Components []component `json:"components"`
}

type component struct {
	ID string `json:"id"`
	// Entries are the resolved catalog names, in catalog order: package
	// functions (constructors), types and methods.
	Entries []entry `json:"entries"`
	// Types are the builder types returned by the entries (role "builder"),
	// the named types listed in the catalog (role "type") and the struct
	// types taken as parameters (role "param"), each once.
	Types []typeDoc `json:"types"`
	// Unresolved catalog names: not a declared symbol, or a method already
	// listed in one of the Types.
	Unresolved []string  `json:"unresolved,omitempty"`
	Examples   []example `json:"examples"`
}

type entry struct {
	Name       string `json:"name"`       // as in the catalog ("chart.Line", "ProSettings.Apply")
	Kind       string `json:"kind"`       // "func", "type" or "method"
	ImportPath string `json:"importPath"` // package of the declaration
	Signature  string `json:"signature,omitempty"`
	Doc        string `json:"doc"`
	DocMD      string `json:"docMarkdown"`
	Ref        string `json:"ref"`               // API reference page and anchor
	Returns    string `json:"returns,omitempty"` // builder type name, if any
	// Generic is set instead of Returns for a general-purpose type
	// (*ui.Box): its methods are not the component's own API.
	Generic *typeLink `json:"generic,omitempty"`
}

type typeDoc struct {
	Name       string   `json:"name"`
	Role       string   `json:"role"`
	ImportPath string   `json:"importPath"`
	Decl       string   `json:"decl,omitempty"` // non-struct types: the declaration
	Doc        string   `json:"doc"`
	DocMD      string   `json:"docMarkdown"`
	Ref        string   `json:"ref,omitempty"`
	Methods    []method `json:"methods"`
	Fields     []field  `json:"fields,omitempty"`
	// Embeds lists the general-purpose types the type embeds; their
	// promoted methods are left out of Methods.
	Embeds []typeLink `json:"embeds,omitempty"`
}

type typeLink struct {
	Name string `json:"name"` // "ui.Box"
	Ref  string `json:"ref"`
}

// generic are the general-purpose types (relative package path, type name)
// that constructors return or builders embed: they get a link to their API
// reference instead of a method table on every component page.
var generic = map[[2]string]bool{
	{"ui", "Box"}:         true,
	{"ui/theme", "Theme"}: true,
}

func genericLink(p *pkgInfo, name string) *typeLink {
	if !generic[[2]string{p.rel, name}] {
		return nil
	}
	return &typeLink{Name: path.Base(p.rel) + "." + name, Ref: refPage(p.rel) + "#" + name}
}

type method struct {
	Name      string `json:"name"`
	Signature string `json:"signature"` // without "func" and the receiver
	Synopsis  string `json:"synopsis"`
	Doc       string `json:"doc"`
	Ref       string `json:"ref,omitempty"`
	Promoted  string `json:"promoted,omitempty"` // embedded type the method comes from
}

type field struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Synopsis string `json:"synopsis"`
}

type example struct {
	Demo   string `json:"demo"`
	Title  string `json:"title"`
	Source string `json:"source"`
	Line   int    `json:"line"`
	// "literal" (Render: func(...){...}), "func" (named function) or
	// "call" (the registering call of a gallery helper such as tbPage).
	Kind    string   `json:"kind"`
	Code    string   `json:"code"`
	Helpers []helper `json:"helpers,omitempty"`
}

type helper struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"` // "func", "var", "const" or "type"
	Source string `json:"source"`
	Line   int    `json:"line"`
	Code   string `json:"code"`
}

func main() {
	root := flag.String("root", ".", "module root `dir` (where go.mod is)")
	in := flag.String("in", "-", "catalog JSON `file` (- for stdin)")
	out := flag.String("out", "-", "output JSON `file` (- for stdout)")
	flag.Parse()
	log.SetFlags(0)
	log.SetPrefix("compdoc: ")

	var r io.Reader = os.Stdin
	if *in != "-" {
		f, err := os.Open(*in)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		r = f
	}
	var cat input
	if err := json.NewDecoder(r).Decode(&cat); err != nil {
		log.Fatalf("reading catalog: %v", err)
	}
	ctx := build.Default
	ctx.GOOS, ctx.GOARCH, ctx.CgoEnabled = "darwin", "arm64", true
	l := &loader{root: *root, ctx: &ctx, pkgs: map[string]*pkgInfo{}}
	g, err := loadGallery(&ctx, *root)
	if err != nil {
		log.Fatal(err)
	}
	demos := map[string]int{}
	for i, d := range cat.Demos {
		demos[d.Name] = i
	}

	var res output
	unresolved := 0
	for _, c := range cat.Components {
		comp := component{ID: c.ID, Entries: []entry{}, Types: []typeDoc{}, Examples: []example{}}
		defPkg := "ui"
		if i := strings.Index(c.ID, "."); i > 0 {
			defPkg = qualifiers[c.ID[:i]]
		}
		b := &apiBuilder{l: l, comp: &comp, seen: map[string]bool{}}
		var later []string
		for _, name := range c.Names {
			if !b.resolve(name, defPkg, false) {
				later = append(later, name)
			}
		}
		b.paramTypes()
		// Names that are methods or fields (Form's Hidden, Table's Wrap)
		// are found once the builder and option types are known.
		for _, name := range later {
			if !b.resolve(name, defPkg, true) {
				comp.Unresolved = append(comp.Unresolved, name)
				unresolved++
			}
		}
		for _, name := range c.Demos {
			i, ok := demos[name]
			if !ok {
				log.Fatalf("%s: demo %s is not in the catalog", c.ID, name)
			}
			d := cat.Demos[i]
			ex, err := g.example(d.Source, d.Line, d.Name)
			if err != nil {
				log.Fatalf("%s: demo %s: %v", c.ID, name, err)
			}
			ex.Demo, ex.Title = d.Name, d.Title
			comp.Examples = append(comp.Examples, ex)
		}
		res.Components = append(res.Components, comp)
	}

	data, err := json.MarshalIndent(res, "", " ")
	if err != nil {
		log.Fatal(err)
	}
	data = append(data, '\n')
	if *out == "-" {
		os.Stdout.Write(data)
	} else if err := os.WriteFile(*out, data, 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("%d components, %d catalog names without a declaration of their own", len(res.Components), unresolved)
}

// ---- API ----

type pkgInfo struct {
	rel        string // "ui/chart"
	importPath string
	fset       *token.FileSet
	doc        *doc.Package
	funcs      map[string]*doc.Func
	types      map[string]*doc.Type
}

type loader struct {
	root string
	ctx  *build.Context
	pkgs map[string]*pkgInfo
}

// load parses a package of the module by its path relative to the root.
func (l *loader) load(rel string) (*pkgInfo, error) {
	if p, ok := l.pkgs[rel]; ok {
		return p, nil
	}
	dir := filepath.Join(l.root, filepath.FromSlash(rel))
	bp, err := l.ctx.ImportDir(dir, 0)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, name := range bp.GoFiles {
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	ip := module + "/" + rel
	// AllDecls keeps builders of unexported types; AllMethods lists the
	// methods promoted from every embedded type, as a caller sees them.
	d, err := doc.NewFromFiles(fset, files, ip, doc.AllDecls|doc.AllMethods)
	if err != nil {
		return nil, err
	}
	p := &pkgInfo{rel: rel, importPath: ip, fset: fset, doc: d, funcs: map[string]*doc.Func{}, types: map[string]*doc.Type{}}
	for _, f := range d.Funcs {
		p.funcs[f.Name] = f
	}
	for _, t := range d.Types {
		p.types[t.Name] = t
		for _, f := range t.Funcs {
			p.funcs[f.Name] = f
		}
	}
	l.pkgs[rel] = p
	return p, nil
}

// refPage is the API reference page of a package, as written by apigen.
func refPage(rel string) string { return "/reference/api/" + strings.ReplaceAll(rel, "/", "-") }

// docMD renders a doc comment as Markdown, with doc links pointing at the
// API reference pages (headings start at level 5 inside the component page).
func (p *pkgInfo) docMD(text string) string {
	pr := p.doc.Printer()
	pr.HeadingLevel = 5
	pr.DocLinkURL = func(dl *comment.DocLink) string {
		ip := dl.ImportPath
		if ip == "" {
			ip = p.importPath
		}
		anchor := dl.Name
		if dl.Recv != "" {
			anchor = dl.Recv + "." + dl.Name
		}
		if ip == module || strings.HasPrefix(ip, module+"/") {
			rel := strings.TrimPrefix(strings.TrimPrefix(ip, module), "/")
			u := "/reference/api/tango"
			if rel != "" {
				u = refPage(rel)
			}
			if anchor != "" {
				u += "#" + anchor
			}
			return u
		}
		u := "https://pkg.go.dev/" + ip
		if anchor != "" {
			u += "#" + anchor
		}
		return u
	}
	return strings.TrimSpace(string(pr.Markdown(p.doc.Parser().Parse(text))))
}

// synopsis is the first sentence of a doc comment (go/doc's Synopsis),
// except that the abbreviations "e.g." and "i.e." do not end it.
func (p *pkgInfo) synopsis(text string) string {
	r := strings.NewReplacer("e.g.", "e\x00g\x00", "i.e.", "i\x00e\x00")
	return strings.ReplaceAll(p.doc.Synopsis(r.Replace(text)), "\x00", ".")
}

func (p *pkgInfo) node(n any) string {
	var buf bytes.Buffer
	if err := format.Node(&buf, p.fset, n); err != nil {
		return fmt.Sprintf("/* %v */", err)
	}
	return buf.String()
}

// signature prints a function declaration without its body and doc comment;
// noRecv also drops "func" and the receiver ("Primary() *ButtonW").
func (p *pkgInfo) signature(d *ast.FuncDecl, noRecv bool) string {
	c := *d
	c.Body, c.Doc = nil, nil
	if noRecv {
		c.Recv = nil
		return strings.TrimPrefix(p.node(&c), "func ")
	}
	return p.node(&c)
}

type apiBuilder struct {
	l    *loader
	comp *component
	seen map[string]bool // importPath.Type already in comp.Types
	// params are the parameter types of the entries, resolved after them.
	params []paramRef
}

type paramRef struct {
	p    *pkgInfo
	expr ast.Expr
}

// resolve documents one catalog name; methodsOnly only accepts methods of
// types not yet listed on the page (a method of a listed type is in its
// table already and counts as resolved).
func (b *apiBuilder) resolve(name, defPkg string, methodsOnly bool) bool {
	rel, sym := defPkg, name
	if i := strings.Index(name, "."); i > 0 {
		if q, ok := qualifiers[name[:i]]; ok {
			rel, sym = q, name[i+1:]
		}
	}
	rels := []string{rel}
	if rel != "ui" && !strings.Contains(name, ".") {
		rels = append(rels, "ui")
	}
	for _, rel := range rels {
		p, err := b.l.load(rel)
		if err != nil {
			log.Fatalf("loading %s: %v", rel, err)
		}
		if typ, m, ok := strings.Cut(sym, "."); ok { // Type.Method
			if t := p.types[typ]; t != nil {
				for _, f := range t.Methods {
					if f.Name == m {
						b.addMethodEntry(name, p, t, f)
						return true
					}
				}
				if spec := typeSpec(t); spec != nil {
					if st, ok := spec.Type.(*ast.StructType); ok {
						for _, f := range fields(p, st) {
							if f.Name == m {
								b.comp.Entries = append(b.comp.Entries, entry{Name: name, Kind: "field", ImportPath: p.importPath,
									Signature: f.Name + " " + f.Type, Doc: f.Synopsis, DocMD: f.Synopsis, Ref: refPage(p.rel) + "#" + t.Name})
								b.addType(p, t, "type")
								return true
							}
						}
					}
				}
			}
			continue
		}
		if !methodsOnly {
			if f := p.funcs[sym]; f != nil && token.IsExported(sym) {
				b.addFunc(name, p, f)
				return true
			}
			if t := p.types[sym]; t != nil && token.IsExported(sym) {
				b.comp.Entries = append(b.comp.Entries, entry{Name: name, Kind: "type", ImportPath: p.importPath,
					Doc: t.Doc, DocMD: p.docMD(t.Doc), Ref: refPage(p.rel) + "#" + t.Name})
				b.addType(p, t, "type")
				return true
			}
			continue
		}
		if !token.IsExported(sym) {
			continue
		}
		// A builder obtained from another builder ("FormItem" is the
		// *FormItemW of FormW.Item).
		if t := p.types[sym+"W"]; t != nil {
			b.comp.Entries = append(b.comp.Entries, entry{Name: name, Kind: "type", ImportPath: p.importPath,
				Doc: t.Doc, DocMD: p.docMD(t.Doc), Ref: refPage(p.rel) + "#" + t.Name})
			b.addType(p, t, "builder")
			return true
		}
		for _, t := range b.comp.Types {
			for _, m := range t.Methods {
				if m.Name == sym {
					return true
				}
			}
			for _, f := range t.Fields {
				if f.Name == sym {
					return true
				}
			}
		}
		// A method of exactly one exported type of the package
		// (mobile.Area's TakePointer is (*ui.Box).TakePointer).
		var owner *doc.Type
		var fn *doc.Func
		n := 0
		for _, t := range p.doc.Types {
			if !token.IsExported(t.Name) {
				continue
			}
			for _, f := range t.Methods {
				if f.Name == sym && f.Level == 0 {
					owner, fn, n = t, f, n+1
				}
			}
		}
		if n == 1 {
			b.addMethodEntry(owner.Name+"."+name, p, owner, fn)
			return true
		}
	}
	return false
}

func (b *apiBuilder) addFunc(name string, p *pkgInfo, f *doc.Func) {
	e := entry{Name: name, Kind: "func", ImportPath: p.importPath, Signature: p.signature(f.Decl, false),
		Doc: f.Doc, DocMD: p.docMD(f.Doc), Ref: refPage(p.rel) + "#" + f.Name}
	if res := f.Decl.Type.Results; res != nil && len(res.List) > 0 {
		if rp, t := b.namedType(p, res.List[0].Type); t != nil && genericLink(rp, t.Name) != nil {
			e.Generic = genericLink(rp, t.Name)
		} else if t != nil && len(exportedMethods(t)) > 0 {
			e.Returns = t.Name
			b.addType(rp, t, "builder")
		}
	}
	for _, prm := range f.Decl.Type.Params.List {
		b.params = append(b.params, paramRef{p, prm.Type})
	}
	b.comp.Entries = append(b.comp.Entries, e)
}

func (b *apiBuilder) addMethodEntry(name string, p *pkgInfo, t *doc.Type, f *doc.Func) {
	b.comp.Entries = append(b.comp.Entries, entry{Name: name, Kind: "method", ImportPath: p.importPath,
		Signature: p.signature(f.Decl, false), Doc: f.Doc, DocMD: p.docMD(f.Doc), Ref: methodRef(p, t, f)})
}

// namedType resolves *T, T, T[X] and pkg.T to a type declared in the module.
func (b *apiBuilder) namedType(p *pkgInfo, x ast.Expr) (*pkgInfo, *doc.Type) {
	switch x := x.(type) {
	case *ast.StarExpr:
		return b.namedType(p, x.X)
	case *ast.IndexExpr:
		return b.namedType(p, x.X)
	case *ast.IndexListExpr:
		return b.namedType(p, x.X)
	case *ast.Ident:
		return p, p.types[x.Name]
	case *ast.SelectorExpr:
		id, ok := x.X.(*ast.Ident)
		if !ok {
			return nil, nil
		}
		rel, ok := importOf(p, id.Name)
		if !ok {
			return nil, nil
		}
		q, err := b.l.load(rel)
		if err != nil {
			return nil, nil
		}
		return q, q.types[x.Sel.Name]
	}
	return nil, nil
}

// importOf maps a package name used in p to a module-relative path.
func importOf(p *pkgInfo, name string) (string, bool) {
	for _, ip := range p.doc.Imports {
		if !strings.HasPrefix(ip, module+"/") {
			continue
		}
		if path.Base(ip) == name {
			return strings.TrimPrefix(ip, module+"/"), true
		}
	}
	return "", false
}

func exportedMethods(t *doc.Type) []*doc.Func {
	var out []*doc.Func
	for _, f := range t.Methods {
		if token.IsExported(f.Name) {
			out = append(out, f)
		}
	}
	return out
}

func methodRef(p *pkgInfo, t *doc.Type, f *doc.Func) string {
	owner := t.Name
	if f.Level > 0 {
		if o := strings.TrimPrefix(f.Orig, "*"); token.IsExported(o) {
			owner = o // documented under the embedded type
		}
	}
	if !token.IsExported(owner) {
		return ""
	}
	return refPage(p.rel) + "#" + owner + "." + f.Name
}

func (b *apiBuilder) addType(p *pkgInfo, t *doc.Type, role string) {
	key := p.importPath + "." + t.Name
	if b.seen[key] {
		return
	}
	b.seen[key] = true
	td := typeDoc{Name: t.Name, Role: role, ImportPath: p.importPath, Doc: t.Doc, DocMD: p.docMD(t.Doc), Methods: []method{}}
	if token.IsExported(t.Name) {
		td.Ref = refPage(p.rel) + "#" + t.Name
	}
	for _, f := range exportedMethods(t) {
		if f.Level > 0 {
			if l := genericLink(p, strings.TrimPrefix(f.Orig, "*")); l != nil {
				if !slices.Contains(td.Embeds, *l) {
					td.Embeds = append(td.Embeds, *l)
				}
				continue
			}
		}
		m := method{Name: f.Name, Signature: p.signature(f.Decl, true), Synopsis: p.synopsis(f.Doc), Doc: f.Doc, Ref: methodRef(p, t, f)}
		if f.Level > 0 {
			m.Promoted = strings.TrimPrefix(f.Orig, "*")
			// common[T] embedded as common[LineW]: show the signature
			// with the type arguments the caller sees.
			if args := embeddedArgs(p, typeSpec(t), f); args != nil {
				m.Signature = substitute(m.Signature, args)
				m.Promoted = substitute(m.Promoted, args)
			}
		}
		td.Methods = append(td.Methods, m)
	}
	sort.Slice(td.Methods, func(i, j int) bool { return td.Methods[i].Name < td.Methods[j].Name })
	if spec := typeSpec(t); spec == nil {
		// not a type declaration (cannot happen with go/doc)
	} else if st, ok := spec.Type.(*ast.StructType); ok {
		td.Fields = fields(p, st)
	} else {
		td.Decl = p.node(&ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{spec}})
	}
	b.comp.Types = append(b.comp.Types, td)
}

// embeddedArgs maps the type parameters of a promoted method's receiver
// (func (c *common[T]) ...) to the type arguments of the field embedding
// it in spec (common[LineW]); nil when the method is not generic or the
// embedding is not direct.
func embeddedArgs(p *pkgInfo, spec *ast.TypeSpec, f *doc.Func) map[string]string {
	if spec == nil {
		return nil
	}
	st, ok := spec.Type.(*ast.StructType)
	if !ok {
		return nil
	}
	// go/doc rewrites the receiver of a promoted method to the outer
	// type, so the original declaration is looked up on the embedded one.
	for _, fl := range st.Fields.List {
		if len(fl.Names) != 0 {
			continue
		}
		x := fl.Type
		if s, ok := x.(*ast.StarExpr); ok {
			x = s.X
		}
		args, base := indices(x)
		et := p.types[base]
		if len(args) == 0 || et == nil {
			continue
		}
		for _, m := range et.Methods {
			if m.Name != f.Name || m.Level != 0 || m.Decl.Recv == nil || len(m.Decl.Recv.List) != 1 {
				continue
			}
			recv := m.Decl.Recv.List[0].Type
			if s, ok := recv.(*ast.StarExpr); ok {
				recv = s.X
			}
			params, _ := indices(recv)
			if len(params) != len(args) {
				return nil
			}
			out := map[string]string{}
			for i, prm := range params {
				if id, ok := prm.(*ast.Ident); ok {
					out[id.Name] = p.node(args[i])
				}
			}
			return out
		}
	}
	return nil
}

// indices splits X[A, B] into its index expressions and base name.
func indices(x ast.Expr) ([]ast.Expr, string) {
	switch x := x.(type) {
	case *ast.IndexExpr:
		if id, ok := x.X.(*ast.Ident); ok {
			return []ast.Expr{x.Index}, id.Name
		}
	case *ast.IndexListExpr:
		if id, ok := x.X.(*ast.Ident); ok {
			return x.Indices, id.Name
		}
	}
	return nil, ""
}

// substitute replaces identifier tokens of a Go fragment.
func substitute(src string, m map[string]string) string {
	var sc scanner.Scanner
	fs := token.NewFileSet()
	file := fs.AddFile("", fs.Base(), len(src))
	sc.Init(file, []byte(src), nil, 0)
	var b strings.Builder
	last := 0
	for {
		pos, tok, lit := sc.Scan()
		if tok == token.EOF {
			break
		}
		if tok != token.IDENT {
			continue
		}
		if r, ok := m[lit]; ok {
			off := file.Offset(pos)
			b.WriteString(src[last:off])
			b.WriteString(r)
			last = off + len(lit)
		}
	}
	b.WriteString(src[last:])
	return b.String()
}

func typeSpec(t *doc.Type) *ast.TypeSpec {
	for _, s := range t.Decl.Specs {
		if ts, ok := s.(*ast.TypeSpec); ok && ts.Name.Name == t.Name {
			return ts
		}
	}
	return nil
}

func fields(p *pkgInfo, st *ast.StructType) []field {
	var out []field
	for _, f := range st.Fields.List {
		text := f.Doc.Text()
		if text == "" {
			text = f.Comment.Text()
		}
		typ := p.node(f.Type)
		if len(f.Names) == 0 { // embedded
			if id := embeddedName(f.Type); id != nil && token.IsExported(id.Name) {
				out = append(out, field{Name: id.Name, Type: typ + "（嵌入）", Synopsis: p.synopsis(text)})
			}
			continue
		}
		for _, n := range f.Names {
			if n.IsExported() {
				out = append(out, field{Name: n.Name, Type: typ, Synopsis: p.synopsis(text)})
			}
		}
	}
	return out
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

// paramTypes documents the exported struct types the entries take as
// parameters (directly, by pointer, in a slice or map, or variadic):
// their fields are the component's options (TableColumn, MenuItem...).
func (b *apiBuilder) paramTypes() {
	for _, pr := range b.params {
		x := pr.expr
		for {
			switch y := x.(type) {
			case *ast.StarExpr:
				x = y.X
				continue
			case *ast.ArrayType:
				x = y.Elt
				continue
			case *ast.Ellipsis:
				x = y.Elt
				continue
			case *ast.MapType:
				x = y.Value
				continue
			}
			break
		}
		p, t := b.namedType(pr.p, x)
		if t == nil || !token.IsExported(t.Name) || t.Name == "Context" {
			continue
		}
		if spec := typeSpec(t); spec == nil {
			continue
		} else if st, ok := spec.Type.(*ast.StructType); !ok || len(fields(p, st)) == 0 {
			continue
		}
		b.addType(p, t, "param")
	}
}

// ---- Examples ----

type gallery struct {
	root  string
	fset  *token.FileSet
	files map[string]*ast.File // by repository-relative path
	src   map[string][]byte
	decls map[string]declRef // package-level declarations by name
	// methods of the gallery's own types, by receiver type name
	methods map[string][]*ast.FuncDecl
}

type declRef struct {
	file string
	kind string
	node ast.Node // *ast.FuncDecl, or the *ast.GenDecl / spec to print
	gen  *ast.GenDecl
	spec ast.Spec
}

func loadGallery(ctx *build.Context, root string) (*gallery, error) {
	rel := "examples/ui-gallery"
	dir := filepath.Join(root, rel)
	bp, err := ctx.ImportDir(dir, 0)
	if err != nil {
		return nil, err
	}
	g := &gallery{root: root, fset: token.NewFileSet(), files: map[string]*ast.File{}, src: map[string][]byte{}, decls: map[string]declRef{}, methods: map[string][]*ast.FuncDecl{}}
	for _, name := range bp.GoFiles {
		file := rel + "/" + name
		src, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		f, err := parser.ParseFile(g.fset, file, src, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		g.files[file], g.src[file] = f, src
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv != nil && len(d.Recv.List) == 1 {
					if id := embeddedName(d.Recv.List[0].Type); id != nil {
						g.methods[id.Name] = append(g.methods[id.Name], d)
					}
				} else if d.Recv == nil && d.Name.Name != "init" && d.Name.Name != "main" {
					g.decls[d.Name.Name] = declRef{file: file, kind: "func", node: d}
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.ValueSpec:
						for _, n := range s.Names {
							g.decls[n.Name] = declRef{file: file, kind: d.Tok.String(), node: d, gen: d, spec: s}
						}
					case *ast.TypeSpec:
						g.decls[s.Name.Name] = declRef{file: file, kind: "type", node: d, gen: d, spec: s}
					}
				}
			}
		}
	}
	return g, nil
}

// example finds the Render function the catalog points at: a function
// literal starting on the line (Render: func(c *ui.Context) {...}), or a
// named function declared there. Demos registered through a gallery helper
// (tbPage("TableVirtual", ..., func() ... { var sel []int; return
// func(c *ui.Context) {...} })) show the whole registering call, so the
// state the Render closure captures is part of the example.
func (g *gallery) example(source string, line int, name string) (example, error) {
	f := g.files[source]
	if f == nil {
		return example{}, fmt.Errorf("%s is not a file of the gallery package", source)
	}
	var lit, renderLit *ast.FuncLit
	var litPath []ast.Node
	var decl *ast.FuncDecl
	var stack []ast.Node
	ast.Inspect(f, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		stack = append(stack, n)
		switch n := n.(type) {
		case *ast.FuncDecl:
			if g.fset.Position(n.Name.Pos()).Line == line {
				decl = n
			}
		case *ast.KeyValueExpr:
			if k, ok := n.Key.(*ast.Ident); ok && k.Name == "Render" {
				if fl, ok := n.Value.(*ast.FuncLit); ok && g.fset.Position(fl.Pos()).Line == line {
					renderLit = fl
				}
			}
		case *ast.FuncLit:
			if lit == nil && g.fset.Position(n.Pos()).Line == line {
				lit = n
				litPath = append([]ast.Node(nil), stack...)
			}
		}
		return true
	})
	ex := example{Source: source, Line: line}
	var body ast.Node
	switch {
	case decl != nil:
		start := decl.Pos()
		if decl.Doc != nil {
			start = decl.Doc.Pos()
		}
		ex.Kind, ex.Code, body = "func", g.text(source, start, decl.End()), decl
	case lit != nil:
		// litPath: File, FuncDecl, BlockStmt, top-level statement, ...
		var outer *ast.FuncDecl
		var stmt ast.Stmt
		if len(litPath) > 3 {
			outer, _ = litPath[1].(*ast.FuncDecl)
			stmt, _ = litPath[3].(ast.Stmt)
		}
		switch {
		case outer != nil && outer.Name.Name == "init" && renderLit == lit:
			ex.Kind, ex.Code, body = "literal", g.text(source, lit.Pos(), lit.End()), lit
		case outer != nil && outer.Name.Name == "init" && stmt != nil:
			ex.Line = g.fset.Position(stmt.Pos()).Line
			ex.Kind, ex.Code, body = "call", g.text(source, stmt.Pos(), stmt.End()), stmt
		case outer != nil && outer.Recv == nil:
			// The literal is the wrapper inside a registering helper
			// (p2ePage): show the helper's call for this demo.
			if file, call := g.callSite(outer.Name.Name, name); call != nil {
				ex.Source, ex.Line = file, g.fset.Position(call.Pos()).Line
				ex.Kind, ex.Code, body = "call", g.text(file, call.Pos(), call.End()), call
				break
			}
			fallthrough
		default:
			ex.Kind, ex.Code, body = "literal", g.text(source, lit.Pos(), lit.End()), lit
		}
	default:
		return ex, fmt.Errorf("no Render function at %s:%d", source, line)
	}
	ex.Helpers = g.helpers(body)
	return ex, nil
}

// callSite finds the statement calling fn with the string literal name as
// an argument.
func (g *gallery) callSite(fn, name string) (string, ast.Stmt) {
	quoted := strconv.Quote(name)
	files := make([]string, 0, len(g.files))
	for file := range g.files {
		files = append(files, file)
	}
	sort.Strings(files)
	for _, file := range files {
		var found ast.Stmt
		ast.Inspect(g.files[file], func(n ast.Node) bool {
			st, ok := n.(*ast.ExprStmt)
			if !ok || found != nil {
				return found == nil
			}
			call, ok := st.X.(*ast.CallExpr)
			if !ok {
				return true
			}
			if id, ok := call.Fun.(*ast.Ident); !ok || id.Name != fn {
				return true
			}
			for _, a := range call.Args {
				if bl, ok := a.(*ast.BasicLit); ok && bl.Value == quoted {
					found = st
				}
			}
			return true
		})
		if found != nil {
			return file, found
		}
	}
	return "", nil
}

// helpers returns the package-level declarations of the gallery the demo
// uses, directly or through other helpers, in first-use order.
func (g *gallery) helpers(root ast.Node) []helper {
	var out []helper
	done := map[string]bool{}
	specs := map[ast.Spec]int{} // spec printed by out[i]: "var a, b T" once
	if d, ok := root.(*ast.FuncDecl); ok {
		done[d.Name.Name] = true
	}
	queue := []ast.Node{root}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, name := range g.refs(n) {
			if done[name] {
				continue
			}
			done[name] = true
			d := g.decls[name]
			if i, ok := specs[d.spec]; ok && d.spec != nil {
				out[i].Name += ", " + name
				continue
			}
			h := helper{Name: name, Kind: d.kind, Source: d.file}
			switch node := d.node.(type) {
			case *ast.FuncDecl:
				start := node.Pos()
				if node.Doc != nil {
					start = node.Doc.Pos()
				}
				h.Code = g.text(d.file, start, node.End())
				h.Line = g.fset.Position(node.Pos()).Line
				queue = append(queue, node)
			case *ast.GenDecl:
				h.Code, h.Line = g.genText(d)
				queue = append(queue, d.spec)
				specs[d.spec] = len(out)
			}
			out = append(out, h)
			// A gallery type comes with its methods: the demo calls them
			// through values (d.render(c)) that refs cannot follow.
			if d.kind == "type" {
				for _, m := range g.methods[name] {
					start := m.Pos()
					if m.Doc != nil {
						start = m.Doc.Pos()
					}
					file := g.fset.Position(m.Pos()).Filename
					out = append(out, helper{Name: name + "." + m.Name.Name, Kind: "method", Source: file,
						Line: g.fset.Position(m.Pos()).Line, Code: g.text(file, start, m.End())})
					queue = append(queue, m)
				}
			}
		}
	}
	return out
}

// refs lists the identifiers of n that name package-level declarations of
// the gallery (not shadowed by a local declaration, not a field or method).
func (g *gallery) refs(n ast.Node) []string {
	var names []string
	skip := map[*ast.Ident]bool{}
	ast.Inspect(n, func(x ast.Node) bool {
		switch x := x.(type) {
		case *ast.SelectorExpr:
			skip[x.Sel] = true
		case *ast.KeyValueExpr:
			// Struct literal keys are field names; map keys are values,
			// but a helper used as a map key is too rare to matter.
			if k, ok := x.Key.(*ast.Ident); ok {
				skip[k] = true
			}
		case *ast.Field:
			for _, id := range x.Names {
				skip[id] = true
			}
		case *ast.Ident:
			if skip[x] {
				return true
			}
			d, ok := g.decls[x.Name]
			if !ok {
				return true
			}
			// Same-file references resolve to their declaration; a local
			// variable or parameter of the same name shadows the helper.
			if x.Obj != nil {
				switch decl := x.Obj.Decl.(type) {
				case *ast.FuncDecl:
					if decl != d.node {
						return true
					}
				case *ast.ValueSpec, *ast.TypeSpec:
					if decl != d.spec {
						return true
					}
				default:
					return true
				}
			}
			names = append(names, x.Name)
		}
		return true
	})
	return names
}

// genText prints a var/const/type declaration: the whole declaration when it
// has one spec, else "var name = ..." for the one spec (with its comment).
func (g *gallery) genText(d declRef) (string, int) {
	if len(d.gen.Specs) == 1 {
		start := d.gen.Pos()
		if d.gen.Doc != nil {
			start = d.gen.Doc.Pos()
		}
		return g.text(d.file, start, d.gen.End()), g.fset.Position(d.gen.Pos()).Line
	}
	start, doc := d.spec.Pos(), ""
	var cg *ast.CommentGroup
	switch s := d.spec.(type) {
	case *ast.ValueSpec:
		cg = s.Doc
	case *ast.TypeSpec:
		cg = s.Doc
	}
	if cg != nil {
		doc = dedent(g.text(d.file, cg.Pos(), cg.End())) + "\n"
	}
	return doc + d.gen.Tok.String() + " " + g.text(d.file, start, d.spec.End()), g.fset.Position(start).Line
}

// text returns the source between two positions, re-indented so that the
// code starts at column 0 (the first line starts mid-line).
func (g *gallery) text(file string, from, to token.Pos) string {
	src := g.src[file]
	s := string(src[g.fset.Position(from).Offset:g.fset.Position(to).Offset])
	return dedent(s)
}

// dedent removes the common leading tabs of all lines but the first.
func dedent(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) < 2 {
		return s
	}
	common := -1
	for _, l := range lines[1:] {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n := len(l) - len(strings.TrimLeft(l, "\t"))
		if common < 0 || n < common {
			common = n
		}
	}
	if common <= 0 {
		return s
	}
	for i := 1; i < len(lines); i++ {
		if len(lines[i]) >= common {
			lines[i] = lines[i][common:]
		} else {
			lines[i] = strings.TrimLeft(lines[i], "\t")
		}
	}
	return strings.Join(lines, "\n")
}
