// Command terminal-workspace demonstrates application-owned terminal sessions:
// real local shells, editable tabs, nested split panes and window shortcuts.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/sys/pty"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/terminal"
	"github.com/tangoui/tango/uihost"
)

const windowTitle = "Tango Terminal Workspace"

func main() {
	shell := flag.String("shell", "", "shell executable (default: user's shell); pass its arguments after --")
	dir := flag.String("dir", "", "working directory for new shells (default: current directory)")
	light := flag.Bool("light", false, "start with the light theme")
	renderer := flag.String("renderer", "", "cpu or gpu; default: automatic")
	quitAfter := flag.Duration("quit-after", 0, "quit after this duration (for demos/screenshots)")
	flag.Parse()
	if *renderer != "" {
		if *renderer != "cpu" && *renderer != "gpu" {
			log.Fatal("-renderer must be cpu or gpu")
		}
		if err := os.Setenv("TANGO_RENDERER", *renderer); err != nil {
			log.Fatal(err)
		}
	}
	path, args := pty.DefaultShell()
	if *shell != "" {
		path, args = *shell, []string{*shell}
	}
	if len(flag.Args()) > 0 {
		args = append([]string{path}, flag.Args()...)
	}
	quitOnClose := true
	app := tango.New(tango.Options{Name: windowTitle, ID: "dev.tango.terminal.workspace", QuitOnLastClose: &quitOnClose})
	a := &terminalApp{dark: !*light, fontSize: 13, focusPending: true}
	a.workspace.openSession = func(id string) (*session, error) {
		conn, err := pty.Start(pty.Options{Path: path, Args: args, Dir: *dir, Cols: 80, Rows: 24})
		if err != nil {
			return nil, fmt.Errorf("could not start %s: %w", path, err)
		}
		term := terminal.New(terminal.Options{Conn: conn})
		return &session{id: id, pid: conn.Pid(), term: term, closeConn: term.Close}, nil
	}
	defer a.workspace.close()
	a.action(a.workspace.newTab)
	app.OnReady(func() {
		runner, surface, err := uihost.Open(app, uihost.Options{Surface: tango.SurfaceOptions{
			Title: windowTitle, Width: 1180, Height: 760, MinWidth: 720, MinHeight: 420,
			Resizable: true, Centered: true,
		}}, a.page, ui.RunnerOptions{Theme: a.theme()})
		if err != nil {
			log.Fatal(err)
		}
		a.invalidate = runner.Invalidate
		surface.OnClosed(a.workspace.close)
		registerShortcuts(surface, runner, a)
		if *quitAfter > 0 {
			time.AfterFunc(*quitAfter, func() { app.Post(app.Quit) })
		}
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// registerShortcuts binds the workspace shortcuts. They fire on the main
// thread and change the workspace, which belongs to the window's UI thread,
// so each runs through runner.Post.
func registerShortcuts(surface *tango.Surface, runner *ui.Runner, a *terminalApp) {
	primary := "Ctrl+Shift"
	if runtime.GOOS == "darwin" {
		primary = "Cmd"
	}
	bindings := []struct {
		accelerator string
		run         func()
	}{
		{primary + "+T", func() { a.action(a.workspace.newTab) }},
		{"Ctrl+PageDown", func() { a.changed(func() { a.workspace.cycleTab(1) }) }},
		{"Ctrl+PageUp", func() { a.changed(func() { a.workspace.cycleTab(-1) }) }},
		{primary + "+D", func() { a.action(func() error { return a.workspace.split(splitRight) }) }},
		{"CmdOrCtrl+Alt+D", func() { a.action(func() error { return a.workspace.split(splitDown) }) }},
		{"CmdOrCtrl+Shift+W", func() { a.changed(a.workspace.closePane) }},
		{"Ctrl+Shift+Right", func() { a.changed(a.workspace.cyclePane) }},
		{"CmdOrCtrl+=", func() { a.changed(func() { a.fontSize = min(24, a.fontSize+1) }) }},
		{"CmdOrCtrl+-", func() { a.changed(func() { a.fontSize = max(10, a.fontSize-1) }) }},
		{"CmdOrCtrl+0", func() { a.changed(func() { a.fontSize = 13 }) }},
	}
	for _, binding := range bindings {
		run := binding.run
		if _, err := uihost.ShortcutWith(surface, binding.accelerator, uihost.ShortcutOptions{InTextInput: true}, func() { runner.Post(run) }); err != nil {
			log.Printf("terminal workspace: shortcut %s: %v", binding.accelerator, err)
		}
	}
}
