package main

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor"
	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/layout"
)

type searchPanel struct {
	open          bool
	query         string
	replacement   string
	caseSensitive bool
	status        string
}

func (p *searchPanel) render(c *ui.Context, target *editor.State) bool {
	if !p.open || target == nil {
		return false
	}
	ui.Modal(c, &p.open, "Find and Replace").Width(640).NoFooter().Body(func() {
		ui.Input(c, &p.query).Placeholder("Find").A11yLabel("Find text").Width(600)
		ui.Input(c, &p.replacement).Placeholder("Replace with").A11yLabel("Replacement text").Width(600)
		ui.Checkbox(c, "Case sensitive", &p.caseSensitive)
		ui.Row(c, func() {
			if ui.Button(c, "Find Previous").Clicked() {
				p.find(target, false)
			}
			if ui.Button(c, "Find Next").Clicked() {
				p.find(target, true)
			}
			if ui.Button(c, "Replace").Clicked() {
				p.replace(target)
			}
			if ui.Button(c, "Replace All").Clicked() {
				count, err := replaceAll(target, p.query, p.replacement, p.caseSensitive)
				if err != nil {
					p.status = err.Error()
				} else {
					p.status = replaceStatus(count)
				}
			}
		}, layout.Gap(8))
		if p.status != "" {
			ui.Text(c, p.status)
		}
	})
	return true
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
