package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// runClassify reads `apidiff -incompatible` output lines ("- ./ui.Tooltip:
// changed ...") and writes each one prefixed with its tier and a tab:
// "stable" for a break in a stable package or of an allowlisted symbol of a
// split package, "experimental" otherwise. Other input lines are dropped.
// It needs only api/tiers.txt and api/stable, not the module's export data.
func runClassify(cfg config, in io.Reader) int {
	tiers, err := readTiers(filepath.Join(cfg.root, "api", "tiers.txt"))
	if err != nil {
		fmt.Fprintln(cfg.errOut, "apitxt:", err)
		return 2
	}
	mod, err := modulePath(cfg.root)
	if err != nil {
		fmt.Fprintln(cfg.errOut, "apitxt:", err)
		return 2
	}
	allow := map[string]*allowlist{}
	for rel, t := range tiers.exact {
		if t != tierSplit || rel == "" {
			continue
		}
		name := strings.ReplaceAll(rel, "/", "_") + ".txt"
		a, err := readAllowlist(filepath.Join(cfg.root, "api", "stable", name))
		if err != nil && !os.IsNotExist(err) {
			fmt.Fprintln(cfg.errOut, "apitxt:", err)
			return 2
		}
		allow[rel] = a // nil when missing: nothing stable
	}
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		rel, top, member := parseAPIDiffSymbol(mod, line)
		tier := tierStable
		switch t, ok := tiers.lookup(rel); {
		case !ok, t == tierStable:
			// Unclassified (for example a removed package) counts as stable: fail safe.
		case t == tierSplit:
			a := allow[rel]
			if top == "" {
				if a == nil || len(a.entries) == 0 {
					tier = tierExperimental
				}
			} else if !a.covers(top, member) {
				tier = tierExperimental
			}
		default:
			tier = tierExperimental
		}
		fmt.Fprintf(cfg.out, "%s\t%s\n", tier, line)
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(cfg.errOut, "apitxt:", err)
		return 2
	}
	return 0
}

// parseAPIDiffSymbol splits the subject of an apidiff line into the
// module-relative package and the symbol. apidiff writes root package
// symbols without a prefix, other packages with "./<rel>.", methods as
// "(*T).M" or "T.M", and package removals as "package <path>: removed":
//
//	"- ./ui.(*Label).Color: ..."   -> ("ui", "Label", "Color")
//	"- Options.Title: ..."         -> ("", "Options", "Title")
//	"- package <mod>/ui/foo: ..."  -> ("ui/foo", "", "")
func parseAPIDiffSymbol(mod, line string) (rel, top, member string) {
	s := strings.TrimPrefix(line, "- ")
	if i := strings.Index(s, ": "); i >= 0 {
		s = s[:i]
	}
	if p, ok := strings.CutPrefix(s, "package "); ok {
		return relPath(mod, p), "", ""
	}
	sym := s
	if r, ok := strings.CutPrefix(s, "./"); ok {
		slash := strings.LastIndexByte(r, '/')
		dot := strings.IndexByte(r[slash+1:], '.')
		if dot < 0 {
			return r, "", ""
		}
		rel, sym = r[:slash+1+dot], r[slash+1+dot+1:]
	}
	if strings.HasPrefix(sym, "(") {
		if end := strings.IndexByte(sym, ')'); end >= 0 {
			top, member = strings.TrimPrefix(sym[1:end], "*"), strings.TrimPrefix(sym[end+1:], ".")
		}
	} else if i := strings.IndexByte(sym, '['); i >= 0 {
		// T[P].M: a value-receiver method of a generic type
		top = sym[:i]
		if j := strings.Index(sym, "]."); j >= 0 {
			member = sym[j+2:]
		}
	} else {
		top, member, _ = strings.Cut(sym, ".")
	}
	if i := strings.IndexByte(top, '['); i >= 0 {
		top = top[:i]
	}
	return rel, top, member
}

// modulePath reads the module path from root/go.mod.
func modulePath(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	for _, l := range strings.Split(string(data), "\n") {
		if p, ok := strings.CutPrefix(strings.TrimSpace(l), "module "); ok {
			return strings.Trim(strings.TrimSpace(p), `"`), nil
		}
	}
	return "", fmt.Errorf("%s/go.mod has no module line", root)
}
