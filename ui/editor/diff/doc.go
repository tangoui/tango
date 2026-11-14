// Package diff computes line and character level differences between two
// versions of a text, the algorithm layer behind a diff editor.
//
// The design follows Monaco's DefaultLinesDiffComputer (monaco-editor
// 0.56): lines are hashed after trimming, aligned with a dynamic-programming
// LCS for small inputs or Myers' O(ND) algorithm for large ones, cleaned up
// by heuristics (sliding changes to indentation and word boundaries, joining
// changes split by trivial matches), and every changed block is refined to
// character ranges. Moved blocks can optionally be detected. A Timeout or a
// cancelled context stops the work early; the result is then still a valid
// (coarser) diff and TimedOut is set.
//
// Lines and columns are 0-based. Columns are UTF-8 byte columns, like
// buffer.Pos; character-level refinement works on runes so ranges never
// split a UTF-8 sequence. Line ranges are half-open.
//
// Lines diffs two slices of lines (see SplitLines); Snapshots diffs two
// buffer.Snapshots. The result also answers the questions a diff view asks:
// which unchanged regions can be folded (UnchangedRegions) and where the
// previous or next change is (Result.Prev, Result.Next). The functions
// keep no state and may run on any goroutine, typically a background one
// with a context that is cancelled when either text changes again.
//
// Experimental: this package's API may change in any 0.x release (see
// docs/release.md, "API 稳定性").
package diff
