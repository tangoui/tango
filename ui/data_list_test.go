package ui

import (
	"fmt"
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
