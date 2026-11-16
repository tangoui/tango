package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type benchResult struct {
	Name, Scene   string
	NsOp          float64
	AllocsOp, BOp float64
	Dirty         float64
	ColdNS        float64
	Budget        float64
	Pass          bool
}
type budgetResult struct {
	Key        string
	NS, Budget float64
	Status     string
}

var benchRE = regexp.MustCompile(`^(?:Benchmark[^/]+/)([^/]+)/([^\s-]+)(?:-\d+)?\s+.*?\s([0-9]+(?:\.[0-9]+)?)\s+ns/op(?:\s+([0-9]+(?:\.[0-9]+)?)\s+dirty/op)?(?:\s+([0-9]+(?:\.[0-9]+)?)\s+B/op)?(?:\s+([0-9]+(?:\.[0-9]+)?)\s+allocs/op)?\s*$`)

func parseBenchLine(s string) (benchResult, error) {
	m := benchRE.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return benchResult{}, fmt.Errorf("not a benchmark line: %q", s)
	}
	vals := make([]float64, 4)
	for i := 3; i < 7; i++ {
		if m[i] != "" {
			v, e := strconv.ParseFloat(m[i], 64)
			if e != nil {
				return benchResult{}, e
			}
			vals[i-3] = v
		}
	}
	return benchResult{Name: m[1], Scene: m[2], NsOp: vals[0], Dirty: vals[1], BOp: vals[2], AllocsOp: vals[3]}, nil
}
func parseBudgetLine(s string) (budgetResult, error) {
	f := strings.Fields(s)
	if len(f) != 5 || f[0] != "PERF" {
		return budgetResult{}, fmt.Errorf("invalid budget line: %q", s)
	}
	ns, e := strconv.ParseFloat(f[2], 64)
	if e != nil {
		return budgetResult{}, e
	}
	b, e := strconv.ParseFloat(f[3], 64)
	if e != nil {
		return budgetResult{}, e
	}
	return budgetResult{Key: f[1], NS: ns, Budget: b, Status: f[4]}, nil
}
func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	x := append([]float64(nil), v...)
	for i := 1; i < len(x); i++ {
		for j := i; j > 0 && x[j] < x[j-1]; j-- {
			x[j], x[j-1] = x[j-1], x[j]
		}
	}
	return x[len(x)/2]
}
func iqr(v []float64) float64 {
	if len(v) < 2 {
		return 0
	}
	x := append([]float64(nil), v...)
	for i := 1; i < len(x); i++ {
		for j := i; j > 0 && x[j] < x[j-1]; j-- {
			x[j], x[j-1] = x[j-1], x[j]
		}
	}
	return x[(3*len(x))/4] - x[len(x)/4]
}
