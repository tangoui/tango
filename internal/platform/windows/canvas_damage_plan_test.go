package windows

import (
	"image"
	"slices"
	"testing"
)

func TestCanvasDamageBlits(t *testing.T) {
	dirty := []image.Rectangle{image.Rect(10, 10, 20, 20), image.Rect(90, 90, 120, 120), image.Rect(5, 5, 5, 9)}
	got := canvasDamageBlits(dirty, image.Rect(0, 0, 30, 5), 100, 100)
	want := []image.Rectangle{image.Rect(10, 10, 20, 20), image.Rect(90, 90, 100, 100), image.Rect(0, 0, 30, 5)}
	if !slices.Equal(got, want) {
		t.Fatalf("blits %v, want %v", got, want)
	}
	if got := canvasDamageBlits(dirty[:1], image.Rectangle{}, 100, 100); len(got) != 1 {
		t.Fatalf("no pending update: %v", got)
	}
	if len(dirty) != 3 || dirty[2] != image.Rect(5, 5, 5, 9) {
		t.Fatal("input modified")
	}
}
