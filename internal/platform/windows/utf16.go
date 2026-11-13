package windows

import (
	"slices"
	"unicode/utf16"
	"unsafe"
)

// Text crossing the Win32 "W" API boundary is UTF-16 ended by a zero code
// unit. The helpers below convert in both directions; the reading side
// works on memory owned by Windows, addressed by a plain uintptr.

// utf16z encodes s for a W function: its UTF-16 code units plus a final
// zero. A zero rune inside s would end the string early on the Windows
// side, so such runes are skipped rather than encoded.
func utf16z(s string) []uint16 {
	units := make([]uint16, 0, len(s)+1)
	for _, r := range s {
		if r != 0 {
			units = utf16.AppendRune(units, r)
		}
	}
	return append(units, 0)
}

// u16 is utf16z for call sites that need a *uint16. Write the call inside
// the argument list of the call that consumes it, so the buffer stays
// referenced for that call.
func u16(s string) *uint16 {
	z := utf16z(s)
	return &z[0]
}

// utf16Units decodes a Go-owned buffer, ignoring everything from its first
// zero unit on (the whole buffer when there is none).
func utf16Units(units []uint16) string {
	if end := slices.Index(units, 0); end >= 0 {
		units = units[:end]
	}
	return string(utf16.Decode(units))
}

// native reinterprets an address of non-Go memory (a COM object, a buffer
// Windows allocated) as an unsafe.Pointer. Reading the bits through a
// pointer to the uintptr keeps vet's unsafeptr check and checkptr quiet;
// it is only sound for memory the Go collector does not manage or for
// objects kept alive by other means.
func native(addr uintptr) unsafe.Pointer {
	bits := (*unsafe.Pointer)(unsafe.Pointer(&addr))
	return *bits
}

// scanUTF16 decodes the text Windows left at p. It stops before the first
// zero unit and, when limit is positive, after limit units.
func scanUTF16(p uintptr, limit int) string {
	if p == 0 {
		return ""
	}
	var units []uint16
	if limit > 0 {
		units = make([]uint16, 0, limit)
	}
	for at := native(p); limit <= 0 || len(units) < limit; at = unsafe.Add(at, 2) {
		u := *(*uint16)(at)
		if u == 0 {
			break
		}
		units = append(units, u)
	}
	return string(utf16.Decode(units))
}

// wstr reads a zero-terminated UTF-16 string at p ("" for 0).
func wstr(p uintptr) string { return scanUTF16(p, 0) }

// wstrN reads at most n units at p, ending early at a zero unit. It is for
// buffers sized by their owner (clipboard memory) that may lack the final
// zero.
func wstrN(p uintptr, n int) string {
	if n <= 0 {
		return ""
	}
	return scanUTF16(p, n)
}
