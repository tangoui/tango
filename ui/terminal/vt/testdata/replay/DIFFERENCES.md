# Differences from xterm.js

The replay reference is headless xterm.js 6.0.0 with
`@xterm/addon-unicode-graphemes` 0.4.0 (Unicode version `15-graphemes`);
see `../../internal/refgen/README.md`. These are the places where
`vt.Terminal` deliberately does something else. The replay samples avoid
them, so `TestReplay` stays exact; unit tests in `terminal_test.go` and
`screen_test.go` pin Tango's behaviour.

Found while building the replay test (2026-11-14). Columns: what xterm.js
does, what Tango does, and why.

## Sequences

| # | Case | xterm.js | Tango | Reason |
|---|---|---|---|---|
| 1 | Cursor position report (`CSI 6 n`) while a wrap is pending, e.g. after filling an 80-column row | Column `cols+1` (`ESC [ y ; 81 R`), outside the screen | Column `cols` (`ESC [ y ; 80 R`) | xterm.js leaks its internal "x == cols" pending-wrap state. The cursor is drawn on, and DEC defines it as being on, the last column; xterm reports that column. A reply outside the screen can confuse programs that use CPR to measure the screen. |
| 2 | SGR color value above 255 (`38;5;300`, `38;2;300;0;0`) | Wraps it modulo 256 (300 becomes 44) | Ignores the color (the rest of the sequence still applies) | The wrap is an artefact of xterm.js's bit packing, not a rule; an out-of-range value is not a color, and guessing one is worse than keeping the current one. |
| 3 | Colon RGB with exactly three values, `38:2:r:g:b` | Reads it as `38:2:cs:r:g` with blue missing: `38:2:1:2:3` is RGB(2,3,0) | RGB(r,g,b); `38:2:cs:r:g:b` (six or more parts) and `38:2::r:g:b` are read as in xterm.js | ITU T.416 puts a color space id first, but programs commonly emit the short form, and several terminals that accept the colon form read it as r:g:b. |
| 4 | Changing the number of columns | Reflows soft-wrapped lines, and wraps lines that no longer fit | Truncates or pads lines; no reflow | Reflow on resize is M2 (spec §6). Replay samples only grow the columns, with no wrapped lines. |

Sequences xterm.js answers that M1 does not implement at all (DA2
`CSI > c`, DECSTBM, the alternate screen and the other M2 features) are
kept out of the samples rather than listed here; they arrive with their
milestones.

## Character widths and grapheme clusters

Tango's widths come from Unicode 15.1 with xterm.js's per-code-point
rules (`width_table.go`, `internal/widthgen`). Comparing `vt.Width` with
the reference for every code point led to three fixes in Tango, which
now matches it: U+00AD SOFT HYPHEN is width 1, U+2028/U+2029 (and
unassigned U+2065) are width 1, and the regional indicators
U+1F1E6–U+1F1FF are width 1 each. What remains:

| # | Case | xterm.js (graphemes addon) | Tango | Reason |
|---|---|---|---|---|
| 5 | Grapheme clusters that join more than zero-width marks: ZWJ sequences, emoji modifiers, VS16 after a narrow character, regional indicator pairs, Hangul jamo vowels and finals after a leading consonant, spacing marks with Grapheme_Extend | One cell per cluster, as wide as its widest part, or 2 when VS16 or a flag asks for emoji presentation. `👨‍👩‍👧` 2 columns, `👍🏽` 2, `❤️` (U+2764 U+FE0F) 2, `🇨🇳` 2 (one wide cell), `ಕೂ` (U+0C95 U+0CC2) 1, `ｶﾞ` (U+FF76 U+FF9E) 1 | Per code point: zero-width marks join the cell before them, everything else takes its own width. `👨‍👩‍👧` 6 columns, `👍🏽` 4, `❤️` 1, `🇨🇳` 2 (two narrow cells), `ಕೂ` 2, `ｶﾞ` 2 | M1 has no Grapheme_Cluster_Break data or cluster segmentation. Spec §4.1 wants clusters to share a cell group like xterm.js; this is the largest open gap and needs its own task (widthgen would also need GraphemeBreakProperty.txt and emoji-sequence data). Affected spacing marks include U+09BE, U+09D7, U+0B3E, U+0B57, U+0BBE, U+0BD7, U+0CC2, U+0CD5–U+0CD6, U+0D3E, U+0D4E, U+0D57, U+0DCF, U+0DDF, U+1734, U+1B35, U+302E–U+302F, U+FF9E–U+FF9F, U+111C2–U+111C3, U+1133E, U+11357, U+114B0, U+114BD, U+115AF, U+11930, U+1193F, U+11941, U+11A3A, U+11A84–U+11A89, U+11D46, U+1D165, U+1D16E–U+1D172 and the modifiers U+1F3FB–U+1F3FF. |
| 6 | Format characters whose Grapheme_Cluster_Break is Control: U+061C, U+180E, U+200B, U+200E–U+200F, U+202A–U+202E, U+2060–U+2064, U+2066–U+206F, U+FEFF after text, U+FFF9–U+FFFB, U+13430–U+13440, U+1BCA0–U+1BCA3, U+1D173–U+1D17A, U+E0001 | A cell of width 1 of their own (the graphemes provider gives every non-joining code point at least width 1) | Width 0, invisible | They are invisible format controls; xterm.js's own unicode11 provider, wcwidth and other terminals give them no cell. A zero-width space taking a column breaks alignment of text that contains it. |
| 7 | Combining mark with no character before it on the row | A cell of width 1 holding just the mark | Dropped | Nothing to attach it to; a mark on its own cell would also differ from the unicode11 provider. |
| 8 | Characters assigned or changed in Unicode 14.0–15.1 | Widths from the addon's older tables: new wide characters (U+2FFC–U+2FFF, U+31EF, U+1AFF0–U+1AFFE, U+1B11F–U+1B122, U+1B132, U+1B155, U+1F6DC–U+1F6DF, U+1F7F0, U+1F979, U+1F9CC, U+1FA75–U+1FA88, U+1FAA9–U+1FAF8) are 1; new combining marks and format characters (U+0890–U+0891, U+0898–U+089F, U+08CA–U+08D2, U+0C3C, U+0ECE, U+180F, U+1AC1–U+1ACE, U+1DFA, U+10EFD–U+10EFF, U+10F82–U+10F85, U+11070, U+11073–U+11074, U+110C2, U+11241, U+11F00–U+11F01, U+11F36–U+11F42, U+13447–U+13455, U+1CF00–U+1CF46, U+1E08F, U+1E2AE, U+1E4EC–U+1E4EF) are 1 | Unicode 15.1: wide characters 2, marks 0 | Ruling R4: Tango follows Unicode 15.1 data with xterm.js's rules; the addon's data is older than its name suggests. |

To re-check widths after changing `widthgen`, compare `vt.Width` with the
reference's `unicodeService.wcwidth` and `charProperties(cp, 0)` for every
code point (a throwaway script; the list above is its output, grouped).
