package graph

import (
	"fmt"
	"testing"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

func commits(spec ...string) []Commit {
	// spec: "hash:parent1,parent2"
	out := make([]Commit, len(spec))
	for i, s := range spec {
		var h, ps string
		for j := range s {
			if s[j] == ':' {
				h, ps = s[:j], s[j+1:]
				break
			}
		}
		if h == "" {
			h = s
		}
		out[i] = Commit{Hash: h, Subject: h}
		start := 0
		for j := 0; j <= len(ps); j++ {
			if j == len(ps) || ps[j] == ',' {
				if j > start {
					out[i].Parents = append(out[i].Parents, ps[start:j])
				}
				start = j + 1
			}
		}
	}
	return out
}

// checkContinuity verifies that every line leaving the bottom of a row
// enters the top of the next row in the same lane, and vice versa.
func checkContinuity(t *testing.T, lay *GitLayout) {
	t.Helper()
	for i := 0; i+1 < len(lay.Rows); i++ {
		bottom := map[int]bool{}
		for _, l := range lay.Rows[i].Lines {
			switch l.Part {
			case LineThrough:
				bottom[l.From] = true
			case LineOut:
				bottom[l.To] = true
			}
		}
		top := map[int]bool{}
		for _, l := range lay.Rows[i+1].Lines {
			if l.Part == LineThrough || l.Part == LineIn {
				top[l.From] = true
			}
		}
		for k := range bottom {
			if !top[k] {
				t.Fatalf("row %d: lane %d leaves the bottom but row %d has no line there", i, k, i+1)
			}
		}
		for k := range top {
			if !bottom[k] {
				t.Fatalf("row %d: lane %d enters the top without a line above", i+1, k)
			}
		}
	}
}

func TestLayoutCommitsLinear(t *testing.T) {
	lay := LayoutCommits(commits("c:b", "b:a", "a"))
	checkContinuity(t, lay)
	for i, r := range lay.Rows {
		if r.Lane != 0 {
			t.Fatalf("row %d lane %d", i, r.Lane)
		}
	}
	if lay.Lanes != 1 {
		t.Fatalf("lanes = %d", lay.Lanes)
	}
	if n := len(lay.Rows[2].Lines); n != 1 || lay.Rows[2].Lines[0].Part != LineIn {
		t.Fatalf("root row lines = %+v", lay.Rows[2].Lines)
	}
}

func TestLayoutCommitsBranchAndMerge(t *testing.T) {
	// m merges f1 into b1; both fork from b0.
	lay := LayoutCommits(commits("m:b1,f1", "f1:b0", "b1:b0", "b0"))
	checkContinuity(t, lay)
	want := []int{0, 1, 0, 0}
	for i, r := range lay.Rows {
		if r.Lane != want[i] {
			t.Fatalf("row %d lane %d, want %d", i, r.Lane, want[i])
		}
	}
	if !lay.Rows[0].Merge || lay.Rows[1].Merge {
		t.Fatal("merge flag wrong")
	}
	if lay.Rows[0].Color == lay.Rows[1].Color {
		t.Fatal("the merged branch should get its own colour")
	}
	// b0 receives both lines.
	in := 0
	for _, l := range lay.Rows[3].Lines {
		if l.Part == LineIn && l.To == 0 {
			in++
		}
	}
	if in != 2 {
		t.Fatalf("fork point gets %d incoming lines, want 2", in)
	}
	if lay.Lanes != 2 {
		t.Fatalf("lanes = %d, want 2", lay.Lanes)
	}
}

func TestLayoutCommitsReusesLanes(t *testing.T) {
	// Two sequential feature branches merged one after the other: the
	// second reuses lane 1.
	lay := LayoutCommits(commits(
		"m2:x,g1", "g1:x", "x:m1", "m1:a,f1", "f1:a", "a",
	))
	checkContinuity(t, lay)
	if lay.Rows[1].Lane != 1 || lay.Rows[4].Lane != 1 {
		t.Fatalf("feature lanes = %d, %d; want 1, 1", lay.Rows[1].Lane, lay.Rows[4].Lane)
	}
	if lay.Lanes != 2 {
		t.Fatalf("lanes = %d, want 2", lay.Lanes)
	}
}

func TestLayoutCommitsMissingParents(t *testing.T) {
	// A truncated history: b's parent is not listed; m merges a commit
	// that is not listed either.
	lay := LayoutCommits(commits("m:b,zz", "b:yy"))
	if lay.Lanes != 1 {
		t.Fatalf("missing parents must not hold lanes: %d", lay.Lanes)
	}
	last := lay.Rows[1].Lines
	out := false
	for _, l := range last {
		if l.Part == LineOut {
			out = true
		}
	}
	if !out {
		t.Fatal("a missing first parent should draw the line leaving the row")
	}
}

func TestLayoutCommitsRandomContinuity(t *testing.T) {
	cs := synthHistory(3000)
	lay := LayoutCommits(cs)
	checkContinuity(t, lay)
	if lay.Lanes > 12 {
		t.Fatalf("lanes = %d for at most 5 parallel branches", lay.Lanes)
	}
}

// synthHistory builds n commits newest first: a main line with up to five
// feature branches that fork and merge back.
func synthHistory(n int) []Commit {
	seed := uint32(99)
	rnd := func(k int) int {
		seed ^= seed << 13
		seed ^= seed >> 17
		seed ^= seed << 5
		return int(seed % uint32(k))
	}
	var made []Commit
	add := func(ps ...string) string {
		h := fmt.Sprintf("%08x", len(made))
		made = append(made, Commit{Hash: h, Parents: ps, Subject: "commit " + h, Author: "a", Time: time.Unix(int64(len(made))*60, 0)})
		return h
	}
	main := add()
	var feats []string
	for len(made) < n {
		switch r := rnd(100); {
		case r < 8 && len(feats) < 5:
			feats = append(feats, main)
		case r < 16 && len(feats) > 0:
			k := rnd(len(feats))
			main = add(main, feats[k])
			feats = append(feats[:k], feats[k+1:]...)
		case r < 45 && len(feats) > 0:
			k := rnd(len(feats))
			feats[k] = add(feats[k])
		default:
			main = add(main)
		}
	}
	out := make([]Commit, len(made))
	for i := range made {
		out[i] = made[len(made)-1-i]
	}
	return out
}

func BenchmarkLayoutCommits10k(b *testing.B) {
	cs := synthHistory(10000)
	b.ReportAllocs()
	for b.Loop() {
		LayoutCommits(cs)
	}
}

func TestGitGraphSelectAndKeys(t *testing.T) {
	cs := synthHistory(500)
	var sel string
	var v *GitGraphView
	var clicked []string
	d := ui.NewDriver(func(c *ui.Context) {
		v = GitGraph(c, cs).Height(300).Selected(&sel).Show()
		if cm, ok := v.Clicked(); ok {
			clicked = append(clicked, cm.Hash)
		}
	}, ui.DriverOptions{Width: 900, Height: 400, Accessibility: true})
	d.Settle()
	r := v.Rect()
	d.Click(geom.Pt(r.X+400, r.Y+30*2+15))
	if sel != cs[2].Hash {
		t.Fatalf("selected %q, want row 2 %q", sel, cs[2].Hash)
	}
	d.Key(input.KeyDown, 0)
	if sel != cs[3].Hash {
		t.Fatalf("Down: %q, want %q", sel, cs[3].Hash)
	}
	d.Key(input.KeyEnd, 0)
	d.Settle()
	if sel != cs[len(cs)-1].Hash {
		t.Fatalf("End: %q", sel)
	}
	// The last row is now in view.
	if _, ok := d.A11yFind(a11y.RoleOption, cs[len(cs)-1].Subject); !ok {
		t.Fatal("End did not scroll the last commit into view")
	}
	if len(clicked) != 3 {
		t.Fatalf("clicked events = %d, want 3", len(clicked))
	}
	lb, ok := d.A11yFind(a11y.RoleListbox, "提交图")
	if !ok || lb.Description != "500 个提交" {
		t.Fatalf("listbox = %+v %v", lb, ok)
	}
	// Selecting from the app scrolls nothing but marks the row.
	sel = cs[0].Hash
	d.Key(input.KeyHome, 0)
	if sel != cs[0].Hash {
		t.Fatalf("Home: %q", sel)
	}
	rep := d.Audit()
	for _, f := range rep.Issues {
		if f.Kind == ui.AuditMissingName || f.Kind == ui.AuditNotKeyboard || f.Kind == ui.AuditNoRole {
			t.Errorf("audit: %+v", f)
		}
	}
}

func TestGitGraphRelativeTime(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	loc := ui.ZhCN
	cases := map[time.Duration]string{
		10 * time.Second: "刚刚",
		5 * time.Minute:  "5 分钟前",
		3 * time.Hour:    "3 小时前",
		48 * time.Hour:   "2 天前",
	}
	for d, want := range cases {
		if got := fmtWhen(now.Add(-d), now, loc); got != want {
			t.Errorf("%v ago = %q, want %q", d, got, want)
		}
	}
	if got := fmtWhen(now.Add(-60*24*time.Hour), now, loc); got != "2026-09-15" {
		t.Errorf("old date = %q", got)
	}
}
