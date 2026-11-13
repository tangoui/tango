package platform

import "testing"

func TestLocaleListeners(t *testing.T) {
	var l LocaleListeners
	n := 0
	c1, first := l.Add(func() { n++ })
	if !first {
		t.Error("first Add not reported as first")
	}
	c2, first := l.Add(func() { n += 10 })
	if first {
		t.Error("second Add reported as first")
	}
	l.Notify()
	c1()
	c1() // twice is fine
	l.Notify()
	c2()
	l.Notify()
	if n != 21 {
		t.Errorf("n = %d, want 21", n)
	}
}
