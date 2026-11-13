package mobilecanvas

import "testing"

// Test strings. Offsets are listed per rune: (rune index, UTF-16 offset).
const (
	sASCII    = "abc"
	sCJK      = "中文字"   // BMP: 1 unit per rune
	sEmoji    = "a😀b"   // U+1F600: a surrogate pair
	sCombine  = "éx"   // e + combining acute: 2 runes, 2 units
	sFamily   = "👨‍👩‍👧" // ZWJ family: 5 runes, 8 units
	sFlags    = "🇨🇳🇺🇸"  // 2 flags, 4 regional indicators, 8 units
	sThumbs   = "👍🏽"    // thumbs up + skin tone: 2 runes, 4 units
	sKeycap   = "1️⃣"   // keycap one: 3 runes, 3 units
	sHeart    = "❤️"    // BMP pictograph + VS16
	sCoder    = "👩🏽‍💻"  // woman, skin tone, ZWJ, laptop: 4 runes, 7 units
	sCRLF     = "a\r\nb"
	sBadUTF8  = "a\xffb"
	sHangulJm = "가" // conjoining jamo (left to the IME)
)

func TestUTF16Len(t *testing.T) {
	tests := []struct {
		s    string
		want int
	}{
		{"", 0}, {sASCII, 3}, {sCJK, 3}, {sEmoji, 4}, {sCombine, 3},
		{sFamily, 8}, {sFlags, 8}, {sThumbs, 4}, {sKeycap, 3}, {sCoder, 7}, {sBadUTF8, 3},
	}
	for _, tt := range tests {
		if got := UTF16Len(tt.s); got != tt.want {
			t.Errorf("UTF16Len(%q) = %d, want %d", tt.s, got, tt.want)
		}
	}
}

func TestOffsets(t *testing.T) {
	tests := []struct {
		s     string
		runes int
		u16   int
		bytes int
	}{
		{sASCII, 0, 0, 0},
		{sASCII, 2, 2, 2},
		{sASCII, 3, 3, 3},
		{sCJK, 1, 1, 3},
		{sCJK, 3, 3, 9},
		{sEmoji, 1, 1, 1},
		{sEmoji, 2, 3, 5},
		{sEmoji, 3, 4, 6},
		{sCombine, 1, 1, 1},
		{sCombine, 2, 2, 3},
		{sFamily, 2, 3, 7},
		{sFamily, 5, 8, 18},
		{sBadUTF8, 2, 2, 2},
	}
	for _, tt := range tests {
		if got := RuneToUTF16(tt.s, tt.runes); got != tt.u16 {
			t.Errorf("RuneToUTF16(%q, %d) = %d, want %d", tt.s, tt.runes, got, tt.u16)
		}
		if got := UTF16ToRune(tt.s, tt.u16); got != tt.runes {
			t.Errorf("UTF16ToRune(%q, %d) = %d, want %d", tt.s, tt.u16, got, tt.runes)
		}
		if got := UTF16ToByte(tt.s, tt.u16); got != tt.bytes {
			t.Errorf("UTF16ToByte(%q, %d) = %d, want %d", tt.s, tt.u16, got, tt.bytes)
		}
		if got := RuneToByte(tt.s, tt.runes); got != tt.bytes {
			t.Errorf("RuneToByte(%q, %d) = %d, want %d", tt.s, tt.runes, got, tt.bytes)
		}
	}
}

func TestOffsetsClampAndSplitPairs(t *testing.T) {
	tests := []struct {
		name string
		got  int
		want int
	}{
		{"inside pair rounds down (rune)", UTF16ToRune(sEmoji, 2), 1},
		{"inside pair rounds down (byte)", UTF16ToByte(sEmoji, 2), 1},
		{"negative utf16", UTF16ToRune(sEmoji, -3), 0},
		{"past end utf16", UTF16ToRune(sEmoji, 99), 3},
		{"past end byte", UTF16ToByte(sEmoji, 99), 6},
		{"negative runes", RuneToUTF16(sEmoji, -1), 0},
		{"past end runes", RuneToUTF16(sEmoji, 99), 4},
		{"past end rune bytes", RuneToByte(sEmoji, 99), 6},
		{"empty", UTF16ToRune("", 5), 0},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: %d, want %d", tt.name, tt.got, tt.want)
		}
	}
}

func TestSliceUTF16(t *testing.T) {
	tests := []struct {
		s          string
		start, end int
		want       string
	}{
		{sEmoji, 1, 3, "😀"},
		{sEmoji, 3, 1, "😀"},  // reversed bounds
		{sEmoji, 2, 4, "😀b"}, // start inside the pair rounds down
		{sEmoji, -5, 99, sEmoji},
		{sFamily, 0, 2, "👨"},
		{sCombine, 0, 2, "é"},
	}
	for _, tt := range tests {
		if got := SliceUTF16(tt.s, tt.start, tt.end); got != tt.want {
			t.Errorf("SliceUTF16(%q, %d, %d) = %q, want %q", tt.s, tt.start, tt.end, got, tt.want)
		}
	}
}

func TestBoundaries(t *testing.T) {
	tests := []struct {
		s    string
		i    int
		prev int // PrevBoundary(s, i)
	}{
		{"", 0, 0},
		{sASCII, 3, 2},
		{sASCII, 0, 0},
		{sASCII, 99, 2},
		{sCJK, 2, 1},
		{sEmoji, 2, 1},
		{sCombine, 2, 0}, // e + acute go together
		{sCombine, 3, 2},
		{sFamily, 5, 0}, // the whole family
		{sFlags, 4, 2},  // the last flag
		{sFlags, 2, 0},
		{sFlags, 3, 2}, // a lone regional indicator after a flag
		{sThumbs, 2, 0},
		{sKeycap, 3, 0},
		{sHeart, 2, 0},
		{sCoder, 4, 0},
		{sCRLF, 3, 1}, // CR LF together
		{sCRLF, 4, 3},
		{"x‍", 2, 0},                // trailing ZWJ extends
		{"a‍👩", 3, 2},               // ZWJ after a non-pictograph does not join
		{"中" + sFamily, 6, 1},       // only the family
		{sHangulJm, 2, 1},           // jamo: one rune at a time
		{"́", 1, 0},                 // lone combining mark
		{"ab́̂", 4, 1},              // stacked marks
		{"कि", 2, 0},                // Devanagari ka + vowel sign i (spacing mark)
		{"😀" + sFamily + "😀", 7, 6}, // trailing plain emoji
	}
	for _, tt := range tests {
		if got := PrevBoundary(tt.s, tt.i); got != tt.prev {
			t.Errorf("PrevBoundary(%q, %d) = %d, want %d", tt.s, tt.i, got, tt.prev)
		}
	}
	next := []struct {
		s    string
		i    int
		want int
	}{
		{sASCII, 0, 1},
		{sASCII, 3, 3},
		{sASCII, -1, 1},
		{sCombine, 0, 2},
		{sFamily, 0, 5},
		{sFlags, 0, 2},
		{sFlags, 2, 4},
		{"a" + sCoder + "b", 1, 5},
		{"a" + sCoder + "b", 2, 5}, // inside the sequence: its end
	}
	for _, tt := range next {
		if got := NextBoundary(tt.s, tt.i); got != tt.want {
			t.Errorf("NextBoundary(%q, %d) = %d, want %d", tt.s, tt.i, got, tt.want)
		}
	}
}
