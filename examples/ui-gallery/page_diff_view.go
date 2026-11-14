package main

import (
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor"
	"github.com/tangoui/tango/ui/editor/diffview"
	editortheme "github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/layout"
)

// The DiffView page (ui/editor/diffview): two versions of a Go file side
// by side or inline, with the unchanged regions folded on demand.

type diffViewDemo struct {
	d      *diffview.State
	mode   string
	folded bool
}

var diffDemo *diffViewDemo

func (g *diffViewDemo) state() *diffview.State {
	if g.d == nil {
		reg := editor.DefaultRegistry()
		g.d = diffview.New(editor.NewModel("main.go", diffOriginal, reg), editor.NewModel("main.go", diffModified, reg), editortheme.Base("vs"))
		g.mode = "sideBySide"
	}
	return g.d
}

func (g *diffViewDemo) render(c *ui.Context) {
	d := g.state()
	ui.Column(c, func() {
		ui.Row(c, func() {
			ui.Segmented(c, &g.mode, []ui.Option{{Label: "并排", Value: "sideBySide"}, {Label: "内联", Value: "inline"}})
			ui.Checkbox(c, "折叠未改动区域", &g.folded)
			if ui.Button(c, "上一处").Clicked() {
				d.Prev()
			}
			if ui.Button(c, "下一处").Clicked() {
				d.Next()
			}
		}, layout.Gap(12), layout.Middle(), layout.WrapItems())
		d.Options.Mode = diffview.SideBySide
		if g.mode == "inline" {
			d.Options.Mode = diffview.Inline
		}
		d.Options.UseInlineWhenSpaceIsLimited = false
		d.Options.HideUnchangedRegions.Enabled = g.folded
		th := editortheme.Base("vs")
		if c.Theme.Dark {
			th = editortheme.Base("vs-dark")
		}
		for _, st := range []*editor.State{d.Original, d.Modified} {
			st.Theme = th
			if hl := st.Highlighter(); hl != nil && !hl.Highlight(st.Snapshot(), st.Snapshot().LineCount(), 3*time.Millisecond) {
				c.WakeAt(c.Now().Add(16 * time.Millisecond))
			}
		}
		ui.Column(c, func() {
			diffview.View(c, d)
		}, layout.H(420)).Border(1, c.Theme.BorderSecondary).Radius(c.Theme.Radius).Clip()
		ui.Text(c, "Alt+F5 / Shift+Alt+F5 在差异间跳转；中间的箭头还原一处修改。").Tertiary()
	}, layout.Gap(12))
}

// waitDiff runs frames until the background diff is shown.
func waitDiff(dr *ui.Driver, g *diffViewDemo) {
	for range 400 {
		dr.Frame()
		if _, ok := g.state().Result(); ok {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	dr.Settle()
}

func init() {
	pages := []struct {
		name, title, mode string
		folded            bool
		order             int
	}{
		{"DiffView", "DiffView 差异编辑器", "sideBySide", false, 83},
		{"DiffViewInline", "DiffView 内联模式", "inline", false, 84},
		{"DiffViewFolded", "DiffView 折叠未改动区域", "sideBySide", true, 85},
	}
	for _, p := range pages {
		p := p
		parent := ""
		if p.name != "DiffView" {
			parent = "DiffView"
		}
		gallery.Register(gallery.Page{Name: p.name, Title: p.title, Group: gallery.Form, Order: p.order, Parent: parent, Width: 1200,
			Render: func(c *ui.Context) {
				if diffDemo == nil {
					diffDemo = &diffViewDemo{}
				}
				diffDemo.render(c)
			},
			Setup: func(dr *ui.Driver) {
				if diffDemo == nil {
					diffDemo = &diffViewDemo{}
				}
				diffDemo.state()
				diffDemo.mode, diffDemo.folded = p.mode, p.folded
				waitDiff(dr, diffDemo)
				if p.folded {
					// Scroll the placeholder of the folded functions into view.
					diffDemo.d.Modified.RevealLine(20, editor.RevealTop)
					dr.Settle()
				} else {
					diffDemo.d.Modified.SetScrollTop(0)
					dr.Settle()
				}
			},
		})
	}
}

const diffOriginal = `package main

import (
	"fmt"
	"strings"
)

// Project is a repository with its stars.
type Project struct {
	Name  string
	Stars int
}

func (p Project) String() string {
	return fmt.Sprintf("%s (%d)", p.Name, p.Stars)
}

func popular(ps []Project, min int) []Project {
	var out []Project
	for _, p := range ps {
		if p.Stars >= min {
			out = append(out, p)
		}
	}
	return out
}

func names(ps []Project) string {
	var b strings.Builder
	for i, p := range ps {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(p.Name)
	}
	return b.String()
}

func total(ps []Project) int {
	n := 0
	for _, p := range ps {
		n += p.Stars
	}
	return n
}

func average(ps []Project) float64 {
	if len(ps) == 0 {
		return 0
	}
	return float64(total(ps)) / float64(len(ps))
}

func main() {
	ps := []Project{{"tango", 149}, {"duet", 12}, {"viva", 7}}
	fmt.Println(names(popular(ps, 10)))
	fmt.Println(total(ps))
	fmt.Println("debug:", ps)
}
`

const diffModified = `package main

import (
	"fmt"
	"slices"
	"strings"
)

// Project is a repository with its stars and topics.
type Project struct {
	Name   string
	Stars  int
	Topics []string
}

func (p Project) String() string {
	return fmt.Sprintf("%s (%d)", p.Name, p.Stars)
}

func popular(ps []Project, min int) []Project {
	return slices.DeleteFunc(slices.Clone(ps), func(p Project) bool { return p.Stars < min })
}

func names(ps []Project) string {
	var b strings.Builder
	for i, p := range ps {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(p.Name)
	}
	return b.String()
}

func total(ps []Project) int {
	n := 0
	for _, p := range ps {
		n += p.Stars
	}
	return n
}

func average(ps []Project) float64 {
	if len(ps) == 0 {
		return 0
	}
	return float64(total(ps)) / float64(len(ps))
}

func main() {
	ps := []Project{{"tango", 149, nil}, {"duet", 12, nil}, {"viva", 7, nil}}
	fmt.Println(names(popular(ps, 10)))
	fmt.Println(total(ps), average(ps))
}
`
