package windows

import (
	"encoding/binary"
	"encoding/hex"
)

// GUID is a COM interface or class identifier (same layout as Win32 GUID).
type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// guidDashes are the positions of the separators in the 36-character
// registry form 8-4-4-4-12.
var guidDashes = [...]int{8, 13, 18, 23}

// parseGUID parses the registry form "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
// optionally in braces. The text spells the first three fields as numbers
// (most significant digit first) and the last 8 bytes in memory order.
func parseGUID(s string) (GUID, bool) {
	if len(s) == 38 && s[0] == '{' && s[37] == '}' {
		s = s[1:37]
	}
	if len(s) != 36 {
		return GUID{}, false
	}
	digits := make([]byte, 0, 32)
	next := 0
	for _, d := range guidDashes {
		if s[d] != '-' {
			return GUID{}, false
		}
		digits = append(digits, s[next:d]...)
		next = d + 1
	}
	digits = append(digits, s[next:]...)

	var raw [16]byte
	if _, err := hex.Decode(raw[:], digits); err != nil {
		return GUID{}, false
	}
	g := GUID{
		Data1: binary.BigEndian.Uint32(raw[0:4]),
		Data2: binary.BigEndian.Uint16(raw[4:6]),
		Data3: binary.BigEndian.Uint16(raw[6:8]),
	}
	copy(g.Data4[:], raw[8:])
	return g, true
}
