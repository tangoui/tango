package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// API tiers as defined by api/tiers.txt and api/stable/<pkg>.txt
// (docs/release.md, "API 稳定性"). The rules mirror internal/cmd/apitxt:
// an exact pattern beats any "x/..." pattern, the longest "x/..." wins, and
// an allowlist line is a top-level name (a type covers all its members) or
// "Type.Member".
const (
	tierStable       = "stable"
	tierExperimental = "experimental"
	tierSplit        = "split"
	tierExclude      = "exclude"
)

type tierSet struct {
	exact  map[string]string
	prefix map[string]string
}

// readTiers parses api/tiers.txt. A missing file yields an empty set, in
// which every package counts as stable (no badges).
func readTiers(root string) (*tierSet, error) {
	ts := &tierSet{exact: map[string]string{}, prefix: map[string]string{}}
	f, err := os.Open(filepath.Join(root, "api", "tiers.txt"))
	if os.IsNotExist(err) {
		return ts, nil
	} else if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line, _, _ := strings.Cut(sc.Text(), "#")
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 {
			return nil, fmt.Errorf("api/tiers.txt:%d: want \"<package pattern> <tier>\"", n)
		}
		pat, m := fields[0], ts.exact
		if strings.HasSuffix(pat, "/...") {
			pat, m = strings.TrimSuffix(pat, "/..."), ts.prefix
		}
		if pat == "." {
			pat = ""
		}
		m[pat] = fields[1]
	}
	return ts, sc.Err()
}

// lookup returns the tier of a module-relative package path ("" or "." is
// the root package); unclassified packages count as stable.
func (ts *tierSet) lookup(rel string) string {
	if rel == "." {
		rel = ""
	}
	if t, ok := ts.exact[rel]; ok {
		return t
	}
	for p := rel; ; {
		if t, ok := ts.prefix[p]; ok {
			return t
		}
		if p == "" {
			return tierStable
		}
		if i := strings.LastIndexByte(p, '/'); i >= 0 {
			p = p[:i]
		} else {
			p = ""
		}
	}
}

// allowlist is the stable-symbol list of a split package.
type allowlist struct {
	names   map[string]bool // whole top-level symbols
	members map[string]bool // "Type.Member"
}

func readAllowlist(root, rel string) (*allowlist, error) {
	a := &allowlist{names: map[string]bool{}, members: map[string]bool{}}
	name := strings.ReplaceAll(rel, "/", "_") + ".txt"
	f, err := os.Open(filepath.Join(root, "api", "stable", name))
	if os.IsNotExist(err) {
		return a, nil
	} else if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line, _, _ := strings.Cut(sc.Text(), "#")
		line = strings.TrimSpace(line)
		switch {
		case line == "":
		case strings.Contains(line, "."):
			a.members[line] = true
		default:
			a.names[line] = true
		}
	}
	return a, sc.Err()
}

// stability answers per-symbol questions for one package page. A nil
// allowlist means the whole package has one tier.
type stability struct {
	tier  string
	allow *allowlist
}

// experimental reports whether a top-level symbol is experimental.
func (s stability) experimental(name string) bool {
	switch s.tier {
	case tierExperimental:
		return true
	case tierSplit:
		return !s.allow.names[name] && !s.partial(name)
	}
	return false
}

// partial reports whether only some members of type name are stable.
func (s stability) partial(name string) bool {
	if s.tier != tierSplit || s.allow.names[name] {
		return false
	}
	for k := range s.allow.members {
		if strings.HasPrefix(k, name+".") {
			return true
		}
	}
	return false
}

// memberExperimental reports whether member of type typ is experimental.
func (s stability) memberExperimental(typ, member string) bool {
	switch s.tier {
	case tierExperimental:
		return true
	case tierSplit:
		return !s.allow.names[typ] && !s.allow.members[typ+"."+member]
	}
	return false
}

// badge is the inline marker of experimental symbols. It is plain HTML
// with the classes of VitePress's <Badge> because the generated pages are
// wrapped in v-pre, where Vue components do not render.
const badge = `<span class="VPBadge warning" title="Experimental: may change in any 0.x release">实验性</span>`
