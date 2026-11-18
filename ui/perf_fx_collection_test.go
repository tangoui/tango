package ui

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// Collection and large-data fixtures (Task 6). Every value a component can
// change lives in the driver's Context (State) and is only changed by the
// component's own events (clicks on options, rows, page buttons and move
// buttons, wheel input). Dropdown collections build closed like the Select
// seed: their interaction opens the popup through the trigger and picks an
// option; their Large pages build open (the public Open) so the n options
// are rendered. Large pages use the true n: the virtualized components
// build only the rows in view, the others (List, Tree, InfiniteScroll,
// Cascader columns) build every row.

func perfCollID(key string) ID { return hashID("perf.coll." + key) }

// perfCollFind is the rect of the widget labelled label, else of a node
// whose text is label (top-most layer first, see Driver.FindText).
func perfCollFind(r *perfRun, label string) geom.Rect {
	if box, ok := r.D.Find(label); ok {
		return box
	}
	if box, ok := r.D.FindText(label); ok {
		return box
	}
	panic("perf: collection target not found: " + label)
}

// perfCollClick clicks the widget / text labelled label.
func perfCollClick(r *perfRun, label string) { r.D.Click(perfCollFind(r, label).Center()) }

// perfCollItem is the label of item i (1-based in the text).
func perfCollItem(i int) string { return "项目 " + strconv.Itoa(i+1) }

// perfCollOpts are n options labelled perfCollItem.
func perfCollOpts(n int) []Option {
	opts := make([]Option, n)
	for i := range opts {
		s := perfCollItem(i)
		opts[i] = Option{Label: s, Value: s}
	}
	return opts
}

var perfCollCities = Opts("北京", "上海", "杭州", "南京", "苏州", "成都")

// perfCollSelectOpts are the options of the typical VirtualSelect: a list
// long enough to scroll (built once; never mutated).
var perfCollSelectOpts = perfCollOpts(1000)

var perfCollRegions = []TreeSelectNode{
	{Label: "浙江省", Value: "zj", Children: []TreeSelectNode{
		{Label: "杭州市", Value: "hz", Children: []TreeSelectNode{
			{Label: "西湖区", Value: "xh"}, {Label: "余杭区", Value: "yh"}, {Label: "滨江区", Value: "bj"},
		}},
		{Label: "宁波市", Value: "nb"},
	}},
	{Label: "江苏省", Value: "js", Children: []TreeSelectNode{
		{Label: "南京市", Value: "nj"}, {Label: "苏州市", Value: "sz"},
	}},
	{Label: "北京市", Value: "bjs"},
}

var perfCollCascade = []CascaderOption{
	{Label: "浙江省", Value: "zj", Children: []CascaderOption{
		{Label: "杭州市", Value: "hz", Children: []CascaderOption{
			{Label: "西湖区", Value: "xh"}, {Label: "余杭区", Value: "yh"}, {Label: "滨江区", Value: "bj"},
		}},
		{Label: "宁波市", Value: "nb"},
	}},
	{Label: "江苏省", Value: "js", Children: []CascaderOption{
		{Label: "南京市", Value: "nj"}, {Label: "苏州市", Value: "sz"},
	}},
	{Label: "北京市", Value: "bjs"},
}

// perfCollTreeKids is the fan-out of the large trees: n nodes are n/100
// groups of one parent and 99 leaves (every group expanded).
const perfCollTreeKids = 99

// perfCollTreeNodes is a tree of exactly n nodes (n a multiple of 100):
// groups "分组 g" with leaves titled perfCollItem; it returns the parent
// keys too (expand them all to reach every node).
func perfCollTreeNodes(n int) ([]TreeNode, []string) {
	groups := n / (perfCollTreeKids + 1)
	out := make([]TreeNode, groups)
	keys := make([]string, groups)
	leaf := 0
	for g := range out {
		kids := make([]TreeNode, perfCollTreeKids)
		for k := range kids {
			kids[k] = TreeNode{Key: "n" + strconv.Itoa(leaf), Title: perfCollItem(leaf)}
			leaf++
		}
		keys[g] = "g" + strconv.Itoa(g)
		out[g] = TreeNode{Key: keys[g], Title: "分组 " + strconv.Itoa(g+1), Children: kids}
	}
	return out, keys
}

// perfCollTreeSelectNodes is perfCollTreeNodes for TreeSelect.
func perfCollTreeSelectNodes(n int) []TreeSelectNode {
	groups := n / (perfCollTreeKids + 1)
	out := make([]TreeSelectNode, groups)
	leaf := 0
	for g := range out {
		kids := make([]TreeSelectNode, perfCollTreeKids)
		for k := range kids {
			kids[k] = TreeSelectNode{Label: perfCollItem(leaf), Value: "n" + strconv.Itoa(leaf)}
			leaf++
		}
		out[g] = TreeSelectNode{Label: "分组 " + strconv.Itoa(g+1), Value: "g" + strconv.Itoa(g), Children: kids}
	}
	return out
}

// perfCollCascadeOpts are n leaf options in the first column.
func perfCollCascadeOpts(n int) []CascaderOption {
	out := make([]CascaderOption, n)
	for i := range out {
		out[i] = CascaderOption{Label: perfCollItem(i), Value: "n" + strconv.Itoa(i)}
	}
	return out
}

// perfCollDemoTree is the typical Tree / VirtualTree data (Ant's demo).
var perfCollDemoTree = []TreeNode{
	{Key: "0-0", Title: "parent 1", Children: []TreeNode{
		{Key: "0-0-0", Title: "parent 1-0", Children: []TreeNode{
			{Key: "0-0-0-0", Title: "leaf 1-0-0"},
			{Key: "0-0-0-1", Title: "leaf 1-0-1", Disabled: true},
			{Key: "0-0-0-2", Title: "leaf 1-0-2"},
		}},
		{Key: "0-0-1", Title: "parent 1-1", Children: []TreeNode{
			{Key: "0-0-1-0", Title: "sss"},
		}},
		{Key: "0-0-2", Title: "parent 1-2", Children: []TreeNode{
			{Key: "0-0-2-0", Title: "leaf 1-2-0"},
			{Key: "0-0-2-1", Title: "leaf 1-2-1"},
		}},
	}},
}

var perfCollTransferItems = func() []TransferItem {
	var out []TransferItem
	for i, t := range []string{"内容一", "内容二", "内容三", "内容四", "内容五", "内容六", "内容七", "内容八"} {
		out = append(out, TransferItem{Key: "k" + strconv.Itoa(i+1), Title: t, Disabled: i == 3})
	}
	return out
}()

func perfCollTransferLarge(n int) []TransferItem {
	out := make([]TransferItem, n)
	for i := range out {
		out[i] = TransferItem{Key: "k" + strconv.Itoa(i+1), Title: perfCollItem(i)}
	}
	return out
}

const perfCollDesc = "Ant Design, a design language for background applications, is refined by Ant UED Team"

var perfCollNames = []string{"Ant Design Title 1", "Ant Design Title 2", "Ant Design Title 3", "Ant Design Title 4", "Ant Design Title 5", "Ant Design Title 6", "Ant Design Title 7", "Ant Design Title 8"}

// perfCollListRow is a list row: avatar + title + description.
func perfCollListRow(c *Context, title string) {
	ListItemMeta(c, func() { Avatar(c, title[:1]) }, title, perfCollDesc)
}

// perfCollScrollBox wraps a non-virtualized Large collection in the page's
// scroll container (Large pages only).
func perfCollScrollBox(c *Context, fn func()) {
	Scroll(c, fn, layout.Size(960, 760))
}

// perfCollLogBuffer is a buffer of n log lines with levels (like the
// gallery's large log).
func perfCollLogBuffer(n int) *LogBuffer {
	buf := NewLogBuffer(0)
	lv := [...]LogLevel{LogInfo, LogDebug, LogWarn, LogError}
	for i := 0; i < n; i++ {
		buf.AppendLevel(lv[i%7%4], "2026-11-14 10:"+perfColl2(i/60000%60)+":"+perfColl2(i/1000%60)+" worker-"+strconv.Itoa(i%16)+": processed job "+strconv.Itoa(i+1))
	}
	return buf
}

func perfColl2(v int) string {
	if v < 10 {
		return "0" + strconv.Itoa(v)
	}
	return strconv.Itoa(v)
}

// perfCollOpenPick opens a dropdown collection with a click on its
// trigger (perfCollOpen) and clicks the option labelled pick in the popup.
func perfCollOpenPick(r *perfRun, pick string) {
	perfCollOpen(r)
	perfCollClick(r, pick)
}

// perfCollOpen clicks the trigger field 40px before its end: past the
// selected tags, before the arrow.
func perfCollOpen(r *perfRun) {
	r.D.Click(geom.Pt(r.Target.Right()-40, r.Target.Y+r.Target.H/2))
}

// perfCollBlur clicks the page padding: blur and unhover.
func perfCollBlur(r *perfRun) { r.D.Click(perfOutside) }

// perfCollWheel wheels at the center of Target: dy down on even steps,
// back up on odd ones.
func perfCollWheel(dy float32) func(*perfRun, int) {
	return func(r *perfRun, i int) {
		d := dy
		if i%2 == 1 {
			d = -dy
		}
		r.D.Scroll(r.Center(), d)
		r.D.Move(perfOutside)
	}
}

// perfCollLogSettle asks for a second frame on a driver's first build.
// Minimal internal preset (fixture rule 4) for a LogView defect owned by
// perf/memory-logview and not fixed here: LogView lays its first frame out
// for the 200px fallback viewport (fillViewH: the real height is known
// only after the first paint) and, unlike the components that need their
// geometry (Mentions, Splitter, Popover: c.animating = true), asks for no
// further frame, so the settled screen lacks rows until another event.
// The preset does what the component should; TestPerfCollLogViewFirstFrame
// shows the defect without it and fails once LogView is fixed (then
// delete this function).
func perfCollLogSettle(c *Context) {
	State(c, perfCollID("log.settle"), func() bool { c.animating = true; return true })
}

// perfCollLogWheel is LogView's large interaction: it follows the newest
// line, so it wheels up for 20 steps, then down again.
func perfCollLogWheel(r *perfRun, i int) {
	dy := float32(-120)
	if i%40 >= 20 {
		dy = 120
	}
	r.D.Inject(input.Scroll{Pos: r.Center(), DY: dy})
}

// perfCollTransferMove checks label on side from (0 left, 1 right) and
// moves it to the other side with the move button between the lists.
func perfCollTransferMove(r *perfRun, label string, from int) {
	perfCollClick(r, label)
	r.D.Click(perfCollTransferButton(r, from))
}

// perfCollTransferButton is the center of the move button of side from:
// the buttons are the only interactive widgets between the two lists.
func perfCollTransferButton(r *perfRun, from int) geom.Point {
	var btns []geom.Rect
	mid := r.Target.X + r.Target.W/2
	for _, h := range r.D.r.c.hits {
		if h.rect.X < mid && h.rect.Right() > mid && h.rect.W < 80 {
			btns = append(btns, h.rect)
		}
	}
	if len(btns) != 2 {
		panic("perf: transfer move buttons not found: " + strconv.Itoa(len(btns)))
	}
	if btns[0].Y > btns[1].Y {
		btns[0], btns[1] = btns[1], btns[0]
	}
	return btns[from].Center()
}

func init() {
	registerPerf(
		perfFixture{
			// Picking an option toggles it (the popup stays open, a tag is
			// added); step 1 toggles it back, closes and blurs.
			Name: "MultiSelect",
			Build: func(c *Context) {
				vals := State(c, perfCollID("multi"), func() []string { return []string{"杭州"} })
				MultiSelect(c, vals, perfCollCities).Placeholder("请选择城市").Width(320)
			},
			Interact: func(r *perfRun, i int) {
				if i%2 == 0 {
					perfCollOpenPick(r, "上海")
					return
				}
				perfCollClick(r, "上海")
				r.D.Key(input.KeyEscape, 0)
				perfCollBlur(r)
			},
			Large: func(n int) func(*Context) {
				opts := perfCollOpts(n)
				return func(c *Context) {
					vals := State(c, perfCollID("multi.large"), func() []string { return nil })
					MultiSelect(c, vals, opts).Placeholder("请选择").Width(320).Open()
				}
			},
		},
		perfFixture{
			Name: "VirtualSelect",
			Build: func(c *Context) {
				v := State(c, perfCollID("vselect"), func() string { return perfCollItem(0) })
				VirtualSelect(c, v, perfCollSelectOpts).Width(240)
			},
			Interact: func(r *perfRun, i int) {
				if i%2 == 0 {
					perfCollOpenPick(r, perfCollItem(2))
					return
				}
				perfCollOpenPick(r, perfCollItem(0))
				perfCollBlur(r)
			},
			Large: func(n int) func(*Context) {
				opts := perfCollOpts(n)
				return func(c *Context) {
					v := State(c, perfCollID("vselect.large"), func() string { return "" })
					VirtualSelect(c, v, opts).Placeholder("请选择").Width(240).Open()
				}
			},
		},
		perfFixture{
			Name: "TreeSelect",
			Build: func(c *Context) {
				v := State(c, perfCollID("treeselect"), func() string { return "xh" })
				TreeSelect(c, v, perfCollRegions).Width(240)
			},
			Interact: func(r *perfRun, i int) {
				if i%2 == 0 {
					perfCollOpenPick(r, "余杭区")
					return
				}
				perfCollOpenPick(r, "西湖区")
				perfCollBlur(r)
			},
			Large: func(n int) func(*Context) {
				nodes := perfCollTreeSelectNodes(n)
				return func(c *Context) {
					v := State(c, perfCollID("treeselect.large"), func() string { return "" })
					TreeSelect(c, v, nodes).Placeholder("请选择").DefaultExpandAll().Width(240).Open()
				}
			},
		},
		perfFixture{
			Name: "Cascader",
			Build: func(c *Context) {
				v := State(c, perfCollID("cascader"), func() []string { return []string{"zj", "hz", "xh"} })
				Cascader(c, v, perfCollCascade).Width(240)
			},
			Interact: func(r *perfRun, i int) {
				if i%2 == 0 {
					perfCollOpenPick(r, "余杭区")
					return
				}
				perfCollOpenPick(r, "西湖区")
				perfCollBlur(r)
			},
			Large: func(n int) func(*Context) {
				opts := perfCollCascadeOpts(n)
				return func(c *Context) {
					v := State(c, perfCollID("cascader.large"), func() []string { return nil })
					Cascader(c, v, opts).Placeholder("请选择").Width(240).Open()
				}
			},
		},
		perfFixture{
			Name: "CascaderPanel",
			Build: func(c *Context) {
				v := State(c, perfCollID("cascader.panel"), func() []string { return []string{"zj", "hz", "xh"} })
				CascaderPanel(c, v, perfCollCascade)
			},
			Interact: func(r *perfRun, i int) {
				if i%2 == 0 {
					perfCollClick(r, "余杭区")
					return
				}
				perfCollClick(r, "西湖区")
				perfCollBlur(r)
			},
			Large: func(n int) func(*Context) {
				opts := perfCollCascadeOpts(n)
				return func(c *Context) {
					v := State(c, perfCollID("cascader.panel.large"), func() []string { return nil })
					CascaderPanel(c, v, opts)
				}
			},
		},
		perfFixture{
			// Check an item and move it right; step 1 moves it back.
			Name: "Transfer",
			Build: func(c *Context) {
				target := State(c, perfCollID("transfer"), func() []string { return []string{"k6"} })
				Transfer(c, perfCollTransferItems, target).Titles("候选", "已选")
			},
			Interact: func(r *perfRun, i int) {
				if i%2 == 0 {
					perfCollTransferMove(r, "内容一", 0)
				} else {
					perfCollTransferMove(r, "内容一", 1)
				}
				r.D.Move(perfOutside)
			},
			Large: func(n int) func(*Context) {
				items := perfCollTransferLarge(n)
				return func(c *Context) {
					target := State(c, perfCollID("transfer.large"), func() []string { return nil })
					Transfer(c, items, target).ListSize(320, 600)
				}
			},
			LargeInteract: func(r *perfRun, i int) {
				perfCollTransferMove(r, perfCollItem(0), i%2)
				r.D.Move(perfOutside)
			},
		},
		perfFixture{
			// ListItemMeta is a non-interactive visual helper (avatar,
			// title, description): every row is one, and the page switch
			// rebuilds them with other titles.
			Name: "List", Covers: []string{"ListItemMeta"},
			Build: func(c *Context) {
				page := State(c, perfCollID("list.page"), func() int { return 1 })
				List(c, len(perfCollNames)).Pagination(page, 3).Item(func(i int) {
					perfCollListRow(c, perfCollNames[i])
				})
			},
			Interact: func(r *perfRun, i int) {
				if i%2 == 0 {
					perfCollClick(r, "2")
				} else {
					perfCollClick(r, "1")
				}
				r.D.Move(perfOutside)
			},
			Large: func(n int) func(*Context) {
				return func(c *Context) {
					perfCollScrollBox(c, func() {
						List(c, n).Item(func(i int) { ListItemMeta(c, nil, perfCollItem(i), perfCollDesc) })
					})
				}
			},
			// The same items in List's own 960×760 virtual viewport.
			LargeVirtual: func(n int) func(*Context) {
				return func(c *Context) {
					Column(c, func() {
						List(c, n).Height(760).Virtual().Item(func(i int) { ListItemMeta(c, nil, perfCollItem(i), perfCollDesc) })
					}, layout.W(960))
				}
			},
			NA: map[string]string{"Large100k": perfNANonVirtual},
		},
		perfFixture{
			// An action link toggles the row's favourite flag (its label).
			Name: "ListActions",
			Build: func(c *Context) {
				fav := State(c, perfCollID("list.fav"), func() []bool { return make([]bool, 3) })
				List(c, 3).Item(func(i int) {
					perfCollListRow(c, perfCollNames[i])
					ListActions(c, func() {
						Link(c, "编辑")
						label := "收藏"
						if (*fav)[i] {
							label = "取消收藏"
						}
						if Link(c, label).Clicked() {
							(*fav)[i] = !(*fav)[i]
						}
					})
				})
			},
			Interact: func(r *perfRun, i int) {
				if i%2 == 0 {
					perfCollClick(r, "收藏")
				} else {
					perfCollClick(r, "取消收藏")
				}
				r.D.Move(perfOutside)
			},
		},
		perfFixture{
			// Clicking a row selects it (the app marks it); clicking it
			// again clears the selection.
			Name: "Listy",
			Build: func(c *Context) {
				sel := State(c, perfCollID("listy"), func() int { return -1 })
				l := Listy(c, 200, func(i int) {
					Row(c, func() {
						Text(c, perfCollItem(i)).Layout(layout.Grow(1))
						if *sel == i {
							Tag(c, "已选择").Color("blue")
						}
					}, layout.Middle())
				}).Group(func(i int) string { return string(rune('A' + i/20)) }, nil).Sticky().Virtual().Height(360).Show()
				if i, ok := l.Clicked(); ok {
					if *sel == i {
						*sel = -1
					} else {
						*sel = i
					}
				}
			},
			Interact: func(r *perfRun, _ int) {
				perfCollClick(r, perfCollItem(1))
				r.D.Move(perfOutside)
			},
			Large: func(n int) func(*Context) {
				return func(c *Context) {
					Listy(c, n, func(i int) { Text(c, perfCollItem(i)) }).
						Group(func(i int) string { return string(rune('A' + i*26/n)) }, nil).Sticky().Virtual().Height(760).Show()
				}
			},
		},
		perfFixture{
			Name: "VirtualList",
			Build: func(c *Context) {
				VirtualList(c, 1000, 40, func(i int) {
					Row(c, func() {
						Text(c, perfCollItem(i)).Layout(layout.Grow(1))
						Text(c, "#"+strconv.Itoa(100000+i)).Tertiary()
					}, layout.Middle(), layout.PadVH(0, 16), layout.Grow(1))
				}).Height(320).Layout(layout.W(480))
			},
			Interact: perfCollWheel(120),
			Large: func(n int) func(*Context) {
				return func(c *Context) {
					VirtualList(c, n, 40, func(i int) {
						Row(c, func() { Text(c, perfCollItem(i)) }, layout.Middle(), layout.PadVH(0, 16), layout.Grow(1))
					}).Height(760).Layout(layout.W(480))
				}
			},
		},
		perfFixture{
			// Clicking a node selects it; step 1 selects the first leaf
			// again. Leaves only: the Element flavor expands / collapses a
			// parent whose title is clicked.
			Name: "Tree",
			Build: func(c *Context) {
				exp := State(c, perfCollID("tree.exp"), func() []string { return []string{"0-0", "0-0-0", "0-0-1"} })
				sel := State(c, perfCollID("tree.sel"), func() []string { return []string{"0-0-0-0"} })
				chk := State(c, perfCollID("tree.chk"), func() []string { return []string{"0-0-0-0", "0-0-1"} })
				Tree(c, perfCollDemoTree).Expanded(exp).Selected(sel).Checkable(chk).Show()
			},
			Interact: func(r *perfRun, i int) {
				if i%2 == 0 {
					perfCollClick(r, "leaf 1-0-2")
				} else {
					perfCollClick(r, "leaf 1-0-0")
					perfCollBlur(r)
				}
				r.D.Move(perfOutside)
			},
			Large: func(n int) func(*Context) {
				nodes, keys := perfCollTreeNodes(n)
				return func(c *Context) {
					exp := State(c, perfCollID("tree.large.exp"), func() []string { return append([]string(nil), keys...) })
					perfCollScrollBox(c, func() { Tree(c, nodes).Expanded(exp).Show() })
				}
			},
			// The same nodes in Tree's own Height(760) viewport, which builds
			// only the rows in view (data_tree.go's windowed path, not
			// VirtualTree's: the VirtualTree fixture times that one).
			LargeVirtual: func(n int) func(*Context) {
				nodes, keys := perfCollTreeNodes(n)
				return func(c *Context) {
					exp := State(c, perfCollID("tree.virtual.exp"), func() []string { return append([]string(nil), keys...) })
					Column(c, func() { Tree(c, nodes).Expanded(exp).Height(760).Show() }, layout.W(960))
				}
			},
			// The Height viewport with DataVersion: the walk and the
			// flattened rows are cached, so a frame follows the viewport.
			LargeVersioned: func(n int) func(*Context) {
				nodes, keys := perfCollTreeNodes(n)
				return func(c *Context) {
					exp := State(c, perfCollID("tree.versioned.exp"), func() []string { return append([]string(nil), keys...) })
					Column(c, func() { Tree(c, nodes).Expanded(exp).Height(760).DataVersion(1).Show() }, layout.W(960))
				}
			},
			NA: map[string]string{"Large100k": perfNANonVirtual},
		},
		perfFixture{
			// Clicking a node makes it current; step 1 clicks the first one
			// back.
			Name: "VirtualTree",
			Build: func(c *Context) {
				exp := State(c, perfCollID("vtree.exp"), func() []string { return []string{"0-0", "0-0-0", "0-0-1", "0-0-2"} })
				chk := State(c, perfCollID("vtree.chk"), func() []string { return []string{"0-0-0-0"} })
				cur := State(c, perfCollID("vtree.cur"), func() string { return "0-0-0-0" })
				VirtualTree(c, perfCollDemoTree).Expanded(exp).Checkable(chk).Current(cur).Height(320).Show()
			},
			Interact: func(r *perfRun, i int) {
				if i%2 == 0 {
					perfCollClick(r, "leaf 1-2-0")
				} else {
					perfCollClick(r, "leaf 1-0-0")
					perfCollBlur(r)
				}
				r.D.Move(perfOutside)
			},
			Large: func(n int) func(*Context) {
				nodes, keys := perfCollTreeNodes(n)
				return func(c *Context) {
					exp := State(c, perfCollID("vtree.large.exp"), func() []string { return append([]string(nil), keys...) })
					VirtualTree(c, nodes).Expanded(exp).Height(760).Show()
				}
			},
		},
		perfFixture{
			// Wheeling to the bottom asks for more (NeedMore): the app
			// appends a page of 8 until 40 rows; step 1 wheels back to the
			// top.
			Name: "InfiniteScroll",
			Build: func(c *Context) {
				count := State(c, perfCollID("infinite"), func() int { return 8 })
				inf := InfiniteScroll(c, func() {
					for i := 0; i < *count; i++ {
						Key(c, i, func() { perfCollListRow(c, perfCollItem(i)) })
					}
				}).Height(260).Distance(20).Done(*count >= 40).Layout(layout.W(480))
				if inf.NeedMore() {
					*count += 8
				}
			},
			Interact: perfCollWheel(4000),
			Large: func(n int) func(*Context) {
				return func(c *Context) {
					InfiniteScroll(c, func() {
						for i := 0; i < n; i++ {
							Key(c, i, func() { ListItemMeta(c, nil, perfCollItem(i), perfCollDesc) })
						}
					}).Height(760).Done(true).Layout(layout.W(960))
				}
			},
			// The virtual infinite list is Listy(...).Virtual().LoadMore +
			// EndReached, timed by the Listy fixture.
			NA: map[string]string{"Large100k": "closure: 接收任意闭包，组件看不到“项”，无法窗口化；虚拟版无限滚动用 Listy(...).Virtual().LoadMore + EndReached（Listy 已计时）"},
		},
		perfFixture{
			// The viewer follows the newest line: step 0 wheels up (it stops
			// following), step 1 wheels back down to the bottom.
			Name: "LogView",
			Build: func(c *Context) {
				buf := State(c, perfCollID("log"), func() *LogBuffer { return perfCollLogBuffer(200) })
				LogView(c, *buf).Height(320).LineNumbers(true).Layout(layout.W(960))
				perfCollLogSettle(c)
			},
			Interact: perfCollWheel(-120),
			Large: func(n int) func(*Context) {
				return func(c *Context) {
					buf := State(c, perfCollID("log.large"), func() *LogBuffer { return perfCollLogBuffer(n) })
					LogView(c, *buf).Height(760).LineNumbers(true).Layout(layout.W(960))
					perfCollLogSettle(c)
				}
			},
			LargeInteract: perfCollLogWheel,
		},
	)
}

// perfCollState reads the fixture value key of the driver's Context.
func perfCollState[T any](d *Driver, key string) T { return *State[T](d.r.c, perfCollID(key), nil) }

// perfCollScrolls are the driver's retained scroll states (read only).
func perfCollScrolls(d *Driver) []*scrollState { return perfNavInternal[scrollState](d) }

// perfCollScrollOffsets snapshots the vertical offset of every retained
// scroll state in the driver's Context, keyed by its State ID (read only).
// Hover, display-list hashes and painted rects play no part: the scroll
// gate must see that the wheel moved content.
func perfCollScrollOffsets(d *Driver) map[ID]float64 {
	out := map[ID]float64{}
	for id, e := range d.r.c.states {
		if s, ok := e.v.(*scrollState); ok {
			out[id] = s.off.Y
		}
	}
	return out
}

// perfCollScrollDY is the vertical offset change of the scroll state that
// moved between the snapshots (0 when none did).
func perfCollScrollDY(before, after map[ID]float64) float64 {
	dy := 0.0
	for id, y := range after {
		if y0, ok := before[id]; ok && y != y0 {
			dy = y - y0
		}
	}
	return dy
}

// perfCollCheckScroll is the scroll gate of the default Large
// interaction, called after perfScroll step 0 ran on r (before: the
// scroll positions at rest). perfScroll wheels down for steps 0-19 and up
// from step 20: steps 0 and 1 must each move a Context-local scroll offset
// down and step 20 must move it back up. A hover repaint at the wheel
// point changes the display list but no offset, so it cannot pass.
func perfCollCheckScroll(r *perfRun, before map[ID]float64) error {
	s0 := perfCollScrollOffsets(r.D)
	if dy := perfCollScrollDY(before, s0); dy <= 0 {
		return fmt.Errorf("default Large scroll step 0 moved no scroll offset down (dy %.0f)", dy)
	}
	perfStep(r, perfScroll, 1)
	s1 := perfCollScrollOffsets(r.D)
	if dy := perfCollScrollDY(s0, s1); dy <= 0 {
		return fmt.Errorf("default Large scroll step 1 moved no scroll offset down (dy %.0f)", dy)
	}
	perfStep(r, perfScroll, 20)
	if dy := perfCollScrollDY(s1, perfCollScrollOffsets(r.D)); dy >= 0 {
		return fmt.Errorf("default Large scroll step 20 (up) moved no scroll offset up (dy %.0f)", dy)
	}
	return nil
}

// perfCollCheckOwnLarge checks the outcome of step 0 of the fixtures with
// their own LargeInteract on their retained state, not on the screen hash
// (a hover repaint changes that too): Transfer moved the first item to the
// target, Tabs switched to tab 2, LogView's wheel moved its view up.
func perfCollCheckOwnLarge(name string, d *Driver, before map[ID]float64) error {
	switch name {
	case "Transfer":
		if got := perfCollState[[]string](d, "transfer.large"); len(got) != 1 || got[0] != "k1" {
			return fmt.Errorf("Transfer Large move: target %v, want [k1]", got)
		}
	case "Tabs":
		if got := *State[string](d.r.c, perfNavID("tabs.large"), nil); got != "t2" {
			return fmt.Errorf("Tabs Large switch: active %q, want t2", got)
		}
	case "LogView":
		if dy := perfCollScrollDY(before, perfCollScrollOffsets(d)); dy >= 0 {
			return fmt.Errorf("LogView Large wheel up moved no scroll offset up (dy %.0f)", dy)
		}
	default:
		return fmt.Errorf("%s: no Large outcome check for its own LargeInteract", name)
	}
	return nil
}

// perfCollExtent is the largest scroll extent (virtual or measured
// content size) along y (x when horiz) of any scroll container.
func perfCollExtent(d *Driver, horiz bool) float64 {
	best := 0.0
	for _, s := range perfCollScrolls(d) {
		v := max(s.extent.Y, s.content.Y)
		if horiz {
			v = max(s.extent.X, s.content.X)
		}
		best = max(best, v)
	}
	return best
}

// perfCollOffset is the largest vertical scroll offset.
func perfCollOffset(d *Driver) string {
	best := 0.0
	for _, s := range perfCollScrolls(d) {
		best = max(best, s.off.Y)
	}
	return strconv.FormatFloat(best, 'f', 0, 64)
}

// perfCollLogText is the text of the log lines in view: LogView paints
// its lines itself (no labelled nodes), its accessibility value lists
// them. Accessibility is turned on (one frame) on first use.
func perfCollLogText(d *Driver) string {
	if d.AccessibilityTree() == nil {
		d.EnableAccessibility()
		d.Frame()
	}
	var out []string
	for _, n := range d.A11yFindAll(a11y.RoleTextbox, "") {
		out = append(out, n.Value)
	}
	return strings.Join(out, "\n")
}

// TestPerfCollInteractOutcome checks the component-specific result of
// each collection interaction (retained value, visible text) at rest,
// after step 0 and after step 1.
func TestPerfCollInteractOutcome(t *testing.T) {
	join := func(key string) func(*Driver) string {
		return func(d *Driver) string { return strings.Join(perfCollState[[]string](d, key), ",") }
	}
	str := func(key string) func(*Driver) string {
		return func(d *Driver) string { return perfCollState[string](d, key) }
	}
	tests := []struct {
		name  string
		probe func(*Driver) string
		want  [3]string
	}{
		{"MultiSelect", join("multi"), [3]string{"杭州", "杭州,上海", "杭州"}},
		{"VirtualSelect", str("vselect"), [3]string{"项目 1", "项目 3", "项目 1"}},
		{"TreeSelect", str("treeselect"), [3]string{"xh", "yh", "xh"}},
		{"Cascader", join("cascader"), [3]string{"zj,hz,xh", "zj,hz,yh", "zj,hz,xh"}},
		{"CascaderPanel", join("cascader.panel"), [3]string{"zj,hz,xh", "zj,hz,yh", "zj,hz,xh"}},
		// antd 6.7 default targetOrder unshift: moved keys go first
		// (transfer/index.js newMoveKeys.concat(targetKeys)).
		{"Transfer", func(d *Driver) string {
			// The item's side: left or right of the move buttons (the
			// middle of the transfer).
			side := "?"
			if box, ok := perfNavShown(d, "内容一"); ok {
				side = "left"
				if box.Center().X > perfTarget(d).Center().X {
					side = "right"
				}
			}
			return join("transfer")(d) + "/" + side
		}, [3]string{"k6/left", "k1,k6/right", "k6/left"}},
		{"List", func(d *Driver) string {
			return strconv.Itoa(perfCollState[int](d, "list.page")) + "," + perfNavHas(d, "Ant Design Title 4") + "," + perfNavHas(d, "Ant Design Title 1")
		}, [3]string{"1,no,yes", "2,yes,no", "1,no,yes"}},
		{"ListActions", func(d *Driver) string {
			var out []string
			for _, f := range perfCollState[[]bool](d, "list.fav") {
				out = append(out, strconv.FormatBool(f))
			}
			return strings.Join(out, ",") + "/" + perfNavHas(d, "取消收藏")
		}, [3]string{"false,false,false/no", "false,false,true/yes", "false,false,false/no"}},
		{"Listy", func(d *Driver) string {
			return strconv.Itoa(perfCollState[int](d, "listy")) + "," + perfNavHas(d, "已选择")
		}, [3]string{"-1,no", "1,yes", "-1,no"}},
		{"VirtualList", perfCollOffset, [3]string{"0", "120", "0"}},
		{"Tree", join("tree.sel"), [3]string{"0-0-0-0", "0-0-0-2", "0-0-0-0"}},
		{"VirtualTree", str("vtree.cur"), [3]string{"0-0-0-0", "0-0-2-0", "0-0-0-0"}},
		{"InfiniteScroll", func(d *Driver) string {
			// Step 0 reaches the bottom: NeedMore appends a page.
			off := "top"
			if perfCollOffset(d) != "0" {
				off = "scrolled"
			}
			return strconv.Itoa(perfCollState[int](d, "infinite")) + "," + off
		}, [3]string{"8,top", "16,scrolled", "16,top"}},
		{"LogView", func(d *Driver) string {
			st := perfNavInternal[logViewState](d)
			if len(st) != 1 {
				return "logviews=" + strconv.Itoa(len(st))
			}
			last := "no"
			if strings.Contains(perfCollLogText(d), "processed job 200") {
				last = "yes"
			}
			return "follow=" + strconv.FormatBool(st[0].follow) + "," + last
		}, [3]string{"follow=true,yes", "follow=false,no", "follow=true,yes"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fx := perfRegistry[tc.name]
			d := newPerfDriver(perfPage(fx.Build), false)
			d.Settle()
			r := &perfRun{D: d}
			for i, want := range tc.want {
				if i > 0 {
					perfStep(r, fx.Interact, i-1)
				}
				if got := tc.probe(d); got != want {
					t.Fatalf("after %d steps: got %q, want %q", i, got, want)
				}
			}
		})
	}
}

// TestPerfCollPopupOpenClose checks the dropdown collections through
// their trigger and their own Interact: the list (text) is not shown at
// rest, a click on the trigger shows it inside an overlay's visible
// content, the fixture's step 0 leaves it open (MultiSelect keeps its list
// open while picking) or closed, and step 1 closes it.
func TestPerfCollPopupOpenClose(t *testing.T) {
	tests := []struct {
		name, text string
		openAfter0 bool
	}{
		{"MultiSelect", "北京", true},
		{"VirtualSelect", "项目 2", false},
		{"TreeSelect", "江苏省", false},
		{"Cascader", "江苏省", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fx := perfRegistry[tc.name]
			check := func(d *Driver, when string, want bool) {
				t.Helper()
				box, ok := perfNavShown(d, tc.text)
				if ok != want {
					t.Fatalf("%s: %q shown = %v, want %v", when, tc.text, ok, want)
				}
				if !ok {
					return
				}
				for _, o := range d.r.c.overlays {
					if perfOverlayContent(o).Intersect(box) == box {
						return
					}
				}
				t.Fatalf("%s: %q is not inside an overlay's visible content", when, tc.text)
			}
			d := newPerfDriver(perfPage(fx.Build), false)
			d.Settle()
			check(d, "at rest", false)
			perfCollOpen(&perfRun{D: d, Target: perfTarget(d)})
			check(d, "after the trigger click", true)

			d = newPerfDriver(perfPage(fx.Build), false)
			d.Settle()
			r := &perfRun{D: d}
			perfStep(r, fx.Interact, 0)
			check(d, "after step 0", tc.openAfter0)
			perfStep(r, fx.Interact, 1)
			check(d, "after step 1", false)
		})
	}
}

// TestPerfCollLarge checks every collection's Large page at n = 100000:
// the first item is rendered (dropdowns are open), all n rows are
// reachable (a non-virtualized list built its last row; a virtualized one
// scrolls over at least n rows of minRow px) and the Large interaction
// works on retained state, never on the screen hash alone (a hover repaint
// at the wheel point changes that too): the default wheel moves a scroll
// offset down and back up (perfCollCheckScroll), an own LargeInteract has
// its outcome checked (perfCollCheckOwnLarge).
func TestPerfCollLarge(t *testing.T) {
	const n = 100000
	leaves := n / (perfCollTreeKids + 1) * perfCollTreeKids
	const minRow = 16
	tests := []struct {
		name, first string
		last        string // non-virtualized: the last row's text is built
		rows        int    // virtualized: rows the scroll extent spans
		horiz       bool   // rows lie along x (tab bar)
	}{
		{name: "MultiSelect", first: perfCollItem(0), rows: n},
		{name: "VirtualSelect", first: perfCollItem(0), rows: n},
		{name: "TreeSelect", first: perfCollItem(0), rows: n},
		{name: "Cascader", first: perfCollItem(0), rows: n},      // columns > 100 options are virtual
		{name: "CascaderPanel", first: perfCollItem(0), rows: n}, // (batch 3 V1)
		{name: "Transfer", first: perfCollItem(0), rows: n},
		{name: "List", first: perfCollItem(0), last: perfCollItem(n - 1)},
		{name: "Listy", first: perfCollItem(0), rows: n},
		{name: "VirtualList", first: perfCollItem(0), rows: n},
		{name: "Tree", first: perfCollItem(0), last: perfCollItem(leaves - 1)},
		{name: "VirtualTree", first: perfCollItem(0), rows: n},
		{name: "InfiniteScroll", first: perfCollItem(0), last: perfCollItem(n - 1)},
		{name: "LogView", first: "processed job " + strconv.Itoa(n), rows: n},
		{name: "Menu", first: "菜单项 1", last: "菜单项 " + strconv.Itoa(n)},
		{name: "Tabs", first: "标签 1", last: "标签 " + strconv.Itoa(n)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fx := perfRegistry[tc.name]
			d := newPerfDriver(perfPage(fx.Large(n)), false)
			d.Settle()
			if _, ok := perfNavShown(d, tc.first); !ok && !strings.Contains(perfCollLogText(d), tc.first) {
				t.Fatalf("%q not rendered", tc.first)
			}
			if tc.last != "" {
				if _, ok := d.FindText(tc.last); !ok {
					t.Fatalf("last row %q not built", tc.last)
				}
			}
			if tc.rows > 0 {
				if ext := perfCollExtent(d, tc.horiz); ext < float64(tc.rows*minRow) {
					t.Fatalf("scroll extent %.0f px is shorter than %d rows of %d px", ext, tc.rows, minRow)
				}
			}
			act := fx.LargeInteract
			defaultScroll := act == nil
			if defaultScroll {
				act = perfScroll
			}
			h := perfListHash(d)
			beforeScroll := perfCollScrollOffsets(d)
			r := &perfRun{D: d}
			perfStep(r, act, 0)
			if defaultScroll {
				if err := perfCollCheckScroll(r, beforeScroll); err != nil {
					t.Fatalf("%v (hash %x → %x)", err, h, perfListHash(d))
				}
			} else if err := perfCollCheckOwnLarge(tc.name, d, beforeScroll); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestPerfCollLogViewFirstFrame shows the LogView defect perfCollLogSettle
// works around: without it, the frame after the settled first one paints
// a different list (the first laid out the rows for a 200px fallback
// viewport and asked for no further frame). When this fails, LogView is
// fixed: delete perfCollLogSettle and this test.
func TestPerfCollLogViewFirstFrame(t *testing.T) {
	d := newPerfDriver(perfPage(func(c *Context) {
		buf := State(c, perfCollID("log"), func() *LogBuffer { return perfCollLogBuffer(200) })
		LogView(c, *buf).Height(320).LineNumbers(true).Layout(layout.W(960))
	}), false)
	d.Settle()
	h0 := perfListHash(d)
	if d.NeedsFrame() {
		t.Fatal("LogView now asks for its second frame: delete perfCollLogSettle")
	}
	d.Frame()
	if perfListHash(d) == h0 {
		t.Fatal("LogView's first settled frame is complete now: delete perfCollLogSettle")
	}
}

// TestPerfCollLargeScrollGuard is the focused regression for the scroll
// gate of TestPerfCollLarge: on a page whose rows fit (nothing scrolls),
// perfScroll's wheel only moves the pointer over a row, whose hover
// background changes the display list. The old hash-only check passed
// that; perfCollCheckScroll must reject it, and accept a real scroll of
// the same rows in a short viewport.
func TestPerfCollLargeScrollGuard(t *testing.T) {
	rows := perfNavLargeItems(5)
	for _, tc := range []struct {
		name   string
		page   func(*Context)
		scroll bool
	}{
		{"hover only", func(c *Context) {
			sel := State(c, perfCollID("guard.menu"), func() string { return "m1" })
			Menu(c, rows, sel).Width(240).Show()
		}, false},
		{"scrolls", func(c *Context) {
			sel := State(c, perfCollID("guard.menu"), func() string { return "m1" })
			Scroll(c, func() { Menu(c, rows, sel).Width(240).Show() }, layout.Size(280, 100))
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := newPerfDriver(perfPage(tc.page), false)
			d.Settle()
			h := perfListHash(d)
			before := perfCollScrollOffsets(d)
			r := &perfRun{D: d}
			perfStep(r, perfScroll, 0)
			if !tc.scroll && perfListHash(d) == h {
				t.Fatal("premise: the wheel must repaint the row hover (hash unchanged)")
			}
			err := perfCollCheckScroll(r, before)
			if tc.scroll && err != nil {
				t.Fatalf("real scroll rejected: %v", err)
			}
			if !tc.scroll && err == nil {
				t.Fatal("hover-only repaint passed the scroll gate")
			}
		})
	}
}
