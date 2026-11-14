package graph

import "time"

// RefKind classifies a git ref for its badge colour.
type RefKind uint8

const (
	RefBranch RefKind = iota // local branch
	RefRemote                // remote-tracking branch (origin/main)
	RefTag
	RefHead // HEAD (or the checked-out branch)
)

// Ref is a branch, tag or HEAD pointing at a commit.
type Ref struct {
	Name string
	Kind RefKind
}

// Commit is one entry of a GitGraph, newest first (git log order: every
// commit above its parents).
type Commit struct {
	Hash    string
	Parents []string
	Refs    []Ref
	Author  string
	Time    time.Time
	Subject string
}

// LinePart says which part of a row a GitLine covers.
type LinePart uint8

const (
	LineThrough LinePart = iota // top to bottom in lane From (= To): a branch passing by
	LineIn                      // from lane From at the top to the commit dot in lane To
	LineOut                     // from the commit dot in lane From to lane To at the bottom
)

// GitLine is one branch line segment inside a row.
type GitLine struct {
	From, To int
	Part     LinePart
	Color    int // index into Style.Lanes (cycled)
}

// GitRow is the graph cell of one commit.
type GitRow struct {
	Lane  int // lane of the commit dot
	Color int
	Merge bool // more than one parent
	Lines []GitLine
}

// GitLayout is the lane layout of a commit list.
type GitLayout struct {
	Rows  []GitRow
	Lanes int // lanes used at most in any row
}

// LayoutCommits assigns lanes to commits (newest first) the way git log
// --graph does: a commit takes the lane that waits for it (the leftmost if
// several lines meet there; the others end in it), its first parent
// continues that lane, further parents join the lane already waiting for
// them or get a free one, and lanes are reused once free. Each new branch line
// gets the next colour. Parents that are not in the list end their line at
// the bottom of the row. It runs in O(commits × lanes).
func LayoutCommits(commits []Commit) *GitLayout {
	index := make(map[string]int, len(commits))
	for i := range commits {
		index[commits[i].Hash] = i
	}
	type slot struct {
		hash  string
		color int
	}
	var lanes []slot
	nextColor := 0
	free := func(prefer int) int {
		for j := range lanes {
			if lanes[j].hash == "" && j != prefer {
				return j
			}
		}
		lanes = append(lanes, slot{})
		return len(lanes) - 1
	}
	out := &GitLayout{Rows: make([]GitRow, len(commits))}
	lines := make([]GitLine, 0, len(commits)*3)
	var incoming []int
	for i := range commits {
		c := &commits[i]
		my := -1
		incoming = incoming[:0]
		for j := range lanes {
			if lanes[j].hash == c.Hash {
				incoming = append(incoming, j)
				if my < 0 {
					my = j
				}
			}
		}
		if my < 0 { // a branch tip: no line from above
			my = free(-1)
			lanes[my] = slot{c.Hash, nextColor}
			nextColor++
		}
		color := lanes[my].color
		start := len(lines)
		for j := range lanes {
			if s := lanes[j]; s.hash != "" && s.hash != c.Hash {
				lines = append(lines, GitLine{From: j, To: j, Part: LineThrough, Color: s.color})
			}
		}
		for _, j := range incoming {
			lines = append(lines, GitLine{From: j, To: my, Part: LineIn, Color: lanes[j].color})
			lanes[j] = slot{}
		}
		lanes[my] = slot{}
		for pi, p := range c.Parents {
			pr, ok := index[p]
			if !ok || pr <= i {
				// Not below in the list: draw the line leaving the row.
				if pi == 0 {
					lines = append(lines, GitLine{From: my, To: my, Part: LineOut, Color: color})
				}
				continue
			}
			if pi == 0 {
				// The first parent continues the lane; other lanes waiting
				// for it converge when it is reached.
				lanes[my] = slot{p, color}
				lines = append(lines, GitLine{From: my, To: my, Part: LineOut, Color: color})
				continue
			}
			k := -1
			for j := range lanes {
				if lanes[j].hash == p {
					k = j
					break
				}
			}
			if k < 0 {
				k = my
				if lanes[my].hash != "" {
					k = free(my)
				}
				lanes[k] = slot{p, nextColor}
				nextColor++
			}
			lines = append(lines, GitLine{From: my, To: k, Part: LineOut, Color: lanes[k].color})
		}
		for len(lanes) > 0 && lanes[len(lanes)-1].hash == "" {
			lanes = lanes[:len(lanes)-1]
		}
		row := &out.Rows[i]
		row.Lane, row.Color, row.Merge = my, color, len(c.Parents) > 1
		row.Lines = lines[start:len(lines):len(lines)]
		used := my + 1
		for _, l := range row.Lines {
			used = max(used, l.From+1, l.To+1)
		}
		out.Lanes = max(out.Lanes, used)
	}
	return out
}
