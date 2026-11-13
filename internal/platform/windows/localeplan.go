package windows

// splitMultiSZ splits a double-NUL-terminated list of UTF-16 strings, the
// format of GetUserPreferredUILanguages.
func splitMultiSZ(buf []uint16) []string {
	var out []string
	start := 0
	for i, u := range buf {
		if u != 0 {
			continue
		}
		if i == start {
			break
		}
		out = append(out, utf16Units(buf[start:i]))
		start = i + 1
	}
	return out
}
