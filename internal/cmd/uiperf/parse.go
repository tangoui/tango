package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type benchResult struct {
	Name, Scene   string
	NsOp          float64
	AllocsOp, BOp float64
	Dirty         float64
	ColdNS        float64
	ColdStatus    string
	ColdError     string `json:",omitempty"`
	Budget        float64
	Pass          bool
	NA            string `json:",omitempty"` // 不适用: the reason (no budget; Pass is false, check NA first)
}
type budgetResult struct {
	Key        string
	NS, Budget float64
	Status     string
	Reason     string // status "n/a": why the scene has no budget
}

var benchmarkNameRE = regexp.MustCompile(`^BenchmarkComponents/(.+)/(.+)$`)
var benchmarkIterationRE = regexp.MustCompile(`-(\d+)$`)
var metricUnitRE = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?)$`)

func parseBenchLine(s string) (benchResult, error) {
	fields := strings.Fields(strings.TrimSpace(s))
	if len(fields) < 3 || !strings.HasPrefix(fields[0], "BenchmarkComponents/") {
		return benchResult{}, fmt.Errorf("not a benchmark line: %q", s)
	}
	name := strings.TrimSuffix(fields[0], "-"+strings.TrimSuffix(benchmarkIterationRE.FindString(fields[0]), ""))
	// The suffix is an iteration count on the complete benchmark name. Do not
	// trim any hyphens in the component or scene itself.
	if m := benchmarkIterationRE.FindStringSubmatch(fields[0]); m != nil {
		name = fields[0][:len(fields[0])-len(m[0])]
	}
	parts := benchmarkNameRE.FindStringSubmatch(name)
	if parts == nil || parts[1] == "" || parts[2] == "" {
		return benchResult{}, fmt.Errorf("invalid benchmark name: %q", fields[0])
	}
	values := map[string]float64{}
	for i := 1; i+1 < len(fields); i++ {
		unit := fields[i+1]
		if unit != "ns/op" && unit != "dirty/op" && unit != "B/op" && unit != "allocs/op" {
			continue
		}
		if _, exists := values[unit]; exists {
			return benchResult{}, fmt.Errorf("duplicate metric %s: %q", unit, s)
		}
		if !metricUnitRE.MatchString(fields[i]) {
			return benchResult{}, fmt.Errorf("invalid metric value %q: %q", fields[i], s)
		}
		v, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return benchResult{}, err
		}
		values[unit] = v
	}
	for _, unit := range []string{"ns/op", "dirty/op", "B/op", "allocs/op"} {
		if _, ok := values[unit]; !ok {
			return benchResult{}, fmt.Errorf("missing metric %s: %q", unit, s)
		}
	}
	return benchResult{Name: parts[1], Scene: parts[2], NsOp: values["ns/op"], Dirty: values["dirty/op"], BOp: values["B/op"], AllocsOp: values["allocs/op"]}, nil
}

// parseBudgetLine parses "PERF <Name/Scene> <ns> <budget ns> <ok|over>",
// or "PERF <Name/Scene> 0 0 n/a <reason…>" for a scene marked not
// applicable (perfFixture.NA in ui).
func parseBudgetLine(s string) (budgetResult, error) {
	f := strings.Fields(s)
	na := len(f) > 5 && f[0] == "PERF" && f[4] == "n/a"
	if (len(f) != 5 && !na) || f[0] != "PERF" {
		return budgetResult{}, fmt.Errorf("invalid budget line: %q", s)
	}
	ns, err := strconv.ParseFloat(f[2], 64)
	if err != nil {
		return budgetResult{}, err
	}
	budget, err := strconv.ParseFloat(f[3], 64)
	if err != nil {
		return budgetResult{}, err
	}
	r := budgetResult{Key: f[1], NS: ns, Budget: budget, Status: f[4]}
	if na {
		r.Reason = strings.Join(f[5:], " ")
	}
	return r, nil
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	x := append([]float64(nil), v...)
	sort.Float64s(x)
	if len(x)%2 == 1 {
		return x[len(x)/2]
	}
	return (x[len(x)/2-1] + x[len(x)/2]) / 2
}
func iqr(v []float64) float64 {
	if len(v) < 2 {
		return 0
	}
	x := append([]float64(nil), v...)
	sort.Float64s(x)
	return x[(3*len(x))/4] - x[len(x)/4]
}
