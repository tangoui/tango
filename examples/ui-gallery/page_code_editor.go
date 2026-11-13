package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/editor"
	editortheme "github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/layout"
)

type codeEditorDemo struct {
	file        string
	large       bool
	readOnly    bool
	lineNumbers bool
	focusWanted bool
	views       map[string]*editor.State
}

func newCodeEditorDemo(large bool) *codeEditorDemo {
	file := "main.go"
	if large {
		file = "app.ts"
	}
	return &codeEditorDemo{file: file, large: large, lineNumbers: true, focusWanted: true, views: map[string]*editor.State{}}
}

func (d *codeEditorDemo) activeState() *editor.State {
	if st := d.views[d.file]; st != nil {
		return st
	}
	st := editor.NewView(editor.NewModel(d.file, codeEditorSample(d.file, d.large), editor.DefaultRegistry()), editortheme.Base("vs"))
	d.views[d.file] = st
	return st
}

func (d *codeEditorDemo) close() {
	for _, st := range d.views {
		st.Close()
	}
	d.views = map[string]*editor.State{}
}

func (d *codeEditorDemo) status() string {
	st := d.views[d.file]
	if st == nil {
		return d.file
	}
	snap := st.Snapshot()
	pos := snap.PosOf(st.Primary().Head)
	mode := "editable"
	if d.readOnly {
		mode = "read-only"
	}
	highlighted := st.Highlighter() != nil && st.Highlighter().Done(snap)
	return fmt.Sprintf("%s | %s | Ln %d, Col %d | %d lines | %d bytes | %s | highlighted=%t", d.file, st.Model.Language(), pos.Line+1, snap.UTF16Col(pos.Line, pos.Col)+1, snap.LineCount(), snap.Len(), mode, highlighted)
}

func (d *codeEditorDemo) render(c *ui.Context) {
	ui.Column(c, func() {
		ui.Row(c, func() {
			ui.Text(c, "文件")
			if ui.Select(c, &d.file, ui.Opts("main.go", "app.ts", "script.py", "app.js")).A11yLabel("文件").Width(160).Changed() {
				d.focusWanted = true
			}
			ui.Checkbox(c, "只读", &d.readOnly).Changed()
			ui.Checkbox(c, "行号", &d.lineNumbers).Changed()
			st := d.activeState()
			st.Options.ReadOnly = d.readOnly
			if ui.Button(c, "撤销").Disabled(d.readOnly).Clicked() {
				st.Execute("undo", nil)
				d.focusWanted = true
			}
			if ui.Button(c, "重做").Disabled(d.readOnly).Clicked() {
				st.Execute("redo", nil)
				d.focusWanted = true
			}
			if ui.Button(c, "恢复示例").Clicked() {
				st.Close()
				delete(d.views, d.file)
				d.focusWanted = true
			}
		}, layout.Gap(12), layout.Middle(), layout.WrapItems())
		st := d.activeState()
		st.Options.ReadOnly = d.readOnly
		st.Theme = editortheme.Base("vs")
		if c.Theme.Dark {
			st.Theme = editortheme.Base("vs-dark")
		}
		// These bounded samples can catch up in foreground batches. No background
		// goroutine or native application lifetime is needed by the static preview.
		if hl := st.Highlighter(); hl != nil {
			if !hl.Highlight(st.Snapshot(), st.Snapshot().LineCount(), 3*time.Millisecond) {
				c.WakeAt(c.Now().Add(16 * time.Millisecond))
			}
		}
		ui.Column(c, func() {
			view := editor.View(c, st).ReadOnly(d.readOnly).LineNumbers(d.lineNumbers)
			view.Box().Semantics(a11y.RoleTextbox, "代码编辑器 "+d.file).A11yState(a11y.Multiline)
			if d.readOnly {
				view.Box().A11yState(a11y.ReadOnly)
			}
			if d.focusWanted {
				c.SetFocus(view.Box().ID())
				d.focusWanted = false
			}
		}, layout.H(330)).Border(1, c.Theme.BorderSecondary).Radius(c.Theme.Radius).Clip()
		ui.Text(c, d.status()).Secondary()
		ui.Text(c, "点击代码后编辑；Tab 缩进，Ctrl/Cmd+Z 撤销，Esc 后按 Tab 离开预览。").Tertiary()
	}, layout.Gap(12))
}

func codeEditorSample(file string, large bool) string {
	switch file {
	case "app.ts":
		if large {
			var source strings.Builder
			source.WriteString("// TypeScript document: 2,000 editable rows\n")
			for i := 0; i < 2000; i++ {
				fmt.Fprintf(&source, "export const row%d = { id: %d, label: \"Tango %d\", enabled: true };\n", i, i, i)
			}
			return source.String()
		}
		return `interface Project {
  name: string;
  stars: number;
}

const project: Project = { name: "Tango", stars: 149 };

export function greet(value: Project): string {
  return ` + "`Hello, ${value.name}!`" + `;
}

console.log(greet(project));
`
	case "script.py":
		return `from dataclasses import dataclass

@dataclass
class Project:
    name: str
    stars: int

def greet(project: Project) -> str:
    return f"Hello, {project.name}!"

print(greet(Project("Tango", 149)))
`
	case "app.js":
		return `const project = { name: "Tango", stars: 149 };

function greet(value) {
  return ` + "`Hello, ${value.name}!`" + `;
}

console.log(greet(project));
`
	default:
		return `package main

import "fmt"

type Project struct {
    Name  string
    Stars int
}

func main() {
    project := Project{Name: "Tango", Stars: 149}
    fmt.Printf("Hello, %s!\n", project.Name)
}
`
	}
}

var codeEditorStandard, codeEditorLarge *codeEditorDemo

func init() {
	gallery.Register(gallery.Page{Name: "CodeEditor", Title: "CodeEditor 代码编辑器", Group: gallery.Form, Order: 81, Width: 1100,
		Render: func(c *ui.Context) {
			if codeEditorStandard == nil {
				codeEditorStandard = newCodeEditorDemo(false)
			}
			codeEditorStandard.render(c)
		},
		Status: func() string {
			if codeEditorStandard == nil {
				return ""
			}
			return codeEditorStandard.status()
		},
	})
	gallery.Register(gallery.Page{Name: "CodeEditorLarge", Title: "CodeEditor 大文档与虚拟滚动", Group: gallery.Form, Order: 82, Parent: "CodeEditor", Width: 1100,
		Render: func(c *ui.Context) {
			if codeEditorLarge == nil {
				codeEditorLarge = newCodeEditorDemo(true)
			}
			codeEditorLarge.render(c)
		},
		Status: func() string {
			if codeEditorLarge == nil {
				return ""
			}
			return codeEditorLarge.status()
		},
	})
}
