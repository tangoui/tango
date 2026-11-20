package ui

import (
	"fmt"
	"github.com/tangoui/tango/ui/layout"
	"reflect"
	"testing"
)

func TestListRange(t *testing.T) {
	cases := []struct{ n, page, size, a, b int }{
		{10, 1, 3, 0, 3}, {10, 4, 3, 9, 10}, {10, 9, 3, 9, 10}, {0, 1, 3, 0, 0}, {5, 0, 10, 0, 5},
	}
	for _, c := range cases {
		if a, b := listRange(c.n, c.page, c.size); a != c.a || b != c.b {
			t.Errorf("listRange(%d,%d,%d) = %d,%d", c.n, c.page, c.size, a, b)
		}
	}
}

func TestListPaginationAndLoadMore(t *testing.T) {
	page := 1
	count := 2
	var shown []int
	d := driver(func(c *Context) {
		shown = shown[:0]
		List(c, 7).Pagination(&page, 3).Item(func(i int) {
			shown = append(shown, i)
			Text(c, fmt.Sprint("item ", i))
		})
		l := List(c, count).LoadMore("more", false).Item(func(i int) { Text(c, fmt.Sprint("row ", i)) })
		if l.LoadMoreClicked() {
			count += 2
		}
	})
	d.Frame()
	if !reflect.DeepEqual(shown, []int{0, 1, 2}) {
		t.Fatalf("page 1 = %v", shown)
	}
	d.Click(mustFind(t, d, "3").Center())
	if page != 3 || !reflect.DeepEqual(shown, []int{6}) {
		t.Fatalf("page %d = %v", page, shown)
	}
	d.Click(mustFind(t, d, "more").Center())
	if count != 4 {
		t.Fatalf("count = %d", count)
	}
	if _, ok := d.Find("row 3"); !ok {
		d.Frame()
	}
}

func TestVirtualListUsesKeyedHeightsAfterReorder(t *testing.T) {
	const last, middle = 59, 30
	order := make([]int, last+1)
	for i := range order {
		order[i] = i
	}
	request := -1
	var st *listVirtState
	var first, end int
	d := newPerfDriver(func(c *Context) {
		w := List(c, len(order)).Height(100).Virtual().EstimatedItemHeight(48).
			RowKey(func(i int) any { return order[i] })
		if request >= 0 {
			w.ScrollToKey(request, AlignTop, 0)
			request = -1
		}
		w.Item(func(i int) {
			h := float32(24)
			if order[i] == 0 {
				h = 300
			}
			Column(c, func() {}, layout.H(h))
		})
		first, end = w.Range()
		st = State[listVirtState](c, hashID(w.view, "list-virt"), nil)
	}, false)
	for range 3 {
		d.Frame()
	}
	request = last
	for range 6 {
		d.Frame()
	}
	request = middle
	for range 6 {
		d.Frame()
	}
	tall, short := st.byKey[0], st.byKey[last]
	if tall <= short || short == 0 || first == 0 || end > last {
		t.Fatalf("need two measured offscreen rows: heights=%v/%v, range=[%d,%d)", tall, short, first, end)
	}
	oldPrefix := st.offs[middle]
	want := oldPrefix - float64(tall-short)
	order[0], order[last] = order[last], order[0]
	request = middle
	for range 6 {
		d.Frame()
	}
	scrolls := v1Scrolls(d)
	if len(scrolls) != 1 {
		t.Fatalf("want one scroll state, got %d", len(scrolls))
	}
	if got := scrolls[0].off.Y; got != want {
		t.Errorf("after reorder: ScrollToKey offset=%g, prefix=%g, want=%g (old prefix=%g, measured keyed heights=%g/%g)", got, st.offs[middle], want, oldPrefix, tall, short)
	}

	// Reorder again with the first row in view. ScrollToKey forces another
	// build before layout, while the old first row's rectangle still exists.
	request = last
	for range 6 {
		d.Frame()
	}
	order[0], order[last] = order[last], order[0]
	request = 0
	d.Frame()
	if got := st.hs[0]; got != tall {
		t.Errorf("after reordering a visible row: first height=%g, want %g", got, tall)
	}
}
