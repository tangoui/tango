// Package graph holds the graph widgets of the Tango self-drawn UI: a
// node-link diagram canvas (Diagram), an entity-relationship preset built on
// it (ER) and a git commit graph (GitGraph).
//
// A Diagram draws an application-owned *Graph: nodes (built-in templates or
// a custom painter) joined by straight, orthogonal or Bézier edges with
// arrows, labels and port anchors. The canvas pans (drag the background,
// Space+drag, the wheel, two-finger touch), zooms (Ctrl/⌘+wheel, pinch,
// toolbar buttons), selects (click, Shift/⌘+click, Shift+drag marquee),
// drags nodes (optionally snapped to a grid) and shows a minimap. Only the
// visible part of the graph is drawn, so graphs of thousands of nodes stay
// smooth.
//
//	g := &graph.Graph{Nodes: nodes, Edges: edges}
//	v := graph.Diagram(c, g).Height(480).Grid(16).Snap().MiniMap().AutoLayout(graph.LayoutOptions{}).Show()
//	if id, ok := v.DoubleClicked(); ok { open(id) }
//
// Layout computes a layered (Sugiyama-style) placement: cycle removal,
// longest-path layering, barycentric crossing reduction and an iterative
// coordinate assignment; edges spanning several layers get bend points.
//
//	graph.Layout(g, graph.LayoutOptions{Direction: graph.LeftRight})
//
// ER draws database tables (a name header and one row per column with its
// type and primary / foreign key marks) and connects foreign keys from the
// referencing column row to the referenced one:
//
//	graph.ER(c, tables, relations).Height(560).Show()
//
// GitGraph lays commits out in lanes (LayoutCommits: branch lines, merge
// curves, per-branch colours) and shows them in a virtual list with ref
// badges, author, date and hash columns, hover and selection. Only the rows
// in view are built, so histories of tens of thousands of commits scroll
// smoothly.
//
//	graph.GitGraph(c, commits).Selected(&hash).Height(420).Show()
//
// Geometry: the diagram keeps its left-to-right geometry in right-to-left
// scopes (a diagram's coordinates are data, like a chart's), while the
// toolbar and minimap move to the mirrored corners; GitGraph mirrors its
// columns with the scope like any list.
//
// Experimental: this package's API may change in any 0.x release (see
// docs/release.md, "API 稳定性").
package graph
