package ui

import (
	"image"
	"strconv"
	"strings"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// Markdown viewer (CommonMark core + common GFM extensions)
//
// Supported: ATX/Setext headings, paragraphs with soft/hard breaks, emphasis
// (* _ ** __ ***), ~~strike~~, ==mark==, `code`, [links](url), <autolinks>,
// bare http(s) URLs, images, block quotes, nested ordered/unordered lists,
// task lists, fenced code blocks, thematic breaks and pipe tables. Raw HTML
// is shown as text.

const (
	mdPara = iota
	mdHeading
	mdCode
	mdQuote
	mdList
	mdHR
	mdTable
	mdImage
)

type mdBlock struct {
	kind    int
	level   int    // heading level
	text    string // inline source (para, heading), code, image alt
	src     string // image source
	lang    string
	kids    []*mdBlock // quote
	items   []*mdItem
	ordered bool
	start   int
	header  []string
	aligns  []TextAlign
	rows    [][]string
}

type mdItem struct {
	blocks []*mdBlock
	task   int // -1 none, 0 open, 1 done
	index  int // task index in document order
}

// ParseMarkdownInline converts inline Markdown (emphasis, code, links...)
// into spans for RichText.
func ParseMarkdownInline(s string) []Span {
	return mdMerge(mdInline(s, Span{}, defaultLocaleNow().Markdown.ImagePrefix))
}

// ---------------------------------------------------------------------------
// Block parser

type mdParser struct{ tasks int }

func mdIndent(s string) int {
	n := 0
	for _, r := range s {
		switch r {
		case ' ':
			n++
		case '\t':
			n += 4 - n%4
		default:
			return n
		}
	}
	return n
}

// mdDedent removes up to n columns of leading indentation.
func mdDedent(s string, n int) string {
	col := 0
	for i, r := range s {
		if col >= n {
			return s[i:]
		}
		switch r {
		case ' ':
			col++
		case '\t':
			col += 4 - col%4
		default:
			return s[i:]
		}
	}
	return ""
}

func mdIsHR(t string) bool {
	if len(t) < 3 {
		return false
	}
	var ch rune
	n := 0
	for _, r := range t {
		switch {
		case r == ' ' || r == '\t':
		case (r == '-' || r == '*' || r == '_') && (ch == 0 || ch == r):
			ch = r
			n++
		default:
			return false
		}
	}
	return n >= 3
}

func mdHeadingOf(t string) (int, string, bool) {
	n := 0
	for n < len(t) && t[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || (n < len(t) && t[n] != ' ' && t[n] != '\t') {
		return 0, "", false
	}
	s := strings.TrimSpace(t[n:])
	// Optional closing #s.
	if k := strings.TrimRight(s, "#"); k != s && (k == "" || strings.HasSuffix(k, " ")) {
		s = strings.TrimSpace(k)
	}
	return n, s, true
}

func mdFence(t string) (string, string, bool) {
	for _, f := range []string{"```", "~~~"} {
		if strings.HasPrefix(t, f) {
			n := 0
			for n < len(t) && t[n] == f[0] {
				n++
			}
			return t[:n], strings.TrimSpace(t[n:]), true
		}
	}
	return "", "", false
}

// mdListMarker parses a list item marker; content is the column where
// the item text starts.
func mdListMarker(line string) (ordered bool, num int, content int, rest string, ok bool) {
	ind := mdIndent(line)
	t := mdDedent(line, ind)
	if t == "" {
		return
	}
	switch t[0] {
	case '-', '*', '+':
		if len(t) == 1 {
			return false, 0, ind + 2, "", true
		}
		if t[1] == ' ' || t[1] == '\t' {
			sp := mdIndent(t[1:])
			if sp > 4 {
				sp = 1
			}
			return false, 0, ind + 1 + sp, strings.TrimLeft(t[1:], " \t"), true
		}
		return
	}
	i := 0
	for i < len(t) && i < 9 && t[i] >= '0' && t[i] <= '9' {
		i++
	}
	if i == 0 || i >= len(t) || (t[i] != '.' && t[i] != ')') {
		return
	}
	n, _ := strconv.Atoi(t[:i])
	if i+1 == len(t) {
		return true, n, ind + i + 2, "", true
	}
	if t[i+1] != ' ' && t[i+1] != '\t' {
		return
	}
	sp := mdIndent(t[i+1:])
	if sp > 4 {
		sp = 1
	}
	return true, n, ind + i + 1 + sp, strings.TrimLeft(t[i+1:], " \t"), true
}

func mdSplitRow(line string) []string {
	t := strings.TrimSpace(line)
	t = strings.TrimPrefix(t, "|")
	if strings.HasSuffix(t, "|") && !strings.HasSuffix(t, "\\|") {
		t = t[:len(t)-1]
	}
	var cells []string
	var b strings.Builder
	code := false
	for i := 0; i < len(t); i++ {
		switch {
		case t[i] == '\\' && i+1 < len(t) && t[i+1] == '|':
			b.WriteByte('|')
			i++
		case t[i] == '`':
			code = !code
			b.WriteByte('`')
		case t[i] == '|' && !code:
			cells = append(cells, strings.TrimSpace(b.String()))
			b.Reset()
		default:
			b.WriteByte(t[i])
		}
	}
	return append(cells, strings.TrimSpace(b.String()))
}

func mdDelimRow(line string) ([]TextAlign, bool) {
	if !strings.Contains(line, "-") {
		return nil, false
	}
	cells := mdSplitRow(line)
	out := make([]TextAlign, len(cells))
	for i, c := range cells {
		if c == "" {
			return nil, false
		}
		l, r := strings.HasPrefix(c, ":"), strings.HasSuffix(c, ":")
		if strings.Trim(c, ":-") != "" || !strings.Contains(c, "-") {
			return nil, false
		}
		// GFM column alignment is HTML align="left|right": AlignLeft /
		// AlignRight (transitional aliases of Start / End that become the
		// physical sides after the RTL audit). Unmarked columns align to
		// the start of each line.
		switch {
		case l && r:
			out[i] = AlignCenter
		case r:
			out[i] = AlignRight
		}
	}
	return out, true
}

// startsBlock reports whether line begins a block that interrupts a
// paragraph.
func mdStartsBlock(line string) bool {
	if mdIndent(line) >= 4 {
		return false
	}
	t := strings.TrimSpace(line)
	if t == "" || mdIsHR(t) || strings.HasPrefix(t, ">") {
		return true
	}
	if _, _, ok := mdHeadingOf(t); ok {
		return true
	}
	if _, _, ok := mdFence(t); ok {
		return true
	}
	if ord, n, _, rest, ok := mdListMarker(line); ok && rest != "" && (!ord || n == 1) {
		return true
	}
	return false
}

func (p *mdParser) parse(lines []string) []*mdBlock {
	var out []*mdBlock
	for i := 0; i < len(lines); {
		line := lines[i]
		t := strings.TrimSpace(line)
		ind := mdIndent(line)
		if t == "" {
			i++
			continue
		}
		// Fenced code.
		if ind < 4 {
			if fence, lang, ok := mdFence(t); ok {
				var code []string
				j := i + 1
				for ; j < len(lines); j++ {
					ct := strings.TrimSpace(lines[j])
					if strings.HasPrefix(ct, fence) && strings.Trim(ct, fence[:1]) == "" {
						break
					}
					code = append(code, mdDedent(lines[j], ind))
				}
				out = append(out, &mdBlock{kind: mdCode, text: strings.Join(code, "\n"), lang: lang})
				i = j + 1
				continue
			}
		}
		// Indented code.
		if ind >= 4 {
			var code []string
			j := i
			for ; j < len(lines); j++ {
				if strings.TrimSpace(lines[j]) != "" && mdIndent(lines[j]) < 4 {
					break
				}
				code = append(code, mdDedent(lines[j], 4))
			}
			for len(code) > 0 && strings.TrimSpace(code[len(code)-1]) == "" {
				code = code[:len(code)-1]
			}
			out = append(out, &mdBlock{kind: mdCode, text: strings.Join(code, "\n")})
			i = j
			continue
		}
		if lvl, s, ok := mdHeadingOf(t); ok {
			out = append(out, &mdBlock{kind: mdHeading, level: lvl, text: s})
			i++
			continue
		}
		if mdIsHR(t) {
			out = append(out, &mdBlock{kind: mdHR})
			i++
			continue
		}
		if strings.HasPrefix(t, ">") {
			var inner []string
			j := i
			for ; j < len(lines); j++ {
				lt := strings.TrimSpace(lines[j])
				if strings.HasPrefix(lt, ">") {
					s := strings.TrimPrefix(lt, ">")
					s = strings.TrimPrefix(s, " ")
					inner = append(inner, s)
					continue
				}
				// Lazy continuation of a quoted paragraph.
				if lt != "" && len(inner) > 0 && strings.TrimSpace(inner[len(inner)-1]) != "" && !mdStartsBlock(lines[j]) {
					inner = append(inner, lt)
					continue
				}
				break
			}
			out = append(out, &mdBlock{kind: mdQuote, kids: p.parse(inner)})
			i = j
			continue
		}
		if ord, num, _, _, ok := mdListMarker(line); ok {
			b, j := p.list(lines, i, ord, num)
			out = append(out, b)
			i = j
			continue
		}
		// Table: header row followed by a delimiter row.
		if strings.Contains(line, "|") && i+1 < len(lines) {
			if al, ok := mdDelimRow(lines[i+1]); ok {
				hdr := mdSplitRow(line)
				if len(hdr) == len(al) {
					b := &mdBlock{kind: mdTable, header: hdr, aligns: al}
					j := i + 2
					for ; j < len(lines); j++ {
						if strings.TrimSpace(lines[j]) == "" || !strings.Contains(lines[j], "|") {
							break
						}
						row := mdSplitRow(lines[j])
						for len(row) < len(hdr) {
							row = append(row, "")
						}
						b.rows = append(b.rows, row[:len(hdr)])
					}
					out = append(out, b)
					i = j
					continue
				}
			}
		}
		// Paragraph (possibly a Setext heading).
		para := []string{line}
		j := i + 1
		level := 0
		for ; j < len(lines); j++ {
			lt := strings.TrimSpace(lines[j])
			if lt != "" && mdIndent(lines[j]) < 4 && strings.Trim(lt, "=") == "" {
				level = 1
				j++
				break
			}
			if lt != "" && mdIndent(lines[j]) < 4 && strings.Trim(lt, "-") == "" && len(para) > 0 {
				level = 2
				j++
				break
			}
			if mdStartsBlock(lines[j]) {
				break
			}
			para = append(para, lines[j])
		}
		src := mdJoinPara(para)
		switch {
		case level > 0:
			out = append(out, &mdBlock{kind: mdHeading, level: level, text: src})
		default:
			if alt, img, ok := mdOnlyImage(src); ok {
				out = append(out, &mdBlock{kind: mdImage, text: alt, src: img})
			} else {
				out = append(out, &mdBlock{kind: mdPara, text: src})
			}
		}
		i = j
	}
	return out
}

// mdJoinPara joins paragraph lines: trailing double space or backslash is
// a hard break; soft breaks become a space except between CJK characters.
func mdJoinPara(lines []string) string {
	var b strings.Builder
	for k, l := range lines {
		s := strings.TrimLeft(l, " \t")
		last := k == len(lines)-1
		hard := false
		if !last {
			if strings.HasSuffix(s, "  ") {
				hard = true
			} else if strings.HasSuffix(s, "\\") {
				hard = true
				s = s[:len(s)-1]
			}
		}
		s = strings.TrimRight(s, " \t")
		b.WriteString(s)
		if last {
			break
		}
		switch {
		case hard:
			b.WriteByte('\n')
		default:
			next := strings.TrimLeft(lines[k+1], " \t")
			nr, _ := moreFirstRune(next)
			if !(moreIsCJK(moreLastRune(s)) && moreIsCJK(nr)) {
				b.WriteByte(' ')
			}
		}
	}
	return b.String()
}

func mdOnlyImage(s string) (alt, src string, ok bool) {
	if !strings.HasPrefix(s, "![") {
		return
	}
	alt, src, n, ok := mdLink(s, 1)
	if !ok || n != len(s) {
		return "", "", false
	}
	return alt, src, true
}

func (p *mdParser) list(lines []string, i int, ordered bool, start int) (*mdBlock, int) {
	b := &mdBlock{kind: mdList, ordered: ordered, start: start}
	baseInd := mdIndent(lines[i])
	for i < len(lines) {
		ord, _, content, rest, ok := mdListMarker(lines[i])
		if !ok || ord != ordered || mdIndent(lines[i]) < baseInd || mdIndent(lines[i]) >= content && mdIndent(lines[i]) > baseInd+3 {
			break
		}
		item := &mdItem{task: -1}
		if len(rest) >= 3 && rest[0] == '[' && rest[2] == ']' && (len(rest) == 3 || rest[3] == ' ') {
			switch rest[1] {
			case ' ':
				item.task = 0
			case 'x', 'X':
				item.task = 1
			}
			if item.task >= 0 {
				item.index = p.tasks
				p.tasks++
				rest = strings.TrimLeft(rest[3:], " ")
			}
		}
		body := []string{rest}
		j := i + 1
		blank := false
		for ; j < len(lines); j++ {
			l := lines[j]
			lt := strings.TrimSpace(l)
			if lt == "" {
				blank = true
				body = append(body, "")
				continue
			}
			if mdIndent(l) >= content {
				body = append(body, mdDedent(l, content))
				blank = false
				continue
			}
			// Lazy continuation of the item's paragraph.
			if !blank && !mdStartsBlock(l) {
				if _, _, _, _, isItem := mdListMarker(l); !isItem {
					body = append(body, lt)
					continue
				}
			}
			break
		}
		item.blocks = p.parse(body)
		b.items = append(b.items, item)
		i = j
		// A blank line followed by something that is not an item ends the list.
		if i < len(lines) {
			if _, _, _, _, ok := mdListMarker(lines[i]); !ok {
				break
			}
		}
	}
	return b, i
}

// ---------------------------------------------------------------------------
// Inline parser

// mdLink parses "[text](url "title")" starting at s[i] == '['; it returns
// the text, URL and the index after ')'.
func mdLink(s string, i int) (string, string, int, bool) {
	if i >= len(s) || s[i] != '[' {
		return "", "", 0, false
	}
	depth := 0
	j := i
	for ; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case '[':
			depth++
		case ']':
			depth--
		}
		if depth == 0 {
			break
		}
	}
	if j >= len(s) || j+1 >= len(s) || s[j+1] != '(' {
		return "", "", 0, false
	}
	k := j + 2
	paren := 0
	for ; k < len(s); k++ {
		if s[k] == '(' {
			paren++
		}
		if s[k] == ')' {
			if paren == 0 {
				break
			}
			paren--
		}
	}
	if k >= len(s) {
		return "", "", 0, false
	}
	dest := strings.TrimSpace(s[j+2 : k])
	if sp := strings.IndexAny(dest, " \t"); sp >= 0 {
		dest = dest[:sp] // drop the title
	}
	dest = strings.TrimSuffix(strings.TrimPrefix(dest, "<"), ">")
	return s[i+1 : j], dest, k + 1, true
}

func mdIsPunct(b byte) bool { return strings.IndexByte("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~", b) >= 0 }

func mdAlnum(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// mdCloser finds the closing delimiter for an emphasis run.
func mdCloser(s string, from int, delim string) int {
	for k := from; k <= len(s)-len(delim); k++ {
		switch {
		case s[k] == '\\':
			k++
		case s[k] == '`':
			n := 1
			for k+n < len(s) && s[k+n] == '`' {
				n++
			}
			if e := strings.Index(s[k+n:], strings.Repeat("`", n)); e >= 0 {
				k += n + e + n - 1
			}
		case strings.HasPrefix(s[k:], delim) && k > from && s[k-1] != ' ':
			// For single delimiters skip doubled ones (they belong to bold).
			if len(delim) == 1 && k+1 < len(s) && s[k+1] == delim[0] {
				k++
				continue
			}
			if delim[0] == '_' && k+len(delim) < len(s) && mdAlnum(s[k+len(delim)]) {
				continue
			}
			// In a longer run ("***" closing "**"), the outer delimiter is
			// the last one: "**b *c***" = strong(b, em(c)).
			r := 0
			for k+r < len(s) && s[k+r] == delim[0] {
				r++
			}
			if len(delim) > 1 && r > len(delim) {
				k += r - len(delim)
			}
			return k
		}
	}
	return -1
}

func mdInline(s string, base Span, imgPrefix string) []Span {
	var out []Span
	var buf strings.Builder
	flush := func() {
		if buf.Len() > 0 {
			sp := base
			sp.Text = buf.String()
			out = append(out, sp)
			buf.Reset()
		}
	}
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '\\' && i+1 < len(s) && mdIsPunct(s[i+1]):
			buf.WriteByte(s[i+1])
			i += 2
			continue
		case c == '`':
			n := 1
			for i+n < len(s) && s[i+n] == '`' {
				n++
			}
			if e := strings.Index(s[i+n:], strings.Repeat("`", n)); e >= 0 {
				flush()
				code := s[i+n : i+n+e]
				if len(code) >= 2 && code[0] == ' ' && code[len(code)-1] == ' ' && strings.TrimSpace(code) != "" {
					code = code[1 : len(code)-1]
				}
				sp := base
				sp.Text, sp.Code, sp.Link = code, true, base.Link
				out = append(out, sp)
				i += n + e + n
				continue
			}
			buf.WriteString(s[i : i+n])
			i += n
			continue
		case c == '!' && i+1 < len(s) && s[i+1] == '[':
			if alt, src, end, ok := mdLink(s, i+1); ok {
				flush()
				sp := base
				sp.Text, sp.Link = imgPrefix+alt, src
				out = append(out, sp)
				i = end
				continue
			}
		case c == '[':
			if txt, url, end, ok := mdLink(s, i); ok {
				flush()
				b := base
				b.Link = url
				out = append(out, mdInline(txt, b, imgPrefix)...)
				i = end
				continue
			}
		case c == '<':
			if e := strings.IndexByte(s[i:], '>'); e > 1 {
				inner := s[i+1 : i+e]
				if !strings.ContainsAny(inner, " <") && (strings.Contains(inner, "://") || strings.Contains(inner, "@")) {
					flush()
					sp := base
					sp.Text, sp.Link = inner, inner
					if !strings.Contains(inner, "://") {
						sp.Link = "mailto:" + inner
					}
					out = append(out, sp)
					i += e + 1
					continue
				}
			}
		case c == 'h' && base.Link == "" && (strings.HasPrefix(s[i:], "https://") || strings.HasPrefix(s[i:], "http://")) &&
			(i == 0 || !mdAlnum(s[i-1])):
			e := i
			for e < len(s) && s[e] > ' ' && s[e] != '<' && s[e] < 0x80 {
				e++
			}
			for e > i && strings.IndexByte(".,;:!?)'\"*_~", s[e-1]) >= 0 {
				e--
			}
			flush()
			sp := base
			sp.Text, sp.Link = s[i:e], s[i:e]
			out = append(out, sp)
			i = e
			continue
		case c == '~' && strings.HasPrefix(s[i:], "~~"):
			if k := mdCloser(s, i+2, "~~"); k > i+2 && s[i+2] != ' ' {
				flush()
				b := base
				b.Strike = true
				out = append(out, mdInline(s[i+2:k], b, imgPrefix)...)
				i = k + 2
				continue
			}
		case c == '=' && strings.HasPrefix(s[i:], "=="):
			if k := mdCloser(s, i+2, "=="); k > i+2 && s[i+2] != ' ' {
				flush()
				b := base
				b.Mark = true
				out = append(out, mdInline(s[i+2:k], b, imgPrefix)...)
				i = k + 2
				continue
			}
		case c == '*' || c == '_':
			n := 1
			for i+n < len(s) && s[i+n] == c && n < 3 {
				n++
			}
			intraword := c == '_' && i > 0 && mdAlnum(s[i-1])
			matched := false
			if !intraword && i+n < len(s) && s[i+n] != ' ' {
				for m := n; m >= 1 && !matched; m-- {
					d := strings.Repeat(string(c), m)
					if k := mdCloser(s, i+m, d); k > i+m {
						flush()
						b := base
						if m >= 2 {
							b.Bold = true
						}
						if m != 2 {
							b.Italic = true
						}
						if n > m {
							buf.WriteString(strings.Repeat(string(c), n-m))
							flush()
						}
						out = append(out, mdInline(s[i+m:k], b, imgPrefix)...)
						i = k + m
						matched = true
					}
				}
			}
			if matched {
				continue
			}
			buf.WriteString(s[i : i+n])
			i += n
			continue
		}
		buf.WriteByte(c)
		i++
	}
	flush()
	return out
}

// mdMerge joins neighbouring spans with identical styles.
func mdMerge(in []Span) []Span {
	var out []Span
	for _, s := range in {
		if s.Text == "" {
			continue
		}
		if n := len(out); n > 0 {
			p := out[n-1]
			q := s
			p.Text, q.Text = "", ""
			if p == q {
				out[n-1].Text += s.Text
				continue
			}
		}
		out = append(out, s)
	}
	return out
}

// ToggleMarkdownTask flips the index-th task checkbox ("- [ ]" / "- [x]",
// in document order) in src and returns the new source.
func ToggleMarkdownTask(src string, index int) string {
	lines := strings.Split(src, "\n")
	seen := 0
	fence := ""
	for li, l := range lines {
		t := strings.TrimSpace(l)
		if f, _, ok := mdFence(t); ok && (fence == "" || strings.HasPrefix(t, fence)) {
			if fence == "" {
				fence = f
			} else {
				fence = ""
			}
			continue
		}
		if fence != "" {
			continue
		}
		// Strip quote markers, then look for a list marker + [ ].
		body := l
		for strings.HasPrefix(strings.TrimLeft(body, " "), ">") {
			body = strings.TrimPrefix(strings.TrimLeft(body, " "), ">")
		}
		_, _, _, rest, ok := mdListMarker(body)
		if !ok || len(rest) < 3 || rest[0] != '[' || rest[2] != ']' || !strings.ContainsRune(" xX", rune(rest[1])) || (len(rest) > 3 && rest[3] != ' ') {
			continue
		}
		if seen == index {
			pos := len(l) - len(rest) + 1
			ch := byte('x')
			if rest[1] != ' ' {
				ch = ' '
			}
			lines[li] = l[:pos] + string(ch) + l[pos+1:]
			return strings.Join(lines, "\n")
		}
		seen++
	}
	return src
}

// ---------------------------------------------------------------------------
// Renderer

// MarkdownW renders a Markdown document.
type MarkdownW struct {
	c        *Context
	b        *Box
	src      string
	size     float32
	images   func(src string) image.Image
	clicked  string
	toggled  int
	z        *lazy
	el       bool
	shad     bool // shadcn's typography (h2 rule, quote bar, muted code)
	copyable bool
	noSelect bool
}

type mdCache struct {
	src    string
	blocks []*mdBlock
}

// Markdown adds a rendered Markdown document; it grows to its content (put
// it in a Scroll for long documents).
//
//	md := ui.Markdown(c, src)
//	if u := md.ClickedLink(); u != "" { openURL(u) }
func Markdown(c *Context, src string, opts ...layout.Option) *MarkdownW {
	m := &MarkdownW{c: c, src: src, toggled: -1, copyable: true}
	m.b = c.box("markdown", layout.Column, opts)
	m.z = c.lazy(m.b.n, m.build)
	return m
}

// Size sets the body font size (default theme FontSize).
func (m *MarkdownW) Size(px float32) *MarkdownW { m.size = px; return m }

// Images supplies decoded images for ![alt](src); return nil to show a
// placeholder.
func (m *MarkdownW) Images(load func(src string) image.Image) *MarkdownW { m.images = load; return m }

// NoSelect makes the text non-selectable (selection and Cmd/Ctrl+C copy
// are on by default).
func (m *MarkdownW) NoSelect() *MarkdownW { m.noSelect = true; return m }

// NoCopy hides the copy button of code blocks.
func (m *MarkdownW) NoCopy() *MarkdownW { m.copyable = false; return m }

// ClickedLink returns the URL of a link clicked this frame ("" if none).
func (m *MarkdownW) ClickedLink() string { m.z.run(); return m.clicked }

// TaskToggled reports a click on a task-list checkbox: its index in document
// order. Apply it with src = ui.ToggleMarkdownTask(src, i).
func (m *MarkdownW) TaskToggled() (int, bool) { m.z.run(); return m.toggled, m.toggled >= 0 }

func (m *MarkdownW) build() {
	c := m.c
	m.el = c.Theme.Flavor == theme.Element
	m.shad = c.Theme.Flavor == theme.StyleShadcn
	st := State[mdCache](c, m.b.n.id, nil)
	if st.blocks == nil || st.src != m.src {
		src := strings.ReplaceAll(m.src, "\r\n", "\n")
		p := &mdParser{}
		st.blocks, st.src = p.parse(strings.Split(src, "\n")), m.src
	}
	m.blocks(st.blocks, Span{}, true)
}

func (m *MarkdownW) fontSize() float32 {
	if m.size > 0 {
		return m.size
	}
	return m.c.Theme.FontSize
}

func (m *MarkdownW) rich(spans []Span, size float32, col theme.Color) *RichTextW {
	w := RichText(m.c, spans...).Size(size)
	if !m.noSelect {
		w.Selectable()
	}
	if col.A > 0 {
		w.Color(col)
	}
	if u := w.ClickedLink(); u != "" {
		m.clicked = u
	}
	w.n.setRole(a11y.RoleParagraph).setName(PlainText(spans))
	return w
}

// blocks renders bs; base carries inherited inline style (quote color).
func (m *MarkdownW) blocks(bs []*mdBlock, base Span, top bool) {
	c, th := m.c, m.c.Theme
	fs := m.fontSize()
	gap := fs // 1em between blocks
	for k, b := range bs {
		last := k == len(bs)-1
		mb := gap
		if last {
			mb = 0
		}
		var mt float32
		Key(c, k, func() {
			switch b.kind {
			case mdHeading:
				size := m.headingSize(b.level)
				if k > 0 {
					mt = size * 0.6
				}
				spans := mdInline(b.text, Span{Bold: true}, m.c.Locale().Markdown.ImagePrefix)
				col := th.TextHeading
				w := m.rich(mdMerge(spans), size, col)
				w.n.setRole(a11y.RoleHeading)
				if lv := min(b.level, 6); c.a11yOn {
					w.n.fill(semFunc(func(s *a11y.Node) { s.Level = lv }))
				}
				w.Layout(layout.Margin(geom.Insets{Top: mt, Bottom: min(mb, size*0.5)}))
				switch {
				case m.el && b.level <= 2:
					w.n.ln.Padding.Bottom = 6
					w.n.over = func(l *paint.List, r geom.Rect) {
						l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - 1, W: r.W, H: 1}, geom.Radii{}, th.BorderSecondary)
					}
				case m.shad && b.level == 2:
					// shadcn typography h2: border-b pb-2.
					w.n.ln.Padding.Bottom = 8
					w.n.over = func(l *paint.List, r geom.Rect) {
						l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - 1, W: r.W, H: 1}, geom.Radii{}, th.Border)
					}
				}
			case mdPara:
				m.rich(mdMerge(mdInline(b.text, base, m.c.Locale().Markdown.ImagePrefix)), fs, base.Color).Layout(layout.Margin(geom.Insets{Bottom: mb}))
			case mdCode:
				m.code(b, mb)
			case mdQuote:
				col := th.TextSecondary
				bar, barW, padL := th.Split, float32(4), float32(12)
				if m.shad {
					// shadcn typography blockquote: border-l-2 pl-6 italic,
					// in the body color.
					col, bar, barW, padL = base.Color, th.Border, 2, 24
				}
				q := Column(c, func() {
					inner := base
					inner.Color = col
					inner.Italic = inner.Italic || m.shad
					m.blocks(b.kids, inner, false)
				}, layout.Padding(geom.Insets{Left: padL, Top: 2, Bottom: 2}), layout.Margin(geom.Insets{Bottom: mb}))
				q.Paint(func(l *paint.List, r geom.Rect) {
					l.Fill(c.inline(r).Rect(0, r.Y, barW, r.H), geom.Rad(barW/2), bar) // bar on the start edge
				})
				if m.el {
					q.n.ln.Padding = geom.Insets{Left: 12, Top: 8, Right: 12, Bottom: 8}
					q.Bg(th.FillQuaternary).Radius(th.RadiusSM)
				}
			case mdList:
				m.list(b, base, 0, mb)
			case mdHR:
				d := Divider(c)
				d.n.ln.Margin = geom.Insets{Top: 4, Bottom: max(mb, 4)}
			case mdTable:
				m.table(b, base, mb)
			case mdImage:
				m.image(b, mb)
			}
		})
	}
	_ = top
}

func (m *MarkdownW) headingSize(level int) float32 {
	th := m.c.Theme
	if level >= 1 && level <= 5 {
		// Documents read better a step below Typography's display sizes.
		sizes := [5]float32{th.Heading[1], th.Heading[2], th.Heading[3], th.Heading[4], th.FontSizeLG}
		return sizes[level-1]
	}
	return m.fontSize()
}

func (m *MarkdownW) code(b *mdBlock, mb float32) {
	c, th := m.c, m.c.Theme
	box := Column(c, func() {
		if b.lang != "" || m.copyable {
			Row(c, func() {
				Text(c, b.lang).Size(12).Color(th.TextTertiary).Layout(layout.Grow(1))
				if m.copyable && IconButton(c, icon.Copy, c.Locale().Markdown.CopyCode).Size(22, 14).Clicked() {
					if c.win != nil {
						c.win.WriteClipboard(b.text)
					}
					Message(c, "success", c.Locale().Markdown.Copied)
				}
			}, layout.Middle(), layout.Margin(geom.Insets{Bottom: 4}))
		}
		code := RichText(c, Span{Text: b.text, Mono: true}).Pre().Size(m.fontSize() - 1)
		code.n.setRole(a11y.RoleParagraph).setName(b.text)
		if !m.noSelect {
			code.Selectable()
		}
	}, layout.PadVH(10, 14), layout.Margin(geom.Insets{Bottom: mb}))
	box.Bg(theme.RGBA(150, 150, 150, .1)).Border(1, theme.RGBA(100, 100, 100, .2)).Radius(th.RadiusSM + 2)
	switch {
	case m.el:
		box.Bg(th.FillAlter).Border(1, th.BorderSecondary).Radius(th.Radius)
	case m.shad:
		// bg-muted, no border, rounded-md (the Typography code token).
		box.Bg(c.ThemeFor(theme.CompTypography).Components.Typography.CodeBg).Border(0, theme.Color{}).Radius(th.Radius)
	}
}

func (m *MarkdownW) list(b *mdBlock, base Span, depth int, mb float32) {
	c, th := m.c, m.c.Theme
	fs := m.fontSize()
	lh := th.LineHeightFor(fs)
	markW := float32(18)
	if b.ordered {
		last := strconv.Itoa(b.start+len(b.items)-1) + "."
		markW = max(markW, text.Width(last, c.style(fs, text.Regular))+6)
	}
	Column(c, func() {
		for i, it := range b.items {
			i, it := i, it
			Key(c, i, func() {
				Row(c, func() {
					switch {
					case it.task >= 0:
						on := it.task == 1
						cb := Checkbox(c, "", &on)
						if len(it.blocks) > 0 {
							cb.A11yLabel(PlainText(mdInline(it.blocks[0].text, Span{}, "")))
						}
						cb.n.ln.Height = lh
						cb.n.ln.Margin.Right = 8
						if cb.Changed() {
							m.toggled = it.index
						}
					default:
						n := c.add("md-mark", "")
						n.aHidden = true // the list item's position is exposed instead
						n.ln.Width, n.ln.Height = markW, lh
						num := strconv.Itoa(b.start+i) + "."
						ordered := b.ordered
						d := depth
						col := th.Text
						if base.Color.A > 0 {
							col = base.Color
						}
						n.paint = func(l *paint.List, r geom.Rect) {
							cy := r.Y + r.H/2
							if ordered {
								lay := text.Shape(num, c.style(fs, text.Regular), text.Options{NoWrap: true})
								// The number hugs the item text (the mark's end edge).
								l.Glyphs(geom.Pt(c.inline(r).X(r.W-6-lay.Width, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
								return
							}
							cx := c.inline(r).X(6, 0)
							switch d % 3 {
							case 0:
								l.Path(paint.NewPath().Circle(cx, cy, 2.5), col)
							case 1:
								l.StrokePath(paint.NewPath().Circle(cx, cy, 2.5), paint.StrokeStyle{Width: 1}, col)
							default:
								l.Fill(geom.Rect{X: cx - 2.5, Y: cy - 2.5, W: 5, H: 5}, geom.Radii{}, col)
							}
						}
					}
					Column(c, func() {
						for k, ib := range it.blocks {
							k, ib := k, ib
							Key(c, k, func() {
								if ib.kind == mdList {
									m.list(ib, base, depth+1, 0)
									return
								}
								m.blocks([]*mdBlock{ib}, base, false)
							})
							if k < len(it.blocks)-1 {
								Gap(c, 4)
							}
						}
					}, layout.Grow(1))
				}, layout.AlignItems(layout.AlignStart)).n.setRole(a11y.RoleListItem)
			})
		}
	}, layout.Gap(4), layout.Padding(geom.Insets{Left: 4}), layout.Margin(geom.Insets{Top: b2f(depth > 0) * 4, Bottom: mb})).n.setRole(a11y.RoleList)
}

func (m *MarkdownW) table(b *mdBlock, base Span, mb float32) {
	c, th := m.c, m.c.Theme
	fs := m.fontSize()
	border := th.BorderSecondary
	cell := func(src string, al TextAlign, head, lastCol bool) {
		sp := base
		sp.Bold = head
		col := Column(c, func() {
			m.rich(mdMerge(mdInline(src, sp, m.c.Locale().Markdown.ImagePrefix)), fs, base.Color).Align(al)
		}, layout.Grow(1), layout.Basis(0), layout.PadVH(8, 12))
		col.n.setRole(a11y.RoleCell)
		if head {
			col.n.setRole(a11y.RoleColumnHeader)
		}
		if !lastCol {
			col.Paint(func(l *paint.List, r geom.Rect) {
				// Column rule on the cell's end edge (cells mirror in RTL).
				l.Fill(c.inline(r).Rect(r.W-1, r.Y, 1, r.H), geom.Radii{}, border)
			})
		}
	}
	t := Column(c, func() {
		row := func(cells []string, head bool, lastRow bool) {
			r := Row(c, func() {
				for i, s := range cells {
					al := AlignStart
					if i < len(b.aligns) {
						al = b.aligns[i]
					}
					cell(s, al, head, i == len(cells)-1)
				}
			}, layout.AlignItems(layout.AlignStretch))
			r.n.setRole(a11y.RoleRow)
			if head {
				r.Bg(th.FillAlter)
			}
			if !lastRow {
				r.Paint(func(l *paint.List, rr geom.Rect) {
					l.Fill(geom.Rect{X: rr.X, Y: rr.Bottom() - 1, W: rr.W, H: 1}, geom.Radii{}, border)
				})
			}
		}
		Key(c, "h", func() { row(b.header, true, len(b.rows) == 0) })
		for i, rw := range b.rows {
			i, rw := i, rw
			Key(c, i, func() { row(rw, false, i == len(b.rows)-1) })
		}
	}, layout.Margin(geom.Insets{Bottom: mb}))
	t.n.setRole(a11y.RoleTable)
	if c.a11yOn {
		rows, cols := len(b.rows)+1, len(b.header)
		t.n.fill(semFunc(func(s *a11y.Node) { s.RowCount, s.ColCount = rows, cols }))
	}
	t.Border(1, border).Radius(th.RadiusSM).Clip()
}

func (m *MarkdownW) image(b *mdBlock, mb float32) {
	c, th := m.c, m.c.Theme
	var img image.Image
	if m.images != nil {
		img = m.images(b.src)
	}
	alt := b.text
	if alt == "" {
		alt = c.Locale().A11y.Image
	}
	if img == nil {
		Row(c, func() {
			Icon(c, icon.File).Size(16).Color(th.TextTertiary)
			alt := b.text
			if alt == "" {
				alt = b.src
			}
			Text(c, alt).Color(th.TextTertiary)
		}, layout.Gap(8), layout.Middle(), layout.PadVH(12, 16), layout.Margin(geom.Insets{Bottom: mb})).
			DashedBorder(1, th.Border).Radius(th.RadiusSM).Semantics(a11y.RoleImage, alt).n.ln.NoStretch = true
		return
	}
	sz := img.Bounds().Size()
	iw, ih := float32(sz.X), float32(sz.Y)
	n := c.add("md-image", b.src)
	n.setRole(a11y.RoleImage).setName(alt)
	n.ln.NoStretch = true
	n.ln.Margin.Bottom = mb
	n.ln.Measure = func(maxW, _ float32) geom.Size {
		w := min(iw, maxW)
		if w <= 0 || iw <= 0 {
			return geom.Size{}
		}
		return geom.Size{W: w, H: ih * w / iw}
	}
	n.paint = func(l *paint.List, r geom.Rect) { l.DrawImage(img, r, geom.Rad(th.RadiusSM)) }
}
