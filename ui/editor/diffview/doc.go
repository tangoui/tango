// Package diffview is the diff editor of ui/editor: two editors side by
// side (or one inline view) showing the differences between an original
// and a modified text, computed by ui/editor/diff in the background.
//
// New makes a State of two Models, View shows it every frame:
//
//	d := diffview.New(original, modified, th)
//	defer d.Close()
//	...
//	diffview.View(c, d)
//
// The package is built only on the public API of ui/editor (decorations,
// view zones, hidden areas, scrolling), like Monaco's diff editor on top
// of its code editor; see docs/specs/diff-view.md. The sides are aligned
// with blank view zones (whole rows: both editors share a line height) and
// scroll together block by block; the diff is recomputed after a debounce
// on every edit, the previous result being mapped through the edits
// meanwhile; unchanged regions can be folded behind a placeholder;
// changes can be reverted one by one or handed to the host's actions
// (ViewW.HunkActions); Alt+F5 and Shift+Alt+F5 move between changes and
// announce them to screen readers.
//
// Experimental: this package's API may change in any 0.x release (see
// docs/release.md, "API 稳定性").
package diffview
