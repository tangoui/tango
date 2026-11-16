package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type benchFile struct {
	Results []benchResult `json:"results"`
}

func writeBenchReport(dir string, r []benchResult) error {
	sort.Slice(r, func(i, j int) bool {
		if r[i].Name == r[j].Name {
			return r[i].Scene < r[j].Scene
		}
		return r[i].Name < r[j].Name
	})
	b, e := json.MarshalIndent(benchFile{r}, "", "  ")
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
	fmt.Fprintln(f, "# UI performance status\n\n| Component | Scene | ns/op | allocs/op | B/op | dirty/op | budget | pass | cold ns |")
	fmt.Fprintln(f, "|---|---:|---:|---:|---:|---:|---:|:---:|---:|")
	for _, x := range r {
		fmt.Fprintf(f, "| %s | %s | %.0f | %.0f | %.0f | %.4f | %.0f | %t | %.0f |\n", x.Name, x.Scene, x.NsOp, x.AllocsOp, x.BOp, x.Dirty, x.Budget, x.Pass, x.ColdNS)
	}
	return nil
}
