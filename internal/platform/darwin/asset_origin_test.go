//go:build darwin && !ios

package darwin

import "testing"

func TestWithOrigin(t *testing.T) {
	h := withOrigin([][2]string{{"Referer", "x"}}, "HTTP://Example.COM:8080/a?b")
	if len(h) != 2 || h[1] != [2]string{"Origin", "http://example.com:8080"} {
		t.Fatalf("added %v", h)
	}
	if h := withOrigin([][2]string{{"origin", "tango://app"}}, "http://evil.example/"); len(h) != 1 {
		t.Fatalf("kept page's Origin? %v", h)
	}
	for _, doc := range []string{"", "about:blank", "data:text/html,x"} {
		if h := withOrigin(nil, doc); len(h) != 0 {
			t.Errorf("%q: %v", doc, h)
		}
	}
}
