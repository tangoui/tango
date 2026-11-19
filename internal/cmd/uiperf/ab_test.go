package main

import (
	"errors"
	"os"
	"os/exec"
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

func TestWorktreeOwnerRequiresExactTree(t *testing.T) {
	parent := t.TempDir()
	for _, path := range []string{
		parent,
		filepath.Join(parent, "foo"),
		filepath.Join(parent, "nested", "tree"),
		filepath.Join(filepath.Dir(parent), "outside"),
	} {
		o := &worktreeOwner{parent: parent, path: path}
		if err := o.cleanup(); err == nil {
			t.Fatalf("expected owner path rejection for %q", path)
		}
	}
}

func TestWorktreeOwnerRejectsSymlinkParentAndTree(t *testing.T) {
	t.Run("parent", func(t *testing.T) {
		realParent := t.TempDir()
		linkParent := filepath.Join(t.TempDir(), "parent-link")
		if err := os.Symlink(realParent, linkParent); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		o := &worktreeOwner{parent: linkParent, path: filepath.Join(linkParent, "tree")}
		if err := o.cleanup(); err == nil {
			t.Fatal("expected symlink parent rejection")
		}
	})

	t.Run("tree", func(t *testing.T) {
		parent := t.TempDir()
		if err := os.Symlink(t.TempDir(), filepath.Join(parent, "tree")); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		o := &worktreeOwner{parent: parent, path: filepath.Join(parent, "tree")}
		if err := o.cleanup(); err == nil {
			t.Fatal("expected symlink tree rejection")
		}
	})
}

func TestRunABJoinsBenchmarkAndCleanupErrors(t *testing.T) {
	oldCmd, oldGit := runCmd, runGit
	defer func() { runCmd, runGit = oldCmd, oldGit }()
	compileErr := errors.New("head compile failed")
	cleanupErr := errors.New("cleanup remove failed")
	var addedPath string
	runCmd = func(_ string, args ...string) error {
		if len(args) >= 5 && args[0] == "git" && args[1] == "worktree" && args[2] == "add" {
			addedPath = args[4]
			return os.Mkdir(addedPath, 0o755)
		}
		if len(args) >= 3 && args[0] == "go" && args[1] == "test" && args[2] == "-c" {
			return compileErr
		}
		return nil
	}
	runGit = func(_ string, args ...string) ([]byte, error) {
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return []byte("worktree " + addedPath + "\n"), nil
		}
		return nil, cleanupErr
	}
	var out strings.Builder
	err := runAB("HEAD", "BenchmarkNoop", ".", 1, &out)
	if addedPath != "" {
		parent := filepath.Dir(addedPath)
		if _, statErr := os.Stat(parent); statErr != nil {
			t.Errorf("parent should be preserved after failed cleanup: %v", statErr)
		}
		os.RemoveAll(parent)
	}
	if !errors.Is(err, compileErr) || !errors.Is(err, cleanupErr) {
		t.Fatalf("error does not expose compile and cleanup failures: %v", err)
	}
}

func TestWorktreeOwnerRejectsSymlinkIntermediate(t *testing.T) {
	realDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(realDir, "owned"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "mid-link")
	if err := os.Symlink(realDir, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	parent := filepath.Join(link, "owned")
	o := &worktreeOwner{parent: parent, path: filepath.Join(parent, "tree")}
	if err := o.cleanup(); err == nil {
		t.Fatal("expected symlink intermediate rejection")
	}
	if _, err := os.Stat(filepath.Join(realDir, "owned")); err != nil {
		t.Fatalf("symlinked parent target removed: %v", err)
	}
}

func TestWorktreeOwnerRemoveSucceedsButRegistrationRemains(t *testing.T) {
	parent := t.TempDir()
	path := filepath.Join(parent, "tree")
	old := runGit
	defer func() { runGit = old }()
	removed := false
	runGit = func(_ string, args ...string) ([]byte, error) {
		if args[0] == "worktree" && args[1] == "remove" {
			removed = true
			return nil, nil
		}
		return []byte("worktree " + path + "\n"), nil
	}
	o := &worktreeOwner{root: parent, parent: parent, path: path, addAttempted: true, registered: true}
	err := o.cleanup()
	if err == nil || !strings.Contains(err.Error(), "registration remains") {
		t.Fatalf("expected registration remains error, got %v", err)
	}
	if !removed {
		t.Fatal("remove was not attempted")
	}
	if _, err := os.Stat(parent); err != nil {
		t.Fatalf("parent should remain: %v", err)
	}
}

func TestWorktreeOwnerExactCleanupWithGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("commit", "-q", "--allow-empty", "-m", "init")
	parent, err := os.MkdirTemp("", "uiperf-base-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(parent)
	path := filepath.Join(parent, "tree")
	git("worktree", "add", "-q", "--detach", path, "HEAD")
	if present, err := registrationPresent(repo, path); err != nil || !present {
		t.Fatalf("registration before cleanup present=%v err=%v", present, err)
	}
	o := &worktreeOwner{root: repo, parent: parent, path: path, addAttempted: true, registered: true}
	if err := o.cleanup(); err != nil {
		t.Fatal(err)
	}
	if present, err := registrationPresent(repo, path); err != nil || present {
		t.Fatalf("registration present=%v err=%v", present, err)
	}
	if _, err := os.Stat(parent); !os.IsNotExist(err) {
		t.Fatalf("parent remains: %v", err)
	}
}
