// Package diffview is the diff editor of ui/editor: two editors side by
// side (or one inline view) showing the differences between an original
// and a modified text, computed by ui/editor/diff in the background.
//
// The package is built only on the public API of ui/editor (decorations,
// view zones, hidden areas, scrolling), like Monaco's diff editor on top
// of its code editor; see docs/specs/diff-view.md. This first stage holds
// the view-independent logic: the alignment of the two sides with padding
// zones and block-anchored scroll synchronisation, mapping a stale diff
// through edits while the next one is computed, the folding state of
// unchanged regions, the debounced background computation, block revert
// and screen-reader announcements. The view itself follows once the
// editor exposes those APIs.
//
// Experimental: this package's API may change in any 0.x release (see
// docs/release.md, "API 稳定性").
package diffview
