package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

// AuditKind classifies an accessibility audit finding.
//
// Append-only: never reorder or insert; the zero value stays AuditMissingName.
type AuditKind uint8

const (
	// AuditMissingName: an interactive (or image / dialog) node without an
	// accessible name. Icon-only controls need an A11yLabel / label.
	AuditMissingName AuditKind = iota
	// AuditNoRole: a pointer target that declares no role; it is exposed
	// as a generic button so it stays reachable, but the widget should
	// annotate it.
	AuditNoRole
	// AuditNotKeyboard: an interactive widget that Tab cannot reach and
	// that is not an item of a keyboard-operated composite (list, menu,
	// tree, grid, popup of a focusable trigger).
	AuditNotKeyboard
	// AuditContrast: a theme token pair below its WCAG contrast minimum.
	AuditContrast
	// AuditUnmirroredIcon (warning): a directional icon (an arrow, a
	// chevron, text lines) drawn without mirroring in a right-to-left
	// scope, so it points against the reading direction. Draw it with the
	// direction-aware helper (Context.paintIcon) instead of icon.Draw.
	AuditUnmirroredIcon
)

func (k AuditKind) String() string {
	return [...]string{"missing-name", "no-role", "not-keyboard", "contrast", "unmirrored-icon"}[k]
}

// AuditIssue is one finding of Driver.Audit.
type AuditIssue struct {
	Kind        AuditKind
	Role        a11y.Role
	Name        string // accessible name (empty for missing-name)
	Label       string // the widget's internal label (to locate it)
	Rect        geom.Rect
	Interactive bool   // the node is a control (counts toward the zero-missing-name goal)
	Detail      string // contrast pair, reason
}

func (i AuditIssue) String() string {
	if i.Kind == AuditContrast {
		return fmt.Sprintf("%s: %s", i.Kind, i.Detail)
	}
	return fmt.Sprintf("%s: %s %q label=%q at (%.0f,%.0f %.0fx%.0f) %s", i.Kind, i.Role, i.Name, i.Label, i.Rect.X, i.Rect.Y, i.Rect.W, i.Rect.H, i.Detail)
}

// AuditReport is the result of Driver.Audit.
type AuditReport struct {
	Issues      []AuditIssue
	Nodes       int // semantic nodes in the tree
	Interactive int // interactive nodes in the tree
}

// Count returns the number of issues of kind k.
func (r AuditReport) Count(k AuditKind) int {
	n := 0
	for _, i := range r.Issues {
		if i.Kind == k {
			n++
		}
	}
	return n
}

// MissingInteractiveNames counts interactive nodes without a name (the
// gallery gate keeps it at zero).
func (r AuditReport) MissingInteractiveNames() int {
	n := 0
	for _, i := range r.Issues {
		if i.Kind == AuditMissingName && i.Interactive {
			n++
		}
	}
	return n
}

// String lists the issues, one per line.
func (r AuditReport) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d nodes, %d interactive, %d issues\n", r.Nodes, r.Interactive, len(r.Issues))
	for _, i := range r.Issues {
		b.WriteString("  ")
		b.WriteString(i.String())
		b.WriteByte('\n')
	}
	return b.String()
}

// Audit enables accessibility, settles and checks the current frame:
// interactive nodes without an accessible name, pointer targets without a
// role, controls the keyboard cannot reach, and contrast problems of the
// current theme (see theme.CheckContrast), and in right-to-left scopes
// directional icons drawn without mirroring. Drive the page into the
// state to check (open popups with Click / Key) before calling it.
func (d *Driver) Audit() AuditReport { return d.audit(true) }

// AuditNoContrast is Audit without the theme contrast checks (when several
// pages share one theme and contrast is reported once).
func (d *Driver) AuditNoContrast() AuditReport { return d.audit(false) }

func (d *Driver) audit(contrast bool) AuditReport {
	rec, done := d.recordIcons()
	d.EnableAccessibility() // settles: runs at least one frame with the recorder
	done()
	rep := d.r.c.audit(d.r.a11yTree, contrast)
	icons, _ := d.r.c.iconViolations(rec)
	for _, v := range icons {
		rep.Issues = append(rep.Issues, AuditIssue{Kind: AuditUnmirroredIcon, Label: v.Label, Rect: v.Rect,
			Detail: fmt.Sprintf("directional icon %q drawn unmirrored in a right-to-left scope", v.Icon)})
	}
	return rep
}

func (c *Context) audit(t *a11y.Tree, contrast bool) AuditReport {
	var rep AuditReport
	rep.Nodes = t.Len()
	for _, n := range t.Nodes() {
		inter := n.Role.Interactive() || n.Role == a11y.RoleCombobox
		if inter {
			rep.Interactive++
		}
		needs := inter || n.Role == a11y.RoleImage || n.Role == a11y.RoleDialog || n.Role == a11y.RoleAlertDialog ||
			n.Role == a11y.RoleFigure || n.Role == a11y.RoleProgressBar || n.Role == a11y.RoleTabPanel
		if needs && strings.TrimSpace(n.Name) == "" {
			lbl := ""
			if un := c.a11yNodes[ID(n.ID)]; un != nil {
				lbl = un.label
			}
			rep.Issues = append(rep.Issues, AuditIssue{Kind: AuditMissingName, Role: n.Role, Label: lbl, Interactive: inter,
				Rect: geom.Rect{X: n.Bounds.X, Y: n.Bounds.Y, W: n.Bounds.W, H: n.Bounds.H}})
		}
	}
	modal := c.modalLayer()
	for i, root := range c.layerRoots() {
		if i < modal {
			continue
		}
		reach := false
		if i > 0 {
			// Popup content is reachable through its focusable trigger
			// (Select, Dropdown, DatePicker keep focus and own the keys).
			if a := c.overlays[i-1].opts.Anchor; a != 0 && c.focusOrder.Contains(a) {
				reach = true
			}
		}
		c.auditNode(&rep, root, reach)
	}
	if contrast {
		for _, ck := range theme.ContrastIssues(c.Theme) {
			rep.Issues = append(rep.Issues, AuditIssue{Kind: AuditContrast, Detail: ck.String()})
		}
	}
	sort.SliceStable(rep.Issues, func(i, j int) bool { return rep.Issues[i].Kind < rep.Issues[j].Kind })
	return rep
}

// auditNode walks the widget tree for role and keyboard findings. reach
// is true inside a focusable composite (its keys move between items).
func (c *Context) auditNode(rep *AuditReport, n *node, reach bool) {
	if _, painted := c.rects[n.id]; !painted || n.aHidden {
		return
	}
	disabled := n.disabled || n.cursor == input.CursorNotAllowed
	pointer := n.hit && n.cursor == input.CursorPointer && !n.blocks
	if pointer && !disabled {
		r := c.rects[n.id]
		if interactiveUnannotated(n) {
			rep.Issues = append(rep.Issues, AuditIssue{Kind: AuditNoRole, Label: n.label, Rect: r})
		}
		tab := n.focus && !n.noTab
		if !tab && !reach && !n.aKbd {
			rep.Issues = append(rep.Issues, AuditIssue{Kind: AuditNotKeyboard, Role: n.role, Name: n.aName, Label: n.label, Rect: r})
		}
	}
	if n.focus && !n.noTab && !disabled {
		reach = true
	}
	for _, k := range n.kids {
		c.auditNode(rep, k, reach)
	}
}

// AuditTheme reports the contrast findings of th as audit issues.
func AuditTheme(th *theme.Theme) []AuditIssue {
	var out []AuditIssue
	for _, ck := range theme.ContrastIssues(th) {
		out = append(out, AuditIssue{Kind: AuditContrast, Detail: ck.String()})
	}
	return out
}
