package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/importer"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// module is the root module loaded for every platform.
type module struct {
	path string   // module path, e.g. github.com/tangoui/tango
	goos []string // platforms in load order
	pkgs map[string]*pkgInfo
}

// pkgInfo is one non-internal package of the module.
type pkgInfo struct {
	path   string // import path
	rel    string // path relative to the module; "" for the root package
	name   string // package name
	isMain bool
	plat   map[string]*platPkg // by GOOS; missing when no file builds there
}

// platPkg is a package as type-checked for one platform.
type platPkg struct {
	goos  string
	types *types.Package
	dir   string
	files []string // absolute paths of the Go files that build on goos
	imp   types.Importer
	fset  *token.FileSet
}

// listPkg is the subset of `go list -json` used here.
type listPkg struct {
	ImportPath string
	Name       string
	Dir        string
	Export     string
	GoFiles    []string
	DepOnly    bool
	Module     *struct {
		Path string
		Main bool
	}
	Error *struct{ Err string }
}

// load lists and imports the module's packages for each platform.
func load(root string, goosList []string) (*module, error) {
	type result struct {
		list []*listPkg
		imp  types.Importer
		fset *token.FileSet
		err  error
	}
	results := make([]result, len(goosList))
	var wg sync.WaitGroup
	for i, goos := range goosList {
		wg.Add(1)
		go func() {
			defer wg.Done()
			list, err := goList(root, goos)
			if err != nil {
				results[i].err = err
				return
			}
			exports := map[string]string{}
			for _, p := range list {
				if p.Export != "" {
					exports[p.ImportPath] = p.Export
				}
			}
			fset := token.NewFileSet()
			imp := importer.ForCompiler(fset, "gc", func(path string) (io.ReadCloser, error) {
				f, ok := exports[path]
				if !ok {
					return nil, fmt.Errorf("no export data for %q (%s)", path, goos)
				}
				return os.Open(f)
			})
			results[i] = result{list: list, imp: imp, fset: fset}
		}()
	}
	wg.Wait()

	m := &module{goos: goosList, pkgs: map[string]*pkgInfo{}}
	for i, goos := range goosList {
		r := results[i]
		if r.err != nil {
			return nil, r.err
		}
		for _, p := range r.list {
			if p.DepOnly || p.Module == nil || !p.Module.Main {
				continue
			}
			if m.path == "" {
				m.path = p.Module.Path
			} else if m.path != p.Module.Path {
				return nil, fmt.Errorf("packages from two main modules: %s and %s", m.path, p.Module.Path)
			}
			rel := relPath(m.path, p.ImportPath)
			if isInternal(rel) {
				continue
			}
			if p.Error != nil {
				if strings.Contains(p.Error.Err, "build constraints exclude all Go files") {
					continue
				}
				return nil, fmt.Errorf("%s (%s): %s", p.ImportPath, goos, p.Error.Err)
			}
			info := m.pkgs[p.ImportPath]
			if info == nil {
				info = &pkgInfo{path: p.ImportPath, rel: rel, name: p.Name, plat: map[string]*platPkg{}}
				m.pkgs[p.ImportPath] = info
			}
			if p.Name == "main" {
				info.isMain = true
				continue
			}
			tp, err := r.imp.Import(p.ImportPath)
			if err != nil {
				return nil, fmt.Errorf("import %s (%s): %v", p.ImportPath, goos, err)
			}
			files := make([]string, len(p.GoFiles))
			for j, f := range p.GoFiles {
				files[j] = filepath.Join(p.Dir, f)
			}
			info.plat[goos] = &platPkg{goos: goos, types: tp, dir: p.Dir, files: files, imp: r.imp, fset: r.fset}
		}
	}
	if m.path == "" {
		return nil, errors.New("no packages found")
	}
	return m, nil
}

// goList runs go list -export for one platform. The workspace is turned
// off so only the module in root is listed (go.work also pulls in cmd/tan).
func goList(root, goos string) ([]*listPkg, error) {
	cmd := exec.Command("go", "list", "-e", "-export", "-deps",
		"-json=ImportPath,Name,Dir,Export,GoFiles,DepOnly,Module,Error", "./...")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH=amd64", "CGO_ENABLED=0", "GOWORK=off")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("go list (%s): %v\n%s", goos, err, stderr.String())
	}
	var list []*listPkg
	dec := json.NewDecoder(&stdout)
	for {
		p := new(listPkg)
		if err := dec.Decode(p); err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("go list (%s): %v", goos, err)
		}
		list = append(list, p)
	}
	return list, nil
}

func relPath(mod, path string) string {
	if path == mod {
		return ""
	}
	return strings.TrimPrefix(path, mod+"/")
}

// isInternal reports whether a module-relative path has an "internal"
// element.
func isInternal(rel string) bool {
	for _, e := range strings.Split(rel, "/") {
		if e == "internal" {
			return true
		}
	}
	return false
}

// sortedPkgs returns the packages in import path order.
func (m *module) sortedPkgs() []*pkgInfo {
	out := make([]*pkgInfo, 0, len(m.pkgs))
	for _, p := range m.pkgs {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out
}

// platforms returns the loaded platforms of p in the module's GOOS order.
func (m *module) platforms(p *pkgInfo) []*platPkg {
	var out []*platPkg
	for _, g := range m.goos {
		if pp := p.plat[g]; pp != nil {
			out = append(out, pp)
		}
	}
	return out
}

// isAPIPkg reports whether path is a public package of the module, that is
// one that has (or would have) its own golden file.
func (m *module) isAPIPkg(path string) bool {
	if path != m.path && !strings.HasPrefix(path, m.path+"/") {
		return false
	}
	return !isInternal(relPath(m.path, path))
}

// isModuleInternal reports whether path is an internal package of the module.
func (m *module) isModuleInternal(path string) bool {
	if path != m.path && !strings.HasPrefix(path, m.path+"/") {
		return false
	}
	return isInternal(relPath(m.path, path))
}

// fileName is the golden/allowlist file name of a package.
func fileName(p *pkgInfo) string {
	if p.rel == "" {
		return p.name + ".txt"
	}
	return strings.ReplaceAll(p.rel, "/", "_") + ".txt"
}

// displayRel is a package path for messages: "." for the root package.
func displayRel(rel string) string {
	if rel == "" {
		return "."
	}
	return rel
}
