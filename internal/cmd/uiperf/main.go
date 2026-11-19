package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var e error
	switch os.Args[1] {
	case "bench":
		e = benchCLI(os.Args[2:])
	case "hot":
		e = hotCLI(os.Args[2:])
	case "ab":
		e = abCLI(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if e != nil {
		fmt.Fprintln(os.Stderr, "uiperf:", e)
		os.Exit(1)
	}
}
func usage() { fmt.Fprintln(os.Stderr, "usage: uiperf bench|hot|ab") }
func benchCLI(a []string) error {
	f := flag.NewFlagSet("bench", flag.ContinueOnError)
	n := f.Int("count", 3, "")
	r := f.String("run", ".*", "")
	o := f.String("out", "", "")
	if e := f.Parse(a); e != nil {
		return e
	}
	if *o == "" {
		return fmt.Errorf("-out is required")
	}
	return runBench(*n, *r, *o)
}
func hotCLI(a []string) error {
	f := flag.NewFlagSet("hot", flag.ContinueOnError)
	top := f.Int("top", defaultHotNodeCount, "pprof -nodecount per profile")
	o := f.String("out", "", "")
	reuse := f.Bool("reuse-profiles", false, "score existing <out>/prof/*.pprof; do not re-run benchmarks")
	md := f.String("md", "hot.md", "report file name inside -out")
	if e := f.Parse(a); e != nil {
		return e
	}
	if *o == "" {
		return fmt.Errorf("-out is required")
	}
	return runHot(*o, hotOptions{NodeCount: *top, ReuseProfiles: *reuse, Output: *md})
}
func abCLI(a []string) error {
	f := flag.NewFlagSet("ab", flag.ContinueOnError)
	base := f.String("base", "", "")
	bench := f.String("bench", "", "")
	rounds := f.Int("rounds", 5, "")
	pkg := f.String("pkg", "./ui", "")
	if e := f.Parse(a); e != nil {
		return e
	}
	if *base == "" || *bench == "" {
		return fmt.Errorf("-base and -bench are required")
	}
	var b strings.Builder
	if e := runAB(*base, *bench, *pkg, *rounds, &b); e != nil {
		return e
	}
	fmt.Print(b.String())
	return nil
}
