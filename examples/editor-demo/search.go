package main

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor"
	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

type searchPanel struct {
	open                    bool
	replaceMode, focusQuery bool
	query                   string
	replacement             string
	caseSensitive           bool
	status                  string
}

// showSearch preserves the query when switching between Find and Replace.
func showSearch(replace bool) {
	search.open, search.replaceMode, search.focusQuery = true, replace, true
	search.status = ""
	if runner != nil {
		runner.Invalidate()
	}
}

func (p *searchPanel) render(c *ui.Context, target *editor.State) bool {
	if !p.open || target == nil {
		return false
	}
	th := c.Theme
	ui.Column(c, func() {
		ui.Row(c, func() {
			toggleIcon := icon.Right
			if p.replaceMode {
				toggleIcon = icon.Down
			}
			shellIcon(c, toggleIcon, "Toggle replacement", func() { p.replaceMode = !p.replaceMode; ui.Invalidate(c.Window()) })
			query := ui.Input(c, &p.query).Placeholder("Find").A11yLabel("Find text").Layout(layout.Grow(1), layout.MinW(0))
			if p.focusQuery {
				query.Focus()
				p.focusQuery = false
			}
			if query.Submitted() {
				p.find(target, true)
			}
			shellIcon(c, icon.ArrowUp, "Previous match", func() { p.find(target, false) })
			shellIcon(c, icon.ArrowDown, "Next match", func() { p.find(target, true) })
			shellIcon(c, icon.Close, "Close search", func() { p.close(c) })
		}, layout.Middle(), layout.Gap(th.PaddingXXS))
		if p.replaceMode {
			ui.Row(c, func() {
				ui.Div(c, layout.W(th.ControlHeightSM))
				replacement := ui.Input(c, &p.replacement).Placeholder("Replace with").A11yLabel("Replacement text").Layout(layout.Grow(1), layout.MinW(0))
				if replacement.Submitted() {
					p.replace(target)
				}
				if ui.Button(c, "Replace").Small().A11yLabel("Replace match").Clicked() {
					p.replace(target)
				}
				if ui.Button(c, "Replace all").Small().Clicked() {
					count, err := replaceAll(target, p.query, p.replacement, p.caseSensitive)
					if err != nil {
						p.status = err.Error()
					} else {
						p.status = replaceStatus(count)
					}
				}
			}, layout.Middle(), layout.Gap(th.PaddingXXS))
		}
		ui.Row(c, func() {
			ui.Div(c, layout.W(th.ControlHeightSM))
			ui.Checkbox(c, "Match case", &p.caseSensitive)
			ui.Spacer(c)
			if p.status != "" {
				ui.Text(c, p.status).Size(th.FontSizeSM).Secondary().NoWrap().Ellipsis().Layout(layout.MaxW(220))
			}
		}, layout.Middle(), layout.Gap(th.PaddingXS))
	}, layout.Pad(th.PaddingXS), layout.Gap(th.PaddingXXS)).Bg(th.BgContainer)
	if c.Escape() {
		p.close(c)
	}
	return true
}

func (p *searchPanel) close(c *ui.Context) {
	p.open = false
	restoreEditorFocus()
	ui.Invalidate(c.Window())
}

func replaceStatus(count int) string {
	if count == 0 {
		return "No matches"
	}
	return plural(count, "replacement", "replacements")
}

func plural(count int, singular, plural string) string {
	if count == 1 {
		return "1 " + singular
	}
	return fmtInt(count) + " " + plural
}

func fmtInt(n int) string {
	return strconv.Itoa(n)
}

func (p *searchPanel) find(target *editor.State, forward bool) bool {
	if p.query == "" {
		p.status = errEmptySearch.Error()
		return false
	}
	snap := target.Snapshot()
	text := snap.String()
	sel := target.Primary()
	start := min(sel.Anchor, sel.Head)
	if forward {
		start = max(sel.Anchor, sel.Head)
	}
	if start < 0 || start > len(text) {
		start = 0
	}
	if a, b, ok := findMatch(text, p.query, p.caseSensitive, start, forward); ok {
		target.SetSelections([]editor.Selection{{Anchor: a, Head: b}})
		target.RevealCaret()
		p.status = "Match found"
		return true
	}
	p.status = "No matches"
	return false
}

func (p *searchPanel) replace(target *editor.State) {
	if p.query == "" {
		p.status = errEmptySearch.Error()
		return
	}
	sel := target.Primary()
	a, b := sel.Anchor, sel.Head
	if a > b {
		a, b = b, a
	}
	text := target.Snapshot().Slice(a, b)
	matched := text == p.query
	if !p.caseSensitive {
		matched = strings.EqualFold(text, p.query)
	}
	if !matched {
		if p.find(target, true) {
			p.status = "Match selected; press Replace again"
		}
		return
	}
	if err := target.Edit([]buffer.Edit{{Start: a, End: b, Text: p.replacement}}, editor.Selection{Anchor: a + len(p.replacement), Head: a + len(p.replacement)}); err != nil {
		p.status = err.Error()
		return
	}
	p.status = "1 replacement"
}

func findMatch(text, query string, caseSensitive bool, start int, forward bool) (int, int, bool) {
	if query == "" {
		return 0, 0, false
	}
	if caseSensitive {
		if forward {
			i := strings.Index(text[start:], query)
			if i >= 0 {
				i += start
				return i, i + len(query), true
			}
			if i = strings.Index(text[:start], query); i >= 0 {
				return i, i + len(query), true
			}
			return 0, 0, false
		}
		before := text[:start]
		if i := strings.LastIndex(before, query); i >= 0 {
			return i, i + len(query), true
		}
		if i := strings.LastIndex(text, query); i >= 0 {
			return i, i + len(query), true
		}
		return 0, 0, false
	}
	re, err := regexp.Compile("(?i)" + regexp.QuoteMeta(query))
	if err != nil {
		return 0, 0, false
	}
	matches := re.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return 0, 0, false
	}
	if forward {
		for _, m := range matches {
			if m[0] >= start {
				return m[0], m[1], true
			}
		}
		return matches[0][0], matches[0][1], true
	}
	for i := len(matches) - 1; i >= 0; i-- {
		if matches[i][1] <= start {
			return matches[i][0], matches[i][1], true
		}
	}
	last := matches[len(matches)-1]
	return last[0], last[1], true
}
