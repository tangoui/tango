package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestABVerdict(t *testing.T) {
	cases := []struct {
		name       string
		base, head []abMetric
		want       string
	}{
		{"keep", []abMetric{{100, 10}}, []abMetric{{88, 10}}, "keep"},
		{"regress", []abMetric{{100, 10}}, []abMetric{{88, 10.6}}, "regress"},
		{"noise", []abMetric{{100, 10}}, []abMetric{{98, 10}}, "noise"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := abVerdict(tc.base, tc.head); got != tc.want {
				t.Fatalf("got %s", got)
			}
		})
	}
}

func TestParseABOutputGroupsNames(t *testing.T) {
	rows, err := parseABOutput("BenchmarkButton-8  1  100 ns/op  1 B/op  2 allocs/op\nBenchmarkTable-8  1  200 ns/op  2 B/op  3 allocs/op\n")
	if err != nil || len(rows) != 2 || rows[0].Name != "BenchmarkButton" || rows[1].Name != "BenchmarkTable" {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
}

func TestABVerdictZeroAllocations(t *testing.T) {
	if got := abVerdict([]abMetric{{100, 0}}, []abMetric{{80, 0}}); got != "keep" {
		t.Fatalf("zero/zero got %s", got)
	}
	for _, tc := range []struct {
		name       string
		base, head float64
		want       string
	}{
		{"zero-to-alloc", 0, 1, "regress"},
		{"alloc-to-zero", 1, 0, "keep"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := abVerdict([]abMetric{{100, tc.base}}, []abMetric{{100, tc.head}})
			if got != tc.want {
				t.Fatalf("got %s", got)
			}
		})
	}
}

func TestWorktreeOwnerRejectsOutsidePath(t *testing.T) {
	parent := t.TempDir()
	o := &worktreeOwner{parent: parent, path: filepath.Join(filepath.Dir(parent), "outside"), root: parent}
	if err := o.cleanup(); err == nil {
		t.Fatal("expected outside path error")
	}
}

func TestWorktreeOwnerAddFailureDoesNotPrune(t *testing.T) {
	parent := t.TempDir()
	old := runGit
	defer func() { runGit = old }()
	var calls []string
	runGit = func(_ string, args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(args, " "))
		return nil, nil
	}
	o := &worktreeOwner{root: parent, parent: parent, path: filepath.Join(parent, "tree"), addAttempted: true}
	if err := o.cleanup(); err != nil {
		t.Fatal(err)
	}
	for _, call := range calls {
		if strings.Contains(call, "prune") {
			t.Fatalf("unexpected global prune: %s", call)
		}
	}
	if _, err := os.Stat(parent); !os.IsNotExist(err) {
		t.Fatalf("parent remains: %v", err)
	}
}

func TestWorktreeOwnerRegistrationFailurePreservesParent(t *testing.T) {
	parent := t.TempDir()
	path := filepath.Join(parent, "tree")
	old := runGit
	defer func() { runGit = old }()
	calls := 0
	runGit = func(_ string, args ...string) ([]byte, error) {
		calls++
		if len(args) > 0 && args[0] == "worktree" && args[1] == "list" {
			return []byte("worktree " + path + "\n"), nil
		}
		return nil, errors.New("remove failed")
	}
	o := &worktreeOwner{root: parent, parent: parent, path: path, addAttempted: true}
	if err := o.cleanup(); err == nil {
		t.Fatal("expected cleanup error")
	}
	if _, err := os.Stat(parent); err != nil {
		t.Fatalf("parent should remain: %v", err)
	}
	if calls < 2 {
		t.Fatalf("expected registration and remove attempts, got %d", calls)
	}
}

func TestWorktreeOwnerPartialRegistrationCleanup(t *testing.T) {
	parent := t.TempDir()
	path := filepath.Join(parent, "tree")
	old := runGit
	defer func() { runGit = old }()
	listed := true
	runGit = func(_ string, args ...string) ([]byte, error) {
		if args[0] == "worktree" && args[1] == "list" {
			if listed {
				listed = false
				return []byte("worktree " + path + "\n"), nil
			}
			return nil, nil
		}
		return nil, nil
	}
	o := &worktreeOwner{root: parent, parent: parent, path: path, addAttempted: true}
	if err := o.cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(parent); !os.IsNotExist(err) {
		t.Fatalf("parent remains: %v", err)
	}
}
