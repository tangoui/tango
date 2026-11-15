package main

// End-to-end acceptance of the terminal (milestone 1), driving the example
// in a real window with a real shell (zsh -f): colored output, history,
// completion, Ctrl+C, Chinese text, scrollback while output arrives,
// bracketed paste, select and copy, Cmd+K, Ctrl+D and the app quitting with
// its program, and a window resize reaching the program. The screen is
// read from the view's accessibility value (the visible rows as text).
// macOS only (zsh, pbcopy, System Events); see
// docs/specs/terminal-m1-report.md.
//
//	TANGO_E2E=1 go test ./examples/terminal -v
//	TANGO_E2E=1 E2E_SHOTS=/tmp/shots go test ./examples/terminal -v  # also save screenshots

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/automation"
	"github.com/tangoui/tango/ui/a11y"
)

var shots = os.Getenv("E2E_SHOTS")

func onlyMac(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only (zsh, pbcopy, System Events)")
	}
}

type rig struct {
	t   *testing.T
	app *automation.App
	win *automation.Window
	tb  *automation.Locator
}

func (r *rig) screen() string { return r.tb.Value() }

func (r *rig) waitFor(what string, d time.Duration, ok func(s string) bool) string {
	r.t.Helper()
	deadline := time.Now().Add(d)
	for {
		s := r.screen()
		if ok(s) {
			return s
		}
		if time.Now().After(deadline) {
			b := r.tb.Bounds()
			r.win.Move(b.X+3, b.Y+3) // force a frame: was the screen stale?
			time.Sleep(300 * time.Millisecond)
			if s2 := r.screen(); ok(s2) {
				r.t.Fatalf("STALE FRAME waiting for %s: the screen was right after a forced frame; before:\n%s", what, s)
			}
			r.t.Fatalf("timed out waiting for %s; screen:\n%s", what, s)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (r *rig) waitText(sub string) string {
	r.t.Helper()
	return r.waitFor(sub, 10*time.Second, func(s string) bool { return strings.Contains(s, sub) })
}

func (r *rig) run(cmd string) {
	r.t.Helper()
	r.win.Type(cmd)
	r.win.Press("Enter")
}

func (r *rig) shot(name string) {
	r.t.Helper()
	if shots != "" {
		r.win.SaveScreenshot(filepath.Join(shots, name+".png"))
	}
}

func lastLine(s string) string {
	ls := strings.Split(strings.TrimRight(s, "\n"), "\n")
	return ls[len(ls)-1]
}

func TestAccept(t *testing.T) {
	onlyMac(t)
	repo, _ := filepath.Abs("../..")
	app := automation.Launch(t, ".", "-shell", "/bin/zsh -f")
	app.SetTimeout(20 * time.Second)
	win := app.Window("Tango Terminal")
	r := &rig{t: t, app: app, win: win, tb: win.Find(a11y.RoleTextbox, "Terminal")}
	r.tb.Wait()
	r.waitFor("a prompt", 10*time.Second, func(s string) bool { return strings.TrimSpace(s) != "" })
	r.run("PS1='$ '; cd " + repo + "; clear")
	r.waitFor("the new prompt", 10*time.Second, func(s string) bool { return strings.TrimSpace(s) == "$" })

	t.Run("ls", func(t *testing.T) {
		r.run("ls --color=always -la")
		r.waitText("webview_test.go")
		r.shot("01-ls-color")
	})
	t.Run("git log", func(t *testing.T) {
		r.run("clear; git log --oneline --color=always | head -20")
		r.waitText("size the segment cache")
		r.shot("02-git-log")
	})
	t.Run("history", func(t *testing.T) {
		r.run("clear; echo hist-one")
		r.waitText("hist-one")
		r.win.Press("Up")
		r.waitFor("the previous command", 5*time.Second, func(s string) bool {
			return strings.HasSuffix(lastLine(s), "echo hist-one")
		})
		r.win.Press("Ctrl+U")
	})
	t.Run("tab completion", func(t *testing.T) {
		r.run("clear")
		r.win.Type("ls examples/termi")
		r.win.Press("Tab")
		r.waitFor("completion", 5*time.Second, func(s string) bool {
			return strings.Contains(lastLine(s), "examples/terminal/")
		})
		r.win.Press("Ctrl+U")
	})
	t.Run("ctrl+c", func(t *testing.T) {
		r.run("clear; sleep 10; echo not-interrupted")
		time.Sleep(500 * time.Millisecond)
		start := time.Now()
		r.win.Press("Ctrl+C")
		r.run("echo after-int")
		s := r.waitText("after-int\n")
		if time.Since(start) > 5*time.Second || strings.Contains(s, "not-interrupted\n") {
			t.Fatalf("sleep was not interrupted; screen:\n%s", s)
		}
	})
	t.Run("chinese", func(t *testing.T) {
		r.run("clear")
		r.waitFor("a cleared screen", 5*time.Second, func(s string) bool { return strings.TrimSpace(s) == "$" })
		r.win.Type("echo 你好")
		r.waitFor("the command line", 5*time.Second, func(s string) bool { return lastLine(s) == "$ echo 你好" })
		r.win.Press("Enter")
		r.waitText("$ echo 你好\n你好\n$")
		r.shot("03-chinese")
	})
	t.Run("scrollback stays while output arrives", func(t *testing.T) {
		r.run("clear; for i in $(seq 1 1500); do echo flood-$i; sleep 0.002; done; echo flood-done")
		r.waitText("flood-200")
		b := r.tb.Bounds()
		r.win.Scroll(b.X+b.W/2, b.Y+b.H/2, 0, -400) // back (positive DY reveals what is below)
		time.Sleep(200 * time.Millisecond)
		a := r.screen()
		time.Sleep(700 * time.Millisecond)
		if c := r.screen(); c != a {
			t.Fatalf("the viewport moved while scrolled back:\nbefore:\n%s\nafter:\n%s", a, c)
		}
		r.shot("04-scrolled-back")
		// Typing returns to the bottom and the view follows the output.
		r.win.Type(" ")
		r.waitFor("the end of the flood", 60*time.Second, func(s string) bool { return strings.Contains(s, "\nflood-done") })
		r.win.Press("Ctrl+U")
	})
	t.Run("paste multi-line (bracketed)", func(t *testing.T) {
		old, _ := exec.Command("pbpaste").Output()
		t.Cleanup(func() { setClipboard(t, string(old)) })
		setClipboard(t, "echo paste-a\necho paste-b")
		r.run("clear")
		r.win.Press("Cmd+V")
		// zsh's bracketed paste keeps both lines on the command line
		// until Enter.
		s := r.waitText("echo paste-b")
		if strings.Contains(s, "\npaste-a") {
			t.Fatalf("paste ran before Enter:\n%s", s)
		}
		r.win.Press("Enter")
		r.waitText("\npaste-b")
		r.shot("05-paste")
	})
	t.Run("select and copy", func(t *testing.T) {
		old, _ := exec.Command("pbpaste").Output()
		t.Cleanup(func() { setClipboard(t, string(old)) })
		cellH := lineHeight(t, r)
		r.run("clear; echo copy-me-please")
		s := r.waitFor("copy-me-please", 5*time.Second, func(s string) bool { return slices.Contains(strings.Split(s, "\n"), "copy-me-please") })
		b := r.tb.Bounds()
		row := -1
		for i, l := range strings.Split(s, "\n") {
			if l == "copy-me-please" {
				row = i
			}
		}
		y := b.Y + (float32(row)+0.5)*cellH
		r.win.Drag(b.X+2, y, b.X+b.W*0.5, y)
		r.win.Press("Cmd+C")
		time.Sleep(200 * time.Millisecond)
		got, _ := exec.Command("pbpaste").Output()
		if strings.TrimSpace(string(got)) != "copy-me-please" {
			t.Fatalf("clipboard = %q, want copy-me-please", got)
		}
		r.shot("06-selection")
	})
	t.Run("cmd+k", func(t *testing.T) {
		r.run("echo before-clear")
		r.waitText("\nbefore-clear")
		r.win.Press("Cmd+K")
		r.waitFor("a cleared screen", 5*time.Second, func(s string) bool { return !strings.Contains(s, "before-clear") })
	})
	t.Run("ctrl+d ends a shell; the app quits with its program", func(t *testing.T) {
		// Ctrl+D ends an inner shell (EOF reaches the program) ...
		r.run("clear; zsh -f")
		r.waitFor("the inner prompt", 5*time.Second, func(s string) bool { return strings.Contains(lastLine(s), "%") })
		r.win.Press("Ctrl+D")
		r.run("echo back-outside")
		r.waitText("\nback-outside")
		// ... and the app quits when the outer one exits. (Ctrl+D there
		// would quit the app before the automation call returns.)
		r.win.Type("sleep 0.5; exit") // exit after the automation call returns
		r.win.Press("Enter")
		deadline := time.Now().Add(10 * time.Second)
		for !app.Exited() {
			if time.Now().After(deadline) {
				t.Fatal("the app did not quit after the shell exited")
			}
			time.Sleep(100 * time.Millisecond)
		}
	})
}

// lineHeight is the cell height: the view's height over the rows stty
// reports.
func lineHeight(t *testing.T, r *rig) float32 {
	t.Helper()
	rows, _ := sttySize(t, r)
	return r.tb.Bounds().H / float32(rows)
}

func sttySize(t *testing.T, r *rig) (rows, cols int) {
	t.Helper()
	r.run("clear; stty size")
	s := r.waitFor("stty size", 5*time.Second, func(s string) bool {
		return len(strings.Fields(strings.Split(s, "\n")[0])) == 2
	})
	f := strings.Fields(strings.Split(s, "\n")[0])
	rows, _ = strconv.Atoi(f[0])
	cols, _ = strconv.Atoi(f[1])
	return rows, cols
}

func setClipboard(t *testing.T, s string) {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(s)
	if err := cmd.Run(); err != nil {
		t.Fatalf("pbcopy: %v", err)
	}
}

// TestAcceptResize resizes the real window (System Events) and checks
// that the program sees the new size.
func TestAcceptResize(t *testing.T) {
	onlyMac(t)
	app := automation.Launch(t, ".", "-shell", "/bin/zsh -f")
	win := app.Window("Tango Terminal")
	r := &rig{t: t, app: app, win: win, tb: win.Find(a11y.RoleTextbox, "Terminal")}
	r.tb.Wait()
	r.waitFor("a prompt", 10*time.Second, func(s string) bool { return strings.TrimSpace(s) != "" })
	r.run("PS1='$ '; clear")
	r.waitFor("prompt", 10*time.Second, func(s string) bool { return strings.TrimSpace(s) == "$" })
	rows0, cols0 := sttySize(t, r)
	proc := procName(t)
	script := `tell application "System Events" to tell (first process whose unix id is ` + proc + `) to set size of window 1 to {640, 400}`
	if out, err := exec.Command("osascript", "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("osascript: %v: %s", err, out)
	}
	time.Sleep(300 * time.Millisecond) // past the 50 ms resize throttle
	rows1, cols1 := sttySize(t, r)
	t.Logf("stty size %dx%d -> %dx%d", rows0, cols0, rows1, cols1)
	if rows1 >= rows0 || cols1 >= cols0 {
		t.Fatalf("stty size did not follow the smaller window: %dx%d -> %dx%d", rows0, cols0, rows1, cols1)
	}
	r.shot("07-resized")
	r.win.Type("sleep 0.5; exit")
	r.win.Press("Enter")
}

// procName returns the pid of the launched example: automation builds it
// as "app", the newest such process.
func procName(t *testing.T) string {
	out, err := exec.Command("pgrep", "-n", "-x", "app").Output()
	if err != nil {
		t.Fatalf("pgrep: %v", err)
	}
	return strings.TrimSpace(string(out))
}
