package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type abMetric struct{ NS, Allocs float64 }

func abVerdict(base, head []abMetric) string {
	if len(base) == 0 || len(head) == 0 {
		return "noise"
	}
	bn, hn, ba, ha := 0.0, 0.0, 0.0, 0.0
	for _, x := range base {
		bn += x.NS
		ba += x.Allocs
	}
	for _, x := range head {
		hn += x.NS
		ha += x.Allocs
	}
	bn /= float64(len(base))
	hn /= float64(len(head))
	ba /= float64(len(base))
	ha /= float64(len(head))
	ns := 1 - hn/bn
	alloc := 1 - ha/ba
	if ns >= .10 || alloc >= .20 {
		if ns < -.05 || alloc < -.05 {
			return "regress"
		}
		return "keep"
	}
	if ns < -.05 || alloc < -.05 {
		return "regress"
	}
	return "noise"
}
func runAB(baseRev, bench, pkg string, rounds int, out *strings.Builder) error {
	if !regexp.MustCompile(`^[[:graph:]]+$`).MatchString(bench) {
		return fmt.Errorf("invalid benchmark regex")
	}
	if rounds < 1 {
		return fmt.Errorf("rounds must be positive")
	}
	root, e := os.Getwd()
	if e != nil {
		return e
	}
	parent, e := os.MkdirTemp("", "uiperf-base-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(parent)
	base := filepath.Join(parent, "tree")
	made := false
	defer func() {
		if made {
			_ = exec.Command("git", "worktree", "remove", "--force", base).Run()
		}
	}()
	if e = runCmd(root, "git", "worktree", "add", "--detach", base, baseRev); e != nil {
		return e
	}
	made = true
	baseBin := filepath.Join(parent, "base.test")
	headBin := filepath.Join(parent, "head.test")
	if e = runCmd(root, "go", "test", "-c", "-o", headBin, pkg); e != nil {
		return e
	}
	if e = runCmd(base, "go", "test", "-c", "-o", baseBin, pkg); e != nil {
		return e
	}
	var bm, hm []abMetric
	for r := 0; r < rounds; r++ {
		if e = loadGate(systemLoad, time.Sleep, 6, 10*60e9); e != nil {
			return fmt.Errorf("round %d invalidated by load: %w", r+1, e)
		}
		order := [][2]string{{baseBin, "base"}, {headBin, "head"}}
		if r%2 == 1 {
			order = order[1:]
			order = append(order, [2]string{baseBin, "base"})
		}
		for _, x := range order {
			cmd := exec.Command(x[0], "-test.run", "^$", "-test.bench", bench, "-test.benchmem", "-test.count", "1")
			o, e := cmd.Output()
			if e != nil {
				return e
			}
			ms, e := parseABOutput(string(o))
			if e != nil {
				return e
			}
			if x[1] == "base" {
				bm = append(bm, ms...)
			} else {
				hm = append(hm, ms...)
			}
		}
	}
	fmt.Fprintf(out, "base median ns/op %.0f, head %.0f; base allocs/op %.1f, head %.1f\nverdict: %s\n", medianAB(bm, 0), medianAB(hm, 0), medianAB(bm, 1), medianAB(hm, 1), abVerdict(bm, hm))
	return nil
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

var abRE = regexp.MustCompile(`(?m)^Benchmark\S+\s+\d+\s+([0-9.]+)\s+ns/op(?:\s+[0-9.]+\s+B/op\s+([0-9.]+)\s+allocs/op)?`)

func parseABOutput(s string) ([]abMetric, error) {
	ms := abRE.FindAllStringSubmatch(s, -1)
	if len(ms) == 0 {
		return nil, fmt.Errorf("no benchmark output")
	}
	out := make([]abMetric, len(ms))
	for i, m := range ms {
		fmt.Sscanf(m[1], "%f", &out[i].NS)
		fmt.Sscanf(m[2], "%f", &out[i].Allocs)
	}
	return out, nil
}
func runCmd(dir string, args ...string) error {
	c := exec.Command(args[0], args[1:]...)
	c.Dir = dir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
