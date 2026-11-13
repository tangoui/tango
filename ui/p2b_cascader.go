package ui

// Cascader: lazy loading (LoadData + CascaderOption.IsLeaf, a spinner on
// the loading option) and the checked strategy of multiple mode
// (ShowCheckedStrategy).

import (
	"strings"
	"sync"
	"time"

	"github.com/tangoui/tango/ui/theme"
)

// casLazy is the retained LoadData bookkeeping of a Cascader: children
// loaded per option path, paths being loaded and results delivered by done
// callbacks (which may run on another goroutine).
type casLazy struct {
	mu      sync.Mutex
	inbox   []casLoaded
	loaded  map[string][]CascaderOption
	loading map[string]bool
}

type casLoaded struct {
	key  string
	kids []CascaderOption
}

// LoadData enables lazy loading: expanding an option that has no Children
// and is not IsLeaf calls fn with its value path; fn (now or later, on any
// goroutine) calls done with the children. A spinner replaces the arrow
// meanwhile; done(nil) marks the option as a leaf.
//
//	ui.Cascader(c, &v, roots).LoadData(func(path []string, done func([]ui.CascaderOption)) {
//		go func() { done(fetchChildren(path)) }()
//	})
func (w *CascaderW) LoadData(fn func(path []string, done func([]CascaderOption))) *CascaderW {
	w.loadData = fn
	return w
}

// ShowCheckedStrategy chooses the tags of multiple mode (default
// ShowParent: a fully checked parent replaces its children; ShowChild:
// every checked leaf; ShowAll: every checked node). By default the bound
// value holds the checked leaf paths and CheckedPaths returns the
// strategy's paths; with StrategyValue(true) the bound value itself
// follows the strategy (Ant).
func (w *CascaderW) ShowCheckedStrategy(s CheckedStrategy) *CascaderW { w.strategy = s; return w }

// StrategyValue makes the bound value of multiple mode follow the checked
// strategy, as Ant's value does (ShowParent, the default, stores a fully
// checked parent path instead of its leaf paths). Paths written by the
// application are read the same way: a parent path checks its subtree.
// Default false: the value holds the checked leaf paths (what earlier
// versions stored).
func (w *CascaderW) StrategyValue(on bool) *CascaderW { w.stratVal = on; return w }

// CheckedPaths returns the checked paths of multiple mode by the checked
// strategy (the paths shown as tags).
func (w *CascaderW) CheckedPaths() [][]string {
	w.z.run()
	return w.chipPaths
}

// cascaderDefaults applies ConfigProvider.Defaults("Cascader", ...):
// size, status, allowClear, showSearch, placeholder, expandTrigger
// ("hover"), changeOnSelect, maxTagCount, showCheckedStrategy,
// strategyValue.
func (w *CascaderW) cascaderDefaults() {
	c, b := w.c, w.box
	shad2aTreeSize(c, "Cascader", &b.size)
	if v, ok := p2bStr(c, "Cascader", "status"); ok && b.status == "" {
		b.status = v
	}
	if v, ok := p2bBool(c, "Cascader", "strategyValue"); ok {
		w.stratVal = v
	}
	if v, ok := p2bBool(c, "Cascader", "allowClear"); ok {
		b.allowClear = v
	}
	if v, ok := p2bBool(c, "Cascader", "showSearch"); ok {
		b.search = v
	}
	if v, ok := p2bStr(c, "Cascader", "placeholder"); ok {
		b.placeholder = v
	}
	if v, ok := p2bStr(c, "Cascader", "expandTrigger"); ok {
		w.hoverExpand = v == "hover"
	}
	if v, ok := p2bBool(c, "Cascader", "changeOnSelect"); ok {
		w.changeOnSelect = v
	}
	if v, ok := p2bNum(c, "Cascader", "maxTagCount"); ok {
		b.maxCount = int(v)
	}
	w.strategy = p2bStrategy(c, "Cascader", w.strategy)
}

func (w *CascaderW) lz() *casLazy {
	if w.st.lazy == nil {
		w.st.lazy = &casLazy{loaded: map[string][]CascaderOption{}, loading: map[string]bool{}}
	}
	return w.st.lazy
}

// kids returns the children of o at path (loaded ones for lazy options).
func (w *CascaderW) kids(o CascaderOption, path []string) []CascaderOption {
	if len(o.Children) > 0 || w.st.lazy == nil {
		return o.Children
	}
	return w.st.lazy.loaded[casKey(path)]
}

// pendingLazy reports an option whose children have not been loaded yet.
func (w *CascaderW) pendingLazy(o CascaderOption, path []string) bool {
	if w.loadData == nil || o.IsLeaf || len(o.Children) > 0 {
		return false
	}
	if w.st.lazy == nil {
		return true
	}
	_, done := w.st.lazy.loaded[casKey(path)]
	return !done
}

// isLoading reports a running load of path.
func (w *CascaderW) isLoading(path []string) bool {
	return w.st.lazy != nil && w.st.lazy.loading[casKey(path)]
}

// load starts loading the children of path (once).
func (w *CascaderW) load(path []string) {
	if w.loadData == nil {
		return
	}
	z := w.lz()
	key := casKey(path)
	if z.loading[key] {
		return
	}
	if _, ok := z.loaded[key]; ok {
		return
	}
	z.loading[key] = true
	w.loadData(append([]string(nil), path...), func(kids []CascaderOption) {
		z.mu.Lock()
		z.inbox = append(z.inbox, casLoaded{key: key, kids: kids})
		z.mu.Unlock()
	})
	w.pollLoads()
}

// pollLoads moves delivered children into the tree and keeps frames
// coming while loads are running.
func (w *CascaderW) pollLoads() {
	z := w.st.lazy
	if z == nil {
		return
	}
	z.mu.Lock()
	in := z.inbox
	z.inbox = nil
	z.mu.Unlock()
	for _, r := range in {
		z.loaded[r.key] = r.kids
		delete(z.loading, r.key)
		w.c.invalid = true
		w.inheritCheck(r.key)
	}
	if len(z.loading) > 0 {
		w.c.WakeAt(w.c.now.Add(50 * time.Millisecond))
	}
}

// inheritCheck hands the check of a lazy option that was checked before
// its children loaded down to its leaves (the option was never a leaf).
func (w *CascaderW) inheritCheck(key string) {
	if w.multi == nil {
		return
	}
	k := -1
	for i, p := range *w.multi {
		if casKey(p) == key {
			k = i
			break
		}
	}
	if k < 0 {
		return
	}
	path := append([]string(nil), (*w.multi)[k]...)
	o, ok := w.casFindPath(path)
	if !ok {
		return
	}
	ls := w.leaves(o, path, nil)
	if len(ls) == 1 && casKey(ls[0]) == key {
		return // loaded no children: a leaf after all
	}
	out := append(append([][]string(nil), (*w.multi)[:k]...), (*w.multi)[k+1:]...)
	set := map[string]bool{}
	for _, p := range out {
		set[casKey(p)] = true
	}
	for _, l := range ls {
		if !set[casKey(l)] {
			out = append(out, l)
		}
	}
	*w.multi = out
	w.changed = true
}

// loadExpanded starts loading the last expanded option when its children
// are missing (paths set with Expand or restored from the value).
func (w *CascaderW) loadExpanded() {
	if w.loadData == nil || len(w.st.path) == 0 {
		return
	}
	opts := w.opts
	for i, v := range w.st.path {
		o, k := casFind(opts, v)
		if k < 0 {
			return
		}
		p := w.st.path[:i+1]
		if w.pendingLazy(o, p) {
			w.load(p)
			return
		}
		opts = w.kids(o, p)
	}
}

// strategyChips computes the multiple-mode tags for ShowChild / ShowAll.
func (w *CascaderW) strategyChips() (labels []string, paths [][]string) {
	set := w.checked()
	el := w.th.Flavor == theme.Element
	label := func(o CascaderOption, p []string) string {
		if el {
			return strings.Join(w.labelsOf(p), " / ")
		}
		return o.Label
	}
	var walk func(opts []CascaderOption, prefix []string)
	walk = func(opts []CascaderOption, prefix []string) {
		for _, o := range opts {
			p := append(append([]string(nil), prefix...), o.Value)
			all, some := w.checkState(o, p, set)
			if !some {
				continue
			}
			ks := w.kids(o, p)
			leaf := len(ks) == 0
			if all && (leaf || w.strategy == ShowAll) {
				labels = append(labels, label(o, p))
				paths = append(paths, p)
			}
			if !leaf {
				walk(ks, p)
			}
		}
	}
	walk(w.opts, nil)
	return
}

// casCheckState is checkState over static children (no lazy loading).
func casCheckState(o CascaderOption, path []string, set map[string]bool) (all, some bool) {
	w := &CascaderW{st: &casState{}}
	return w.checkState(o, path, set)
}
