package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Tier names used in api/tiers.txt.
const (
	tierStable       = "stable"
	tierExperimental = "experimental"
	tierSplit        = "split" // stable allowlist in api/stable/<pkg>.txt, the rest experimental
	tierExclude      = "exclude"
)

// tierSet is the parsed api/tiers.txt. Each line is "<pattern> <tier>";
// a pattern is a module-relative package path ("." is the root package),
// optionally ending in "/..." to cover the package and everything below
// it. An exact pattern beats any "/..." pattern; among "/..." patterns the
// longest wins.
type tierSet struct {
	exact  map[string]string
	prefix map[string]string // "ui" for "ui/..."
}

func readTiers(path string) (*tierSet, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	ts := &tierSet{exact: map[string]string{}, prefix: map[string]string{}}
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 {
			return nil, fmt.Errorf("%s:%d: want \"<package pattern> <tier>\"", path, n)
		}
		pat, tier := fields[0], fields[1]
		switch tier {
		case tierStable, tierExperimental, tierSplit, tierExclude:
		default:
			return nil, fmt.Errorf("%s:%d: unknown tier %q (want stable, experimental, split or exclude)", path, n, tier)
		}
		m := ts.exact
		if strings.HasSuffix(pat, "/...") {
			pat = strings.TrimSuffix(pat, "/...")
			m = ts.prefix
			if tier == tierSplit {
				return nil, fmt.Errorf("%s:%d: split applies to single packages, not %s/...", path, n, pat)
			}
		}
		if pat == "." {
			pat = ""
		}
		if _, dup := m[pat]; dup {
			return nil, fmt.Errorf("%s:%d: duplicate pattern %s", path, n, fields[0])
		}
		m[pat] = tier
	}
	return ts, sc.Err()
}

// lookup returns the tier of a module-relative package path.
func (ts *tierSet) lookup(rel string) (string, bool) {
	if t, ok := ts.exact[rel]; ok {
		return t, true
	}
	for p := rel; ; {
		if t, ok := ts.prefix[p]; ok {
			return t, true
		}
		if p == "" {
			return "", false
		}
		i := strings.LastIndexByte(p, '/')
		if i < 0 {
			p = ""
		} else {
			p = p[:i]
		}
	}
}

// allowlist is api/stable/<pkg>.txt: one stable symbol per line, either a
// top-level name ("Button", covering all fields and methods of a type) or
// "Type.Member" (one field or method).
type allowlist struct {
	file    string
	entries []allowEntry
	names   map[string]bool
	members map[string]bool
}

type allowEntry struct {
	line int
	text string
}

func (a *allowlist) hasName(name string) bool { return a != nil && a.names[name] }

// covers reports whether a symbol (top-level name and optional member) is
// stable. A type with at least one stable member counts as stable itself
// for whole-type facts such as comparability.
func (a *allowlist) covers(top, member string) bool {
	if a == nil {
		return false
	}
	if a.names[top] {
		return true
	}
	if member != "" {
		return a.members[top+"."+member]
	}
	for k := range a.members {
		if strings.HasPrefix(k, top+".") {
			return true
		}
	}
	return false
}

func readAllowlist(path string) (*allowlist, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	a := &allowlist{file: path, names: map[string]bool{}, members: map[string]bool{}}
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		a.entries = append(a.entries, allowEntry{n, line})
		if strings.Contains(line, ".") {
			a.members[line] = true
		} else {
			a.names[line] = true
		}
	}
	return a, sc.Err()
}

// allowlists holds the allowlists by package path plus the problems found
// while reading them (missing or orphaned files).
type allowlists struct {
	byPkg    map[string]*allowlist
	problems []string
}

// readAllowlists reads api/stable/*.txt for the split packages.
func readAllowlists(root string, m *module, tiers *tierSet) (*allowlists, error) {
	dir := filepath.Join(root, "api", "stable")
	res := &allowlists{byPkg: map[string]*allowlist{}}
	want := map[string]bool{}
	for _, p := range m.sortedPkgs() {
		if t, _ := tiers.lookup(p.rel); t != tierSplit {
			continue
		}
		name := fileName(p)
		want[name] = true
		a, err := readAllowlist(filepath.Join(dir, name))
		if os.IsNotExist(err) {
			res.problems = append(res.problems, fmt.Sprintf("%s is split but api/stable/%s does not exist (create it, empty is fine)", displayRel(p.rel), name))
			continue
		} else if err != nil {
			return nil, err
		}
		res.byPkg[p.path] = a
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.txt"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	for _, f := range files {
		if !want[filepath.Base(f)] {
			res.problems = append(res.problems, fmt.Sprintf("api/stable/%s does not belong to a split package in api/tiers.txt", filepath.Base(f)))
		}
	}
	return res, nil
}
