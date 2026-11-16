package uihost

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"testing"
	"time"
)

func TestAutoPolicyPromotesSustainedRefresh(t *testing.T) {
	var policy autoRendererState
	now := time.Unix(100, 0)
	for i := 0; i < autoRapidFrames; i++ {
		got := policy.shouldPromote(&paint.List{}, geom.Size{W: 100, H: 100}, now.Add(time.Duration(i)*17*time.Millisecond))
		if got != (i == autoRapidFrames-1) {
			t.Fatalf("rapid frame%d promoted=%v", i, got)
		}
	}
}
func TestAutoPolicyDoesNotPromoteSparseInput(t *testing.T) {
	var policy autoRendererState
	now := time.Unix(100, 0)
	for i := 0; i < 20; i++ {
		if policy.shouldPromote(&paint.List{}, geom.Size{W: 100, H: 100}, now.Add(time.Duration(i)*time.Second)) {
			t.Fatal("sparse counter input promoted")
		}
	}
}
