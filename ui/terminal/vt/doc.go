// Package vt is the terminal emulator core of ui/terminal: it turns the
// bytes a program writes into screen state. It does not depend on any ui
// package and can be tested on its own.
//
// [Terminal] is the emulator: it feeds output through a [Parser] and
// applies the result to a [Screen], tracking the cursor, the modes, the
// title and which rows changed since the view last drew them.
//
// [Parser] implements the DEC ANSI parser state machine described by Paul
// Williams (vt100.net/emu/dec_ansi_parser), extended with UTF-8 decoding,
// ':' sub-parameters and BEL-terminated OSC strings. Runs of printable ASCII
// bypass the state machine and reach [Handler.PrintASCII] in one call.
//
// [Screen] holds the cell grid: the active rows plus a bounded scrollback,
// kept in one ring buffer of [Line] values so scrolling never copies
// cells. Each [Cell] stores a code point, a width and a 16-bit index into
// the screen's [StyleTable]. [Width] gives a character's cell width using
// xterm.js's rules and tables generated from the Unicode data files.
//
// Experimental: this package's API may change in any 0.x release (see
// docs/release.md, "API 稳定性").
package vt
