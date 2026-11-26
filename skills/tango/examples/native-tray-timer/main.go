// Native Tray Timer pairs a countdown and tray menu with a saved checklist.
package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/service/paths"
	"github.com/tangoui/tango/sys/menu"
	"github.com/tangoui/tango/sys/tray"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/uihost"
)

const appID = "io.tangoui.examples.traytimer"

//go:embed timer.png
var trayIcon []byte

type task struct {
	ID   int64
	Text string
	Done bool
}

type timerPage struct {
	runner   *ui.Runner
	deadline time.Time
	tasks    []task
	draft    string
	file     string
}

func (p *timerPage) load() error {
	data, err := os.ReadFile(p.file)
	if errors.Is(err, os.ErrNotExist) {
		p.tasks = []task{{ID: 1, Text: "Pick a task for this focus session"}}
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &p.tasks)
}

func (p *timerPage) save() {
	if err := p.writeTasks(); err != nil {
		log.Printf("save checklist: %v", err)
	}
}

func (p *timerPage) writeTasks() error {
	if err := os.MkdirAll(filepath.Dir(p.file), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(p.file), ".checklist-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	err = errors.Join(json.NewEncoder(f).Encode(p.tasks), f.Close())
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), p.file)
}

func (p *timerPage) start() {
	p.deadline = time.Now().Add(25 * time.Minute)
	deadline := p.deadline
	time.AfterFunc(time.Until(deadline), func() {
		p.runner.Post(func() {
			if p.deadline.Equal(deadline) {
				p.deadline = time.Time{}
			}
		})
	})
}

func (p *timerPage) draw(c *ui.Context) {
	ui.Column(c, func() {
		ui.Card(c, "Focus timer").Body(func() {
			seconds := int(max(0, time.Until(p.deadline)).Seconds() + 0.999)
			if p.deadline.IsZero() {
				seconds = 25 * 60
			} else if seconds > 0 {
				c.WakeAt(p.deadline.Add(-time.Duration(seconds-1) * time.Second))
			}
			ui.Row(c, func() {
				ui.Text(c, fmt.Sprintf("%02d:%02d", seconds/60, seconds%60))
				if ui.Button(c, "Start 25 minutes").Primary().Clicked() {
					p.start()
				}
			}, layout.Gap(12), layout.Middle())
		})
		ui.Card(c, "Checklist").Body(func() {
			ui.Column(c, func() {
				ui.Row(c, func() {
					input := ui.Input(c, &p.draft).Placeholder("New task").Layout(layout.Grow(1))
					if ui.Button(c, "Add").Clicked() || input.Submitted() {
						if text := strings.TrimSpace(p.draft); text != "" {
							p.tasks = append(p.tasks, task{ID: time.Now().UnixNano(), Text: text})
							p.draft = ""
							p.save()
						}
					}
				}, layout.Gap(8))
				remove := -1
				for i := range p.tasks {
					row := &p.tasks[i]
					ui.Key(c, row.ID, func() {
						ui.Row(c, func() {
							if ui.Checkbox(c, row.Text, &row.Done).Changed() {
								p.save()
							}
							ui.Spacer(c)
							if ui.Button(c, "Remove").Clicked() {
								remove = i
							}
						}, layout.Gap(8), layout.Middle())
					})
				}
				if remove >= 0 {
					p.tasks = slices.Delete(p.tasks, remove, remove+1)
					p.save()
				}
			}, layout.Gap(8))
		})
	}, layout.Pad(24), layout.Gap(16))
}

func installMenus(app *tango.App, s *tango.Surface, page *timerPage) {
	bar := menu.New()
	bar.Submenu("Tray Timer").Role(menu.Quit)
	bar.Submenu("Timer").Add("Start 25 minutes", func() {
		page.runner.Post(page.start)
	}).Accelerator = "CmdOrCtrl+T"
	bar.Submenu("Edit").Role(menu.EditMenu)
	if err := menu.SetAppMenu(app, bar); err != nil {
		log.Print(err)
	}

	t, err := tray.New(app, tray.Options{Icon: trayIcon, Tooltip: "Tray Timer", Template: true})
	if err != nil {
		log.Print(err)
		return
	}
	m := menu.New()
	var toggle *menu.Item
	toggle = m.Add("Hide window", func() {
		if s.IsVisible() {
			s.Hide()
			toggle.SetLabel("Show window")
		} else {
			s.Show()
			s.Focus()
			toggle.SetLabel("Hide window")
		}
		if err := m.Update(); err != nil {
			log.Print(err)
		}
	})
	m.Add("Quit", app.Quit)
	if err := t.SetMenu(m); err != nil {
		log.Print(err)
	}
}

func main() {
	dir := flag.String("data-dir", "", "directory for the saved checklist")
	flag.Parse()
	if *dir == "" {
		var err error
		*dir, err = paths.Data(appID)
		if err != nil {
			log.Fatal(err)
		}
	}
	page := &timerPage{file: filepath.Join(*dir, "checklist.json")}
	if err := page.load(); err != nil {
		log.Fatal(err)
	}
	app := tango.New(tango.Options{Name: "Tray Timer", ID: appID})
	app.OnReady(func() {
		runner, surface, err := uihost.Open(app, uihost.Options{Surface: tango.SurfaceOptions{
			Title: "Tray Timer", Width: 620, Height: 430, Resizable: true,
		}}, page.draw, ui.RunnerOptions{})
		if err != nil {
			log.Fatal(err)
		}
		page.runner = runner
		installMenus(app, surface, page)
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
