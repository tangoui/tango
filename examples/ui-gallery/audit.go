package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/theme"
)

// Accessibility audit of every gallery page (go run ./examples/ui-gallery
// -audit, and TestGalleryAccessibility): each page is rendered in the Ant
// Design, Element Plus and shadcn/ui themes, before and after its Setup
// interaction, and checked with ui.Driver.Audit.

// auditStyle is one theme of the audit.
type auditStyle struct {
	name string
	th   func() *theme.Theme
}

var auditStyles = []auditStyle{
	{"antd", func() *theme.Theme { return theme.AntDesign() }},
	{"element", func() *theme.Theme { return theme.ElementPlus() }},
	{"shadcn", func() *theme.Theme { return theme.Shadcn() }},
}

// auditFinding is an issue found on one page in one theme.
type auditFinding struct {
	page, style, state string
	issue              ui.AuditIssue
}

// auditResult sums up a gallery audit.
type auditResult struct {
	pages    int
	findings []auditFinding
	// contrast holds every check of the six built-in themes, hc those of
	// their HighContrast variants.
	contrast, hc []theme.ContrastCheck
}

// auditGallery audits every page (names containing only, when set).
func auditGallery(only string) auditResult {
	var res auditResult
	for _, st := range auditStyles {
		for _, p := range gallery.Pages() {
			if p.Variant || (only != "" && !strings.Contains(strings.ToLower(p.Name), strings.ToLower(only))) {
				continue
			}
			res.pages++
			w := p.Width
			if w == 0 {
				w = 960
			}
			p := p
			page := func(c *ui.Context) { gallery.Frame(c, p.Title, func() { p.Render(c) }) }
			if p.Bare {
				page = p.Render
			}
			dash, ov = newDash(), newOverlayState()
			d := ui.NewDriver(page, ui.DriverOptions{Width: w, Height: 1400, Theme: white(st.th()), Accessibility: true})
			d.Settle()
			add := func(state string, rep ui.AuditReport) {
				for _, is := range rep.Issues {
					res.findings = append(res.findings, auditFinding{page: p.Name, style: st.name, state: state, issue: is})
				}
			}
			add("initial", d.AuditNoContrast())
			setup := p.Interact
			if setup == nil && st.name == "antd" {
				setup = p.Setup // theme-switching setups only make sense in the default pass
			}
			if p.Name == "Mobile" {
				setup = nil // renders the phone screenshots (files), not an interaction
			}
			if setup != nil {
				setup(d)
				d.Settle()
				add("setup", d.AuditNoContrast())
			}
		}
	}
	for _, th := range theme.BuiltinThemes() {
		res.contrast = append(res.contrast, theme.CheckContrast(th)...)
		res.hc = append(res.hc, theme.CheckContrast(theme.HighContrast(th))...)
	}
	return res
}

// contrastBy returns the checks of cs with status s.
func contrastBy(cs []theme.ContrastCheck, s theme.ContrastStatus) []theme.ContrastCheck {
	var out []theme.ContrastCheck
	for _, c := range cs {
		if c.Status == s {
			out = append(out, c)
		}
	}
	return out
}

// contrastFailures counts the gating contrast findings: required pairs
// failing in the default themes (design defaults and exempt pairs are
// listed, not counted) and any required pair below its minimum in the
// HighContrast themes.
func (r auditResult) contrastFailures() (def, hc int) {
	def = len(contrastBy(r.contrast, theme.ContrastStatusFail))
	hc = len(contrastBy(r.hc, theme.ContrastStatusFail)) + len(contrastBy(r.hc, theme.ContrastStatusDesignDefault))
	return def, hc
}

// count returns the findings of kind k.
func (r auditResult) count(k ui.AuditKind) int {
	n := 0
	for _, f := range r.findings {
		if f.issue.Kind == k {
			n++
		}
	}
	return n
}

// missingNames counts interactive nodes without a name.
func (r auditResult) missingNames() int {
	n := 0
	for _, f := range r.findings {
		if f.issue.Kind == ui.AuditMissingName && f.issue.Interactive {
			n++
		}
	}
	return n
}

// write prints a summary and the findings grouped by page.
func (r auditResult) write(w io.Writer) {
	fmt.Fprintf(w, "accessibility audit: %d page renders (3 styles)\n", r.pages)
	for _, k := range []ui.AuditKind{ui.AuditMissingName, ui.AuditNoRole, ui.AuditNotKeyboard} {
		fmt.Fprintf(w, "  %-14s %4d\n", k, r.count(k))
	}
	fmt.Fprintf(w, "  interactive nodes without a name: %d\n", r.missingNames())
	def, hcFail := r.contrastFailures()
	fmt.Fprintf(w, "  contrast (6 built-in themes): %d fail, %d design-default, %d exempt; high contrast: %d fail\n",
		def, len(contrastBy(r.contrast, theme.ContrastStatusDesignDefault)), len(contrastBy(r.contrast, theme.ContrastStatusExempt)), hcFail)
	byPage := map[string][]auditFinding{}
	var names []string
	for _, f := range r.findings {
		key := f.page
		if _, ok := byPage[key]; !ok {
			names = append(names, key)
		}
		byPage[key] = append(byPage[key], f)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(w, "\n%s\n", n)
		seen := map[string]bool{}
		for _, f := range byPage[n] {
			line := fmt.Sprintf("  %s", f.issue)
			if seen[line] {
				continue
			}
			seen[line] = true
			fmt.Fprintf(w, "%s  [%s/%s]\n", line, f.style, f.state)
		}
	}
	for _, sec := range []struct {
		title string
		cs    []theme.ContrastCheck
	}{
		{"contrast: required pairs failing (default themes)", contrastBy(r.contrast, theme.ContrastStatusFail)},
		{"contrast: design-system defaults below WCAG AA (not failures; HighContrast fixes them)", contrastBy(r.contrast, theme.ContrastStatusDesignDefault)},
		{"contrast: exempt (disabled, decorative)", contrastBy(r.contrast, theme.ContrastStatusExempt)},
		{"contrast: HighContrast variants below their minimum", append(contrastBy(r.hc, theme.ContrastStatusFail), contrastBy(r.hc, theme.ContrastStatusDesignDefault)...)},
	} {
		if len(sec.cs) == 0 {
			continue
		}
		fmt.Fprintf(w, "\n%s\n", sec.title)
		for _, c := range sec.cs {
			fmt.Fprintf(w, "  %s\n", c)
		}
	}
}
