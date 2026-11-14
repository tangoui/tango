// Command apitxt writes and checks the API golden files of the module:
// one sorted text file per public package under api/go, one fact per line,
// plus the tier checks of docs/specs/api-stability.md §4.3.
//
//	go run ./internal/cmd/apitxt -w        rewrite api/go/*.txt
//	go run ./internal/cmd/apitxt -check    fail when api/go is out of date or a check fails
//	go run ./internal/cmd/apitxt -classify < apidiff.out
//	                                       tag apidiff lines as stable/experimental (scripts/apidiff.sh)
//
// The API is loaded from compiler export data (go list -export) for each
// of linux, darwin and windows with CGO_ENABLED=0 and merged; lines that
// only exist on some platforms carry a " [darwin]"-style suffix. Only the
// standard library is used, like site/tools/apigen.
//
// Files are named after the package path relative to the module with "/"
// replaced by "_" (ui/theme -> api/go/ui_theme.txt); the root package uses
// its package name (api/go/tango.txt). Tiers come from api/tiers.txt and the
// stable-symbol allowlists of "split" packages from api/stable/<same name>.txt.
// See docs/release.md ("API 稳定性").
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	write := flag.Bool("w", false, "rewrite the golden files")
	check := flag.Bool("check", false, "exit 1 when the golden files are out of date or a check fails")
	classify := flag.Bool("classify", false, "read apidiff output on stdin and prefix each change with its tier")
	strict := flag.Bool("strict", false, "treat warnings (missing Experimental doc, cross-tier leaks) as failures")
	root := flag.String("C", ".", "module root directory")
	goosList := flag.String("goos", "linux,darwin,windows", "comma-separated platforms to merge")
	verbose := flag.Bool("v", false, "list every warning instead of a summary")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: apitxt -w | -check | -classify [-C dir] [-goos list] [-strict] [-v]")
		flag.PrintDefaults()
	}
	flag.Parse()
	n := 0
	for _, b := range []bool{*write, *check, *classify} {
		if b {
			n++
		}
	}
	if n != 1 || flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}
	cfg := config{
		root:    *root,
		goos:    strings.Split(*goosList, ","),
		strict:  *strict,
		verbose: *verbose,
		out:     os.Stdout,
		errOut:  os.Stderr,
	}
	var code int
	switch {
	case *classify:
		code = runClassify(cfg, os.Stdin)
	case *write:
		code = run(cfg, true)
	default:
		code = run(cfg, false)
	}
	os.Exit(code)
}

type config struct {
	root    string
	goos    []string
	strict  bool
	verbose bool
	out     io.Writer
	errOut  io.Writer
}

// run generates the golden files and runs the checks. With write it
// rewrites api/go, otherwise it compares against it. It returns the exit
// status: 0 ok, 1 drift or a failed check, 2 setup error.
func run(cfg config, write bool) int {
	tiers, err := readTiers(filepath.Join(cfg.root, "api", "tiers.txt"))
	if err != nil {
		fmt.Fprintln(cfg.errOut, "apitxt:", err)
		return 2
	}
	m, err := load(cfg.root, cfg.goos)
	if err != nil {
		fmt.Fprintln(cfg.errOut, "apitxt:", err)
		return 2
	}
	allow, err := readAllowlists(cfg.root, m, tiers)
	if err != nil {
		fmt.Fprintln(cfg.errOut, "apitxt:", err)
		return 2
	}

	files, err := generate(m, tiers)
	if err != nil {
		fmt.Fprintln(cfg.errOut, "apitxt:", err)
		return 2
	}

	failed := false
	dir := filepath.Join(cfg.root, "api", "go")
	if write {
		if err := writeFiles(dir, files); err != nil {
			fmt.Fprintln(cfg.errOut, "apitxt:", err)
			return 2
		}
	} else {
		drift, err := compareFiles(dir, files, cfg.errOut)
		if err != nil {
			fmt.Fprintln(cfg.errOut, "apitxt:", err)
			return 2
		}
		if drift {
			fmt.Fprintln(cfg.errOut, "apitxt: api/go is out of date; run: go run ./internal/cmd/apitxt -w")
			failed = true
		}
	}

	res := runChecks(m, tiers, allow)
	if res.report(cfg) {
		failed = true
	}
	if failed {
		return 1
	}
	if !write {
		fmt.Fprintf(cfg.out, "apitxt: %d golden files up to date\n", len(files))
	}
	return 0
}

// writeFiles replaces the *.txt files of dir with files.
func writeFiles(dir string, files map[string][]string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	old, err := filepath.Glob(filepath.Join(dir, "*.txt"))
	if err != nil {
		return err
	}
	for _, f := range old {
		if _, ok := files[filepath.Base(f)]; !ok {
			if err := os.Remove(f); err != nil {
				return err
			}
		}
	}
	for name, lines := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(joinLines(lines)), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// compareFiles reports whether dir differs from files, printing a short
// line diff (the files are sorted sets of lines, so a set difference is
// exact).
func compareFiles(dir string, files map[string][]string, w io.Writer) (bool, error) {
	onDisk := map[string][]string{}
	paths, err := filepath.Glob(filepath.Join(dir, "*.txt"))
	if err != nil {
		return false, err
	}
	for _, p := range paths {
		lines, err := readLines(p)
		if err != nil {
			return false, err
		}
		onDisk[filepath.Base(p)] = lines
	}
	names := map[string]bool{}
	for n := range files {
		names[n] = true
	}
	for n := range onDisk {
		names[n] = true
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)

	const maxShown = 40
	shown := 0
	drift := false
	for _, n := range sorted {
		want, have := files[n], onDisk[n]
		_, inWant := files[n]
		_, inHave := onDisk[n]
		switch {
		case !inHave:
			fmt.Fprintf(w, "api/go/%s: missing (%d lines)\n", n, len(want))
			drift = true
			continue
		case !inWant:
			fmt.Fprintf(w, "api/go/%s: stale file (package gone or excluded)\n", n)
			drift = true
			continue
		}
		added, removed := setDiff(want, have), setDiff(have, want)
		if len(added) == 0 && len(removed) == 0 {
			continue
		}
		drift = true
		fmt.Fprintf(w, "api/go/%s: +%d -%d lines\n", n, len(added), len(removed))
		for _, l := range removed {
			if shown < maxShown {
				fmt.Fprintln(w, "  -", l)
			}
			shown++
		}
		for _, l := range added {
			if shown < maxShown {
				fmt.Fprintln(w, "  +", l)
			}
			shown++
		}
	}
	if shown > maxShown {
		fmt.Fprintf(w, "  ... %d more changed lines\n", shown-maxShown)
	}
	return drift, nil
}

// setDiff returns the lines of a that are not in b.
func setDiff(a, b []string) []string {
	in := make(map[string]bool, len(b))
	for _, l := range b {
		in[l] = true
	}
	var out []string
	for _, l := range a {
		if !in[l] {
			out = append(out, l)
		}
	}
	return out
}

// readLines returns the non-empty lines of a file.
func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if l := sc.Text(); l != "" {
			lines = append(lines, l)
		}
	}
	return lines, sc.Err()
}
