package ui

import (
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

// HeadingBlock returns the zero-based top-level block containing a heading.
// It accepts a heading ID or a URL fragment with a leading #. IDs are based
// on the heading's inline text: lowercase letters, marks, numbers, hyphens
// and underscores are kept, spaces become hyphens and other characters are
// removed. Duplicate IDs receive numeric suffixes in document order.
func (m *MarkdownW) HeadingBlock(id string) (int, bool) {
	id, err := url.PathUnescape(strings.TrimPrefix(id, "#"))
	if err != nil || id == "" {
		return 0, false
	}
	st := m.document()
	if st.headings == nil {
		st.indexHeadings()
	}
	block, ok := st.headings[id]
	return block, ok
}

// ScrollToHeading queues a jump to the heading's containing block in a
// Virtual Markdown viewport and reports whether the fragment was found.
// It returns false for a non-virtual document or an unknown heading.
func (m *MarkdownW) ScrollToHeading(id string) bool {
	if !m.virtual {
		return false
	}
	block, ok := m.HeadingBlock(id)
	return ok && m.ScrollToBlock(block)
}

// ScrollToBlock queues a jump to the zero-based top-level block in a Virtual
// Markdown viewport. It aligns the block near the top, within the scroll
// bounds. It returns false for a non-virtual document or an invalid index.
func (m *MarkdownW) ScrollToBlock(block int) bool {
	if !m.virtual || block < 0 {
		return false
	}
	st := m.document()
	if block >= len(st.blocks) {
		return false
	}
	st.window.jump, st.window.hasJump = block, true
	if m.z.done {
		m.c.Invalidate()
	}
	return true
}

func (st *mdCache) indexHeadings() {
	st.headings = make(map[string]int)
	nextSuffix := make(map[string]int)
	var visit func(b *mdBlock, index int)
	visit = func(b *mdBlock, index int) {
		if b.kind == mdHeading {
			base := mdHeadingID(b.text, st.refs)
			if base == "" {
				return
			}
			id := base
			for _, exists := st.headings[id]; exists; _, exists = st.headings[id] {
				nextSuffix[base]++
				id = base + "-" + strconv.Itoa(nextSuffix[base])
			}
			st.headings[id] = index
		}
		for _, child := range b.kids {
			visit(child, index)
		}
		for _, item := range b.items {
			for _, child := range item.blocks {
				visit(child, index)
			}
		}
	}
	for i, b := range st.blocks {
		visit(b, i)
	}
}

func mdHeadingID(src string, refs map[string]string) string {
	var text strings.Builder
	for _, part := range mdInlines(src, Span{}, "", refs) {
		text.WriteString(part.Text)
	}
	return strings.Map(func(r rune) rune {
		switch {
		case unicode.IsSpace(r):
			return '-'
		case r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsMark(r) || unicode.IsNumber(r):
			return unicode.ToLower(r)
		default:
			return -1
		}
	}, strings.TrimSpace(text.String()))
}
