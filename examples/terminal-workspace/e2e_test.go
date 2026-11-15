package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/automation"
	"github.com/tangoui/tango/ui/a11y"
)

func TestWorkspaceNative(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("native acceptance uses macOS zsh")
	}
	app := automation.Launch(t, ".", "-shell", "/bin/zsh", "--", "-f")
	app.SetTimeout(15 * time.Second)
	win := app.Window(windowTitle)
	wait := func(label string, fn func() bool) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for !fn() {
			if time.Now().After(deadline) {
				t.Fatalf("waiting for %s:\n%s", label, win.Dump())
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	panes := func() []a11y.Node {
		var out []a11y.Node
		for _, n := range win.Nodes() {
			if n.Role == a11y.RoleTextbox {
				out = append(out, n)
			}
		}
		return out
	}
	textbox := func(name string) *automation.Locator { return win.Find(a11y.RoleTextbox, name) }
	write := func(name, command string) {
		t.Helper()
		textbox(name).Click()
		win.Type(command)
		win.Press("Enter")
	}
	readContains := func(name, value string) {
		t.Helper()
		wait(value, func() bool { return strings.Contains(textbox(name).Value(), value) })
	}
	queryN := 0
	query := func(name, expression string) string {
		t.Helper()
		queryN++
		label := fmt.Sprintf("WSQ%d_", queryN)
		pattern := regexp.MustCompile(label + `([0-9]+(?: [0-9]+)?)_END`)
		write(name, fmt.Sprintf("printf '\\n%s%%s_END\\n' \"%s\"", label, expression))
		var value string
		wait("query "+label, func() bool {
			match := pattern.FindStringSubmatch(textbox(name).Value())
			if len(match) > 1 {
				value = match[1]
				return true
			}
			return false
		})
		return value
	}
	wait("first shell", func() bool { return len(panes()) == 1 && strings.TrimSpace(panes()[0].Value) != "" })
	first := panes()[0].Name
	firstPID := query(first, "$$")
	write(first, "export WORKSPACE_SAMPLE=kept; printf '\\nFIRST_%s\\n' \"$WORKSPACE_SAMPLE\"")
	readContains(first, "FIRST_kept")
	win.Find(a11y.RoleButton, "New tab").Click()
	wait("second shell", func() bool {
		p := panes()
		return len(p) == 1 && p[0].Name != first && strings.TrimSpace(p[0].Value) != ""
	})
	second := panes()[0].Name
	secondPID := query(second, "$$")
	fullSize := query(second, "$(stty size)")
	if firstPID == secondPID {
		t.Fatal("tabs share the same shell")
	}
	win.Press("Cmd+D")
	wait("two split terminals", func() bool { return len(panes()) == 2 })
	third := ""
	for _, n := range panes() {
		if n.Name != second {
			third = n.Name
		}
	}
	thirdPID := query(third, "$$")
	if thirdPID == secondPID {
		t.Fatal("split reused its parent's shell")
	}
	write(third, "printf '\\033[32mTango workspace: 你好\\033[0m\\n'; printf 'SPLIT_%s\\n' ready")
	readContains(third, "SPLIT_ready")
	size := query(second, "$(stty size)")
	for range 3 {
		win.Find(a11y.RoleTab, "").Nth(0).Click()
		readContains(first, "FIRST_kept")
		if query(first, "$$") != firstPID {
			t.Fatal("tab switching restarted the first shell")
		}
		win.Find(a11y.RoleTab, "").Nth(1).Click()
		wait("both panes after switch", func() bool { return len(panes()) == 2 })
		if query(second, "$$") != secondPID {
			t.Fatal("tab switching restarted a split")
		}
		if got := query(second, "$(stty size)"); got != size {
			t.Fatalf("tab switch resized PTY: %s -> %s", size, got)
		}
	}
	bar := win.Find(a11y.RoleSeparator, "").First().Bounds()
	win.Drag(bar.X+bar.W/2, bar.Y+bar.H/2, bar.X+bar.W/2-120, bar.Y+bar.H/2)
	time.Sleep(100 * time.Millisecond) // beyond the terminal view's 50ms resize throttle
	resized := query(second, "$(stty size)")
	before := strings.Fields(size)
	after := strings.Fields(resized)
	cols0, _ := strconv.Atoi(before[1])
	cols1, _ := strconv.Atoi(after[1])
	if cols1 >= cols0 {
		t.Fatalf("drag did not resize the shell: %s -> %s", size, resized)
	}
	t.Logf("independent shell PIDs %s/%s/%s; split PTY %s -> %s after drag", firstPID, secondPID, thirdPID, size, resized)
	if shots := os.Getenv("E2E_SHOTS"); shots != "" {
		win.SaveScreenshot(filepath.Join(shots, "workspace-dark.png"))
		win.Find(a11y.RoleButton, "Light theme").Click()
		win.Find(a11y.RoleButton, "Dark theme").Wait()
		win.SaveScreenshot(filepath.Join(shots, "workspace-light.png"))
	}
	write(third, "exit")
	wait("exited shell is retained", func() bool { return win.Find(a11y.RoleText, "exited").Exists() })
	readContains(third, "SPLIT_ready")
	win.Find(a11y.RoleButton, "Close pane").Click()
	wait("surviving split", func() bool { return len(panes()) == 1 })
	if query(second, "$$") != secondPID {
		t.Fatal("closing a split restarted its sibling")
	}
	if got := query(second, "$(stty size)"); got != fullSize {
		t.Fatalf("collapsed horizontal split left stale PTY size: want %s, got %s", fullSize, got)
	}
	win.Find(a11y.RoleButton, "Split down").Click()
	wait("vertical split", func() bool { return len(panes()) == 2 })
	win.Find(a11y.RoleButton, "Close pane").Click()
	wait("collapsed vertical split", func() bool { return len(panes()) == 1 })
	if got := query(second, "$(stty size)"); got != fullSize {
		t.Fatalf("collapsed vertical split left stale PTY size: want %s, got %s", fullSize, got)
	}
	win.Find(a11y.RoleButton, "Close pane").Click()
	readContains(first, "FIRST_kept")
	if query(first, "$$") != firstPID {
		t.Fatal("closing other tab affected the first shell")
	}
	win.Find(a11y.RoleButton, "Close pane").Click()
	win.Find(a11y.RoleButton, "New terminal").Wait()
	t.Log("three tab cycles, both split axes, drag resize, exit retention and empty state passed")
}
