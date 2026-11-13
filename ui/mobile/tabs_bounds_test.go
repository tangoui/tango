package mobile

import (
	"testing"

	"github.com/tangoui/tango/ui"
)

func TestTabsClampsRestoredActiveIndex(t *testing.T) {
	active := 2
	called := -1
	d := phone(func(c *ui.Context) {
		Tabs(c, []string{"A"}, &active).Body(func(i int) { called = i })
	}, false)
	d.Settle()
	if active != 0 {
		t.Fatalf("active = %d, want 0", active)
	}
	if called != 0 {
		t.Fatalf("pane index = %d, want 0", called)
	}
}

func TestTabsEmptyTitlesDoesNotPanic(t *testing.T) {
	active := 7
	d := phone(func(c *ui.Context) {
		Tabs(c, nil, &active).Body(func(int) { t.Fatal("empty tabs built a pane") })
	}, false)
	d.Settle()
	if active != 0 {
		t.Fatalf("active = %d, want 0", active)
	}
}
