#!/bin/sh
# samples.sh writes the hand-made replay samples (the ones not recorded
# from real programs) into ../../testdata/replay. See README.md.
set -eu
cd "$(dirname "$0")/../../testdata/replay"

E=$(printf '\033')

# sgr: every SGR attribute, the 16, 256 and direct colors in both the ';'
# and ':' forms, resets, and erasing with a background color.
{
	for a in 1 2 3 4 5 6 7 8 9 21 53; do printf '%s[%smA%s%s[0m ' "$E" "$a" "$a" "$E"; done
	printf '\r\n%s[1;2;3;4;5;7;8;9mall%s[22mB%s[23mI%s[24mU%s[25mK%s[27mR%s[28mH%s[29mS%s[m\r\n' \
		"$E" "$E" "$E" "$E" "$E" "$E" "$E" "$E" "$E"
	for c in 30 31 32 33 34 35 36 37 39 90 91 92 93 94 95 96 97; do printf '%s[%smf%s' "$E" "$c" "$c"; done
	printf '%s[m\r\n' "$E"
	for c in 40 41 42 43 44 45 46 47 49 100 101 102 103 104 105 106 107; do printf '%s[%smb%s' "$E" "$c" "$c"; done
	printf '%s[m\r\n' "$E"
	n=0
	while [ $n -lt 256 ]; do printf '%s[38;5;%dm#' "$E" $n; n=$((n + 1)); done
	printf '%s[m\r\n' "$E"
	n=0
	while [ $n -lt 256 ]; do printf '%s[48;5;%dm ' "$E" $n; n=$((n + 7)); done
	printf '%s[m\r\n' "$E"
	printf '%s[38;2;255;128;0mrgb%s[48;2;0;64;128mbg%s[38:2::1:2:3mcolon%s[38:2:0:4:5:6mcs0' "$E" "$E" "$E" "$E"
	printf '%s[38:5:200mi200%s[48:5:17mb17%s[39mfd%s[49mbd%s[m\r\n' "$E" "$E" "$E" "$E" "$E"
	printf '%s[4:3mcurly%s[4:0mnone%s[58;2;9;9;9mul%s[59m%s[4;58:5:1mulc%s[m ' "$E" "$E" "$E" "$E" "$E" "$E"
	printf '%s[38;5mbad%s[38;2;1;2mshort%s[1;38;5;9;48;5;4mmix%s[m\r\n' "$E" "$E" "$E" "$E"
	printf '%s[44mbluebg%s[K%s[m\r\n' "$E" "$E" "$E"
	printf '%s[41;1mwxyz%s[2D%s[X%s[m\r\n' "$E" "$E" "$E" "$E"
	printf '%s[7;32minverse green%s[0;8minvisible%s[0;1;2mbold+faint%s[0m\r\n' "$E" "$E" "$E" "$E"
	# Unknown color kinds skip the kind and four more parameters (five
	# for kind 0); values above 255 wrap.
	printf '%s[38;3;1;2;3;4;5;3mk3%s[m %s[38;0;1;2;3;4;5;3mk0%s[m %s[38;3;1;2;3;4;1mk3b%s[m ' "$E" "$E" "$E" "$E" "$E" "$E"
	printf '%s[48;7;1;2;3;4;5;4mk7%s[m %s[38:3:1;1mc3%s[m ' "$E" "$E" "$E" "$E"
	printf '%s[38;5;300mi300%s[48;5;99999mb99999%s[38;2;256;257;511mrgb%s[38:2::256:1:2mcolon%s[38:5:300mc300%s[m\r\n' "$E" "$E" "$E" "$E" "$E" "$E"
} > sgr.vt

# wide: wide characters, single-code-point emoji and combining marks,
# including wide characters at and across the right margin and wide
# characters split by overwriting, insertion and deletion.
{
	printf '中文字符测试 한국어 カタカナ ｶﾀｶﾅ ＡＢＣ 😀🎉🚀✅⌚ ❤ ☺\r\n'
	printf 'e\314\201 a\314\210 n\314\203 a\314\201\314\202 中\314\201 x\r\n'
	printf '%s[3;79H中x' "$E"
	printf '%s[5;80H中y' "$E"
	printf '%s[7;1H中文字%s[7;2Ha' "$E" "$E"
	printf '%s[8;1H中文字%s[8;3Hb' "$E" "$E"
	printf '%s[9;1H中文字符%s[9;2H%s[P' "$E" "$E" "$E"
	printf '%s[10;1H中文字符%s[10;4H%s[2@' "$E" "$E" "$E"
	printf '%s[11;1H中文字符%s[11;4H%s[K' "$E" "$E" "$E"
	printf '%s[12;1H中文字符%s[12;3H%s[1K' "$E" "$E" "$E"
	printf '%s[13;1H中文字符%s[13;2H%s[2X' "$E" "$E" "$E"
	printf '%s[14;1H%s[76C中文字符尾' "$E" "$E"
	printf '%s[?7l%s[17;78H中文字%s[?7h' "$E" "$E" "$E"
	printf '%s[18;1H%s[78C中' "$E" "$E"
	printf '%s[19;1Hstart\314\201 \314\201' "$E"
	printf '%s[20;1H\357\273\277bom\302\255shy' "$E"
} > wide.vt

# marks: zero-width code points join the character printed just before
# them; after anything else (a control, an escape or control sequence,
# cursor movement) they take a cell of their own, the emoji presentation
# selector two. A byte order mark is dropped.
{
	printf '\314\201a\314\201\314\202\r\n'
	printf 'ab%s[31m\314\201c%s[m\r\n' "$E" "$E"
	printf 'ab\a\314\201c\r\n'
	printf 'abc%s[2D\314\201\r\n' "$E"
	printf '%s[10G\342\200\213x%s[20G\342\200\215y%s[30G\357\270\217z\r\n' "$E" "$E" "$E"
	printf 'a\357\273\277b\357\273\277\r\n'
	printf '%s[6;80Hz\314\201w\r\n' "$E"
	printf '%s[8;80Hz\r\n\314\201v' "$E"
	printf '%s[10;80H%s[m\357\270\217' "$E" "$E"
} > marks.vt

# cursor: cursor movement, erasing, insertion and deletion over a filled
# screen.
{
	i=0
	while [ $i -lt 24 ]; do
		printf 'line%02d-abcdefghijklmnopqrstuvwxyz-0123456789-ABCDEFGHIJKLMNOPQRSTUVWXYZ-%02d' $i $i
		[ $i -lt 23 ] && printf '\r\n'
		i=$((i + 1))
	done
	printf '%s[2;5H%s[K' "$E" "$E"        # EL 0
	printf '%s[3;10H%s[1K' "$E" "$E"      # EL 1
	printf '%s[4;10H%s[2K' "$E" "$E"      # EL 2
	printf '%s[5;10H%s[5X' "$E" "$E"      # ECH
	printf '%s[6;10H%s[3P' "$E" "$E"      # DCH
	printf '%s[7;10H%s[4@' "$E" "$E"      # ICH
	printf '%s[8;3H%s[2L' "$E" "$E"       # IL
	printf '%s[12;7H%s[M' "$E" "$E"       # DL
	printf '%s[14;1H%s[3A+%s[2B-%s[5C>%s[3D<' "$E" "$E" "$E" "$E" "$E"
	printf '%s[15;1H%s[2E*%s[F^%s[40G|%s[20d!' "$E" "$E" "$E" "$E" "$E"
	printf '%s[16;1H%s[3a(%s[2e)' "$E" "$E" "$E"
	printf '%s[17;1H\ta\tb\t\tc\b\bd%s[999C\tE' "$E" "$E"
	printf '%s[18;5H%s7%s[1;1HS%s8R' "$E" "$E" "$E" "$E"
	printf '%s[0;0HO%s[9999;9999HZ' "$E" "$E"
	printf '%s[1;1H%sM%sM' "$E" "$E" "$E" # RI at the top scrolls down
	printf '%s[2S%s[T' "$E" "$E"          # SU, SD
	printf '%s[24;1H%sD%sE' "$E" "$E" "$E" # IND, NEL at the bottom
	printf '%s[24;70Hend\r\n\r\nlast%s[22;79Hw%s[6n' "$E" "$E" "$E"
} > cursor.vt

# erase_display: ED 1 and ED 0 in the middle of a filled screen, with a
# background color.
{
	i=0
	while [ $i -lt 24 ]; do
		printf 'line%02d-abcdefghijklmnopqrstuvwxyz-0123456789-ABCDEFGHIJKLMNOPQRSTUVWXYZ-%02d' $i $i
		[ $i -lt 23 ] && printf '\r\n'
		i=$((i + 1))
	done
	printf '%s[8;30H%s[1J' "$E" "$E"
	printf '%s[16;40H%s[44m%s[0J%s[m' "$E" "$E" "$E" "$E"
	printf '%s[10;10Hmid' "$E"
} > erase_display.vt

# Device attribute and status replies.
printf '%s[c%s[0c%s[1c%s[5n%s[6n%s[3;7H%s[6n' "$E" "$E" "$E" "$E" "$E" "$E" "$E" > da1.vt

# Erasing while a wrap is pending. (A cursor position report there
# differs from xterm.js on purpose; see DIFFERENCES.md.)
printf '0123456789ABCDEFGHIJ%s[K\r\nabcdefghij%s[1K\b%s[6n' "$E" "$E" "$E" > erase_pending.vt
echo 10x4 > erase_pending.size

# The cursor column after IL and DL.
printf 'aaaaaaaaaa\r\nbbbbbbbbbb\r\ncccccccccc\r\ndddddddddd\r\neeeeeeeeee%s[2;5H%s[LX%s[4;6H%s[MY%s[5;10Hz%s[LW' \
	"$E" "$E" "$E" "$E" "$E" "$E" > il_dl.vt
echo 10x5 > il_dl.size

# Resizes: .ops files (see README.md).
cat > resize_shrink.ops <<'EOF'
[{"write": "r1\r\nr2\r\nr3\r\nr4\r\nr5\r\nr6\r\nr7"}, {"resize": [10, 3]}, {"write": "Z"}]
EOF
echo 10x5 > resize_shrink.size
cat > resize_grow.ops <<'EOF'
[{"write": "r1\r\nr2\r\nr3\r\nr4\r\nr5\r\nr6\r\nr7\r\nr8"}, {"resize": [10, 8]}, {"write": "Z"}]
EOF
echo 10x5 > resize_grow.size
cat > resize_mid.ops <<'EOF'
[{"write": "r1\r\nr2\r\nr3\r\nr4\r\nr5\u001b[2;2H"}, {"resize": [10, 3]}, {"write": "A"}, {"resize": [10, 6]}, {"write": "B"}]
EOF
echo 10x5 > resize_mid.size
# Columns only grow: xterm.js reflows wrapped lines and lines cut by a
# narrower screen, which M1 does not (DIFFERENCES.md).
cat > resize_cols.ops <<'EOF'
[{"write": "abcdefgh中\r\n0123\u001b[1;3H"}, {"resize": [14, 4]}, {"write": "X\u001b[2;13HY"}]
EOF
echo 10x4 > resize_cols.size
# A pending wrap survives a change of rows only.
cat > resize_pending_rows.ops <<'EOF'
[{"write": "\r\n0123456789"}, {"resize": [10, 5]}, {"write": "A"}]
EOF
echo 10x4 > resize_pending_rows.size
# Gaining columns while a wrap is pending puts the cursor just past the
# old last column.
cat > resize_pending_cols.ops <<'EOF'
[{"write": "0123456789\u001b7"}, {"resize": [12, 4]}, {"write": "B\u001b8C\r\n01234"}]
EOF
echo 10x4 > resize_pending_cols.size
