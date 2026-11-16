package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type benchFile struct {
	Results []benchResult `json:"results"`
	Budget  budgetRun     `json:"budget"`
}

type budgetRun struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	Output string `json:"output,omitempty"`
}

func writeBenchReport(dir string, r []benchResult, budget budgetRun) error {
	sort.Slice(r, func(i, j int) bool {
		if r[i].Name == r[j].Name {
			return r[i].Scene < r[j].Scene
		}
		return r[i].Name < r[j].Name
	})
	b, e := json.MarshalIndent(benchFile{Results: r, Budget: budget}, "", "  ")
	if e == nil {
		e = os.WriteFile(filepath.Join(dir, "bench.json"), b, 0644)
	}
	if e != nil {
		return e
	}
	f, e := os.Create(filepath.Join(dir, "status.md"))
	if e != nil {
		return e
	}
	defer f.Close()
	fmt.Fprintf(f, "# UI performance status\n\nBudget run: %s", budget.Status)
	if budget.Error != "" {
		fmt.Fprintf(f, " (%s; full output in bench.json `budget.output`)", budget.Error)
	}
	fmt.Fprintln(f, "\n\n| Component | Scene | ns/op | allocs/op | B/op | dirty/op | budget | pass | cold ns | cold status |")
	fmt.Fprintln(f, "|---|---:|---:|---:|---:|---:|---:|:---:|---:|---|")
	for _, x := range r {
		fmt.Fprintf(f, "| %s | %s | %.0f | %.0f | %.0f | %.4f | %.0f | %t | %s | %s |\n", x.Name, x.Scene, x.NsOp, x.AllocsOp, x.BOp, x.Dirty, x.Budget, x.Pass, coldCell(x), coldStatusCell(x))
	}
	return nil
}

func coldCell(x benchResult) string {
	if x.ColdStatus != coldOK {
		return "—"
	}
	return fmt.Sprintf("%.0f", x.ColdNS)
}

func coldStatusCell(x benchResult) string {
	if x.ColdError == "" {
		return x.ColdStatus
	}
	msg := []rune(strings.Join(strings.Fields(x.ColdError), " "))
	if len(msg) > 120 {
		msg = append(msg[:120], '…')
	}
	return x.ColdStatus + ": " + strings.ReplaceAll(string(msg), "|", "\\|")
}
