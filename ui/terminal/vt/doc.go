// Package vt is the terminal emulator core of ui/terminal: it turns the
// bytes a program writes into actions and, in later milestones, screen
// state. It does not depend on any ui package and can be tested on its own.
//
// [Parser] implements the DEC ANSI parser state machine described by Paul
// Williams (vt100.net/emu/dec_ansi_parser), extended with UTF-8 decoding,
// ':' sub-parameters and BEL-terminated OSC strings. Runs of printable ASCII
// bypass the state machine and reach [Handler.PrintASCII] in one call.
//
// Experimental: this package's API may change in any 0.x release (see
// docs/release.md, "API 稳定性").
package vt
