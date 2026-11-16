package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type abMetric struct{ NS, Allocs float64 }
type namedABMetric struct {
	Name   string
	Metric abMetric
}

func abVerdict(base, head []abMetric) string {
	if len(base) == 0 || len(head) == 0 {
		return "noise"
	}
	bn, hn := medianAB(base, 0), medianAB(head, 0)
	if bn <= 0 {
		return "noise"
	}
	ns := 1 - hn/bn
	ba, ha := medianAB(base, 1), medianAB(head, 1)
	alloc, comparable := 0.0, false
	switch {
	case ba > 0:
		alloc, comparable = 1-ha/ba, true
	case ha > 0:
		alloc, comparable = -1, true
	}
	keep := ns >= .10 || (comparable && alloc >= .20)
	regress := ns < -.05 || (comparable && alloc < -.05)
	if keep && !regress {
		return "keep"
	}
	if regress {
		return "regress"
	}
	return "noise"
}

func runAB(baseRev, bench, pkg string, rounds int, out *strings.Builder) (err error) {
	if !regexp.MustCompile(`^[[:graph:]]+$`).MatchString(bench) {
		return fmt.Errorf("invalid benchmark regex")
	}
	if rounds < 1 {
		return fmt.Errorf("rounds must be positive")
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	parent, err := os.MkdirTemp("", "uiperf-base-")
	if err != nil {
		return err
	}
	base := filepath.Join(parent, "tree")
	owner := &worktreeOwner{root: root, parent: parent, path: base}
	defer func() {
		if ce := owner.cleanup(); ce != nil {
			err = errors.Join(err, ce)
		}
	}()
	owner.addAttempted = true
	if err = runCmd(root, "git", "worktree", "add", "--detach", base, baseRev); err != nil {
		return err
	}
	owner.registered = true
	baseBin, headBin := filepath.Join(parent, "base.test"), filepath.Join(parent, "head.test")
	if err = runCmd(root, "go", "test", "-c", "-o", headBin, pkg); err != nil {
		return err
	}
	if err = runCmd(base, "go", "test", "-c", "-o", baseBin, pkg); err != nil {
		return err
	}
	baseRows, headRows := map[string][]abMetric{}, map[string][]abMetric{}
	for r := 0; r < rounds; r++ {
		if err = loadGate(systemLoad, time.Sleep, 6, 10*time.Minute); err != nil {
			return fmt.Errorf("round %d invalidated by load: %w", r+1, err)
		}
		order := [][2]string{{baseBin, "base"}, {headBin, "head"}}
		if r%2 == 1 {
			order = [][2]string{{headBin, "head"}, {baseBin, "base"}}
		}
		for _, x := range order {
			cmd := exec.Command(x[0], "-test.run", "^$", "-test.bench", bench, "-test.benchmem", "-test.count", "1")
			o, ce := cmd.Output()
			if ce != nil {
				return ce
			}
			rows, pe := parseABOutput(string(o))
			if pe != nil {
				return pe
			}
			dst := baseRows
			if x[1] == "head" {
				dst = headRows
			}
			for _, row := range rows {
				dst[row.Name] = append(dst[row.Name], row.Metric)
			}
		}
	}
	if len(baseRows) == 0 || len(headRows) == 0 {
		return fmt.Errorf("benchmark produced no matching rows")
	}
	nameSet := map[string]bool{}
	for name := range baseRows {
		nameSet[name] = true
	}
	for name := range headRows {
		nameSet[name] = true
	}
	names := make([]string, 0, len(nameSet))
	for name := range nameSet {
		names = append(names, name)
	}
	sortStrings(names)
	for _, name := range names {
		b, bok := baseRows[name]
		h, hok := headRows[name]
		if !bok || !hok {
			fmt.Fprintf(out, "%s: missing from %s; verdict: noise\n", name, map[bool]string{true: "base", false: "head"}[!bok])
			continue
		}
		fmt.Fprintf(out, "%s: base median ns/op %.0f, head %.0f; base allocs/op %.1f, head %.1f; verdict: %s\n", name, medianAB(b, 0), medianAB(h, 0), medianAB(b, 1), medianAB(h, 1), abVerdict(b, h))
	}
	return nil
}
func sortStrings(v []string) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}
func medianAB(m []abMetric, k int) float64 {
	v := make([]float64, len(m))
	for i, x := range m {
		if k == 0 {
			v[i] = x.NS
		} else {
			v[i] = x.Allocs
		}
	}
	return median(v)
}

var abRE = regexp.MustCompile(`(?m)^([^\s]+)\s+\d+\s+([0-9.]+)\s+ns/op(?:\s+[0-9.]+\s+B/op\s+([0-9.]+)\s+allocs/op)?`)

func parseABOutput(s string) ([]namedABMetric, error) {
	ms := abRE.FindAllStringSubmatch(s, -1)
	if len(ms) == 0 {
		return nil, fmt.Errorf("no benchmark output")
	}
	out := make([]namedABMetric, len(ms))
	for i, m := range ms {
		var n, a float64
		fmt.Sscanf(m[2], "%f", &n)
		if m[3] != "" {
			fmt.Sscanf(m[3], "%f", &a)
		}
		name := m[1]
		if suffix := benchmarkIterationRE.FindString(name); suffix != "" {
			name = name[:len(name)-len(suffix)]
		}
		out[i] = namedABMetric{Name: name, Metric: abMetric{NS: n, Allocs: a}}
	}
	return out, nil
}

var runGit = func(root string, args ...string) ([]byte, error) {
	return exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
}

type worktreeOwner struct {
	root, parent, path       string
	addAttempted, registered bool
}

func (o *worktreeOwner) validate() error {
	parent, err := filepath.Abs(o.parent)
	if err != nil {
		return err
	}
	path, err := filepath.Abs(o.path)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(parent, path)
	if err != nil || rel != "tree" {
		return fmt.Errorf("worktree path is not the owned tree: %s", o.path)
	}
	if info, err := os.Lstat(parent); err != nil {
		return fmt.Errorf("owned parent unavailable: %w", err)
	} else if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refuse symlink owned parent: %s", parent)
	}
	tempRoot, err := filepath.Abs(os.TempDir())
	if err != nil {
		return fmt.Errorf("resolve temporary root: %w", err)
	}
	parentRel, err := filepath.Rel(tempRoot, parent)
	if err != nil || parentRel == ".." || strings.HasPrefix(parentRel, ".."+string(filepath.Separator)) || filepath.IsAbs(parentRel) {
		return fmt.Errorf("owned parent is outside temporary root: %s", parent)
	}
	component := tempRoot
	for _, part := range strings.Split(parentRel, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		component = filepath.Join(component, part)
		if info, err := os.Lstat(component); err != nil {
			return fmt.Errorf("inspect owned path component: %w", err)
		} else if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refuse symlink component in owned parent: %s", component)
		}
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refuse symlink worktree path: %s", path)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect worktree path: %w", err)
	}
	o.parent, o.path = parent, path
	return nil
}
func registrationPresent(root, path string) (bool, error) {
	out, err := runGit(root, "worktree", "list", "--porcelain")
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(out), "\n") {
		if line == "worktree "+path {
			return true, nil
		}
	}
	return false, nil
}
func (o *worktreeOwner) cleanup() error {
	if err := o.validate(); err != nil {
		return err
	}
	if !o.addAttempted {
		return os.Remove(o.parent)
	}
	if !o.registered {
		present, err := registrationPresent(o.root, o.path)
		if err != nil {
			return fmt.Errorf("verify partial worktree registration: %w", err)
		}
		if !present {
			return os.Remove(o.parent)
		}
		o.registered = true
	}
	removeOut, removeErr := runGit(o.root, "worktree", "remove", "--force", o.path)
	if removeErr != nil {
		return fmt.Errorf("remove worktree %s: %w (%s)", o.path, removeErr, strings.TrimSpace(string(removeOut)))
	}
	present, verifyErr := registrationPresent(o.root, o.path)
	if verifyErr != nil {
		return fmt.Errorf("verify worktree cleanup: %w", verifyErr)
	}
	if present {
		return fmt.Errorf("worktree registration remains: %s", o.path)
	}
	if err := os.RemoveAll(o.parent); err != nil {
		return fmt.Errorf("remove owned parent %s: %w", o.parent, err)
	}
	return nil
}

var runCmd = func(dir string, args ...string) error {
	c := exec.Command(args[0], args[1:]...)
	c.Dir = dir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
