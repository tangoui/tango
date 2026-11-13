package buffer

import "testing"

// Monaco's rule for a marker inside a replaced range [start, start+del) with
// ins new bytes: offsets in the overlapping prefix (k < min(del, ins)) keep
// their offset; the rest move to start+ins. A whole-document replace (LSP
// formatting, reload) therefore keeps anchors where they were, clamped to the
// new length, instead of collapsing them all to 0.
func TestAnchorWholeDocumentReplace(t *testing.T) {
	const old = "func a() {\n\treturn 1\n}\n"
	for _, c := range []struct {
		name, text string
	}{
		{"same length", "func b() {\n\treturn 2\n}\n"},
		{"longer", "func a() {\n    return 1\n}\n\n"},
		{"shorter", "x\n"},
	} {
		for _, bias := range []Bias{Left, Right} {
			b := New(old)
			offs := []int{0, 3, 11, 12, 20, len(old) - 1}
			var as []Anchor
			for _, o := range offs {
				as = append(as, b.Anchor(o, bias))
			}
			mustEdit(t, b, []Edit{{Start: 0, End: len(old), Text: c.text}}, nil, nil)
			for i, a := range as {
				want := min(offs[i], len(c.text))
				if got := b.Resolve(a); got != want {
					t.Errorf("%s %v: anchor at %d -> %d, want %d", c.name, bias, offs[i], got, want)
				}
			}
			// An anchor at the very end follows the end of the document.
			b = New(old)
			endR := b.Anchor(len(old), Right)
			mustEdit(t, b, []Edit{{Start: 0, End: len(old), Text: c.text}}, nil, nil)
			if got := b.Resolve(endR); got != len(c.text) {
				t.Errorf("%s: end anchor -> %d, want %d", c.name, got, len(c.text))
			}
		}
	}
}

// Several replacements in one Apply: each anchor follows the edit it is in,
// shifted by the length changes of the edits before it.
func TestAnchorMultiEditReplace(t *testing.T) {
	// "aaaa bbbb cccc": [0,4) -> "A" (shrinks), [10,14) -> "CCCCCC" (grows).
	b := New("aaaa bbbb cccc")
	type tc struct {
		off  int
		bias Bias
		want int
	}
	cases := []tc{
		{0, Left, 0}, {0, Right, 0}, // replace starts at the anchor: stays
		{1, Right, 1},               // at start+min(del,ins) of a shrinking replace: stays
		{2, Left, 1},                // past the common prefix: start+ins
		{4, Left, 1}, {4, Right, 1}, // end of a shrinking replace: start+ins
		{7, Left, 4}, // between the edits: shifted by -3
		{10, Right, 7},
		{11, Left, 8}, {13, Right, 10}, // inside the common prefix of the second edit: keep, then shift
		{14, Left, 11},  // end of a growing replace, Left: stays at start+del
		{14, Right, 13}, // Right: after the new text
	}
	var as []Anchor
	for _, c := range cases {
		as = append(as, b.Anchor(c.off, c.bias))
	}
	mustEdit(t, b, []Edit{{Start: 0, End: 4, Text: "A"}, {Start: 10, End: 14, Text: "CCCCCC"}}, nil, nil)
	if got := b.Snapshot().String(); got != "A bbbb CCCCCC" {
		t.Fatalf("text %q", got)
	}
	for i, c := range cases {
		if got := b.Resolve(as[i]); got != c.want {
			t.Errorf("anchor %d %v -> %d, want %d", c.off, c.bias, got, c.want)
		}
	}
}

// Undo of a step that merged several multi-edit Edit calls replays the
// inverse batches; anchors inside replaced text come back to their offsets.
func TestAnchorUndoMergedMultiEdit(t *testing.T) {
	b := New("hello world")
	b.PushUndoStop() // the next edits form one explicit step
	a := b.Anchor(8, Left)
	r := b.Anchor(2, Right)
	mustEdit(t, b, []Edit{{Start: 0, End: 5, Text: "HELLO"}, {Start: 6, End: 11, Text: "WORLD"}}, nil, nil)
	mustEdit(t, b, []Edit{{Start: 0, End: 0, Text: ">"}}, nil, nil)
	if got := b.Snapshot().String(); got != ">HELLO WORLD" {
		t.Fatalf("text %q", got)
	}
	if b.Resolve(a) != 9 || b.Resolve(r) != 3 {
		t.Fatalf("before undo: %d %d, want 9 3", b.Resolve(a), b.Resolve(r))
	}
	if _, _, ok := b.Undo(); !ok {
		t.Fatal("nothing to undo")
	}
	if got := b.Snapshot().String(); got != "hello world" {
		t.Fatalf("undo merged step: %q", got)
	}
	if b.Resolve(a) != 8 || b.Resolve(r) != 2 {
		t.Fatalf("after undo: %d %d, want 8 2", b.Resolve(a), b.Resolve(r))
	}
	b.Redo()
	if b.Resolve(a) != 9 || b.Resolve(r) != 3 {
		t.Fatalf("after redo: %d %d, want 9 3", b.Resolve(a), b.Resolve(r))
	}
}

// Keeping an offset inside replaced text can land inside a multi-byte
// character of the new text; Resolve snaps to a character boundary.
func TestAnchorReplaceSnapsToRune(t *testing.T) {
	b := New("abcdef")
	l, r := b.Anchor(1, Left), b.Anchor(1, Right)
	mustEdit(t, b, []Edit{{Start: 0, End: 6, Text: "中文字"}}, nil, nil)
	if got := b.Resolve(l); got != 0 {
		t.Errorf("Left -> %d, want 0", got)
	}
	if got := b.Resolve(r); got != 3 {
		t.Errorf("Right -> %d, want 3", got)
	}
}
