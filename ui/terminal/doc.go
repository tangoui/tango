// Package terminal is a terminal component: it connects a [Conn], the
// program end of a terminal such as a pseudo-terminal, to the emulator in
// ui/terminal/vt.
//
// [Terminal] owns the emulator and a read loop that feeds the program's
// output into it. The view locks the emulator with [Terminal.Lock] to
// draw it, and sends keys and pastes with [Terminal.Send].
//
// Experimental: this package's API may change in any 0.x release (see docs/release.md, "API 稳定性").
package terminal
