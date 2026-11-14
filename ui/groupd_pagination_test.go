package ui

import (
	"reflect"
	"testing"

	"github.com/tangoui/tango/ui/layout"
)

func TestPageListN(t *testing.T) {
	for _, tc := range []struct {
		cur, pages, count int
		want              []int
	}{
		{1, 5, 7, []int{1, 2, 3, 4, 5}},
		{1, 20, 7, []int{1, 2, 3, 4, 5, 6, -2, 20}},
		{10, 20, 7, []int{1, -1, 8, 9, 10, 11, 12, -2, 20}},
		{20, 20, 7, []int{1, -1, 15, 16, 17, 18, 19, 20}},
		{4, 20, 7, []int{1, 2, 3, 4, 5, 6, -2, 20}},
		{5, 20, 7, []int{1, -1, 3, 4, 5, 6, 7, -2, 20}},
		{10, 20, 5, []int{1, -1, 9, 10, 11, -2, 20}},
		{1, 30, 11, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, -2, 30}},
	} {
		if got := pageListN(tc.cur, tc.pages, tc.count); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("pageListN(%d, %d, %d) = %v, want %v", tc.cur, tc.pages, tc.count, got, tc.want)
		}
	}
	// Numbered buttons never exceed count.
	for count := 5; count <= 21; count += 2 {
		for cur := 1; cur <= 50; cur++ {
			n := 0
			for _, pg := range pageListN(cur, 50, count) {
				if pg > 0 {
					n++
				}
			}
			if n != count {
				t.Fatalf("count %d cur %d: %d numbered buttons", count, cur, n)
			}
		}
	}
}

func TestPaginationPagerCount(t *testing.T) {
	page := 10
	var pw *PaginationW
	d := driver(func(c *Context) {
		Column(c, func() { pw = Pagination(c, &page, 200, 10).PagerCount(5).Show() }, layout.Pad(10))
	})
	d.Settle()
	if _, ok := d.Find("8"); ok {
		t.Fatal("page 8 shown with pager count 5 at page 10")
	}
	for _, s := range []string{"1", "9", "10", "11", "20"} {
		if _, ok := d.Find(s); !ok {
			t.Fatalf("page %s missing", s)
		}
	}
	if pw.pagerCount != 5 {
		t.Fatal(pw.pagerCount)
	}
	// The ••• jump moves by count-2 (at least 5).
	d.ClickLabel("•••")
	if page != 5 && page != 15 {
		t.Fatalf("jump to %d", page)
	}
	// Out-of-range values are clamped to odd numbers in 5..21.
	for in, want := range map[int]int{2: 5, 8: 9, 30: 21, 7: 7} {
		if got := (&PaginationW{}).PagerCount(in).pagerCount; got != want {
			t.Errorf("PagerCount(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestPaginationLayout(t *testing.T) {
	page, size := 2, 10
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			Pagination(c, &page, 95, size).SizeChanger(&size).
				Layout(PaginationPrev, PaginationPager, PaginationNext, PaginationJumper, PaginationSpacer, PaginationTotal).Show()
		}, layout.Pad(10))
	}, DriverOptions{Width: 1000, Height: 200})
	d.Settle()
	loc := ZhCN
	prev, _ := d.Find(loc.PrevPage)
	next, _ := d.Find(loc.NextPage)
	total, ok := d.FindText(loc.Total(95))
	if !ok {
		t.Fatal("total missing")
	}
	jump, ok := d.FindText(loc.JumpTo)
	if !ok {
		t.Fatal("jumper missing")
	}
	if !(prev.X < next.X && next.X < jump.X && jump.X < total.X) {
		t.Fatalf("order: prev %v next %v jumper %v total %v", prev.X, next.X, jump.X, total.X)
	}
	// The spacer pushes total to the end of the 980px row.
	if total.Right() < 980 {
		t.Fatalf("total not at the end: %v", total)
	}
	// sizes is not listed: no size select.
	if _, ok := d.Find(loc.PageSizeLabel(10)); ok {
		t.Fatal("size select shown though not in the layout")
	}
	// A layout without the pager: only total and arrows.
	d2 := driver(func(c *Context) {
		Pagination(c, &page, 95, 10).Layout(PaginationTotal, PaginationPrev, PaginationNext).Show()
	})
	d2.Settle()
	if _, ok := d2.Find("3"); ok {
		t.Fatal("pager shown without PaginationPager")
	}
	if _, ok := d2.FindText(loc.Total(95)); !ok {
		t.Fatal("total missing")
	}
}
