package mobilecanvas

import (
	"math"
	"testing"

	"github.com/tangoui/tango/ui/geom"
)

func TestLogical(t *testing.T) {
	tests := []struct{ px, d, want float32 }{
		{84, 2.625, 32},
		{63, 2.625, 24},
		{100, 2, 50},
		{100, 0, 100},
		{100, -1, 100},
		{0, 3, 0},
	}
	for _, tt := range tests {
		if got := Logical(tt.px, tt.d); got != tt.want {
			t.Errorf("Logical(%v, %v) = %v, want %v", tt.px, tt.d, got, tt.want)
		}
	}
}

func TestInsetsPx(t *testing.T) {
	tests := []struct {
		top, right, bottom, left int
		d                        float32
		want                     geom.Insets
	}{
		// A Pixel-like status bar and gesture navigation at 2.625.
		{63, 0, 42, 0, 2.625, geom.Insets{Top: 24, Bottom: 16}},
		{-5, 30, 0, 30, 3, geom.Insets{Right: 10, Left: 10}},
		{10, 10, 10, 10, 0, geom.All(10)},
	}
	for _, tt := range tests {
		if got := InsetsPx(tt.top, tt.right, tt.bottom, tt.left, tt.d); got != tt.want {
			t.Errorf("InsetsPx(%d,%d,%d,%d, %v) = %v, want %v", tt.top, tt.right, tt.bottom, tt.left, tt.d, got, tt.want)
		}
	}
}

func TestClampMax(t *testing.T) {
	nan := float32(math.NaN())
	inf := float32(math.Inf(1))
	if got := Clamp(geom.Insets{Top: -1, Right: nan, Bottom: inf, Left: 4}); got != (geom.Insets{Left: 4}) {
		t.Errorf("Clamp = %v", got)
	}
	tests := []struct{ a, b, want geom.Insets }{
		{geom.Insets{Top: 47, Bottom: 34}, geom.Insets{Bottom: 336}, geom.Insets{Top: 47, Bottom: 336}},
		{geom.Insets{Top: 47, Bottom: 34}, geom.Insets{}, geom.Insets{Top: 47, Bottom: 34}},
		{geom.Insets{Left: 59, Right: 59, Bottom: 21}, geom.Insets{Bottom: 10}, geom.Insets{Left: 59, Right: 59, Bottom: 21}},
	}
	for _, tt := range tests {
		if got := Max(tt.a, tt.b); got != tt.want {
			t.Errorf("Max(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestKeyboardBottom(t *testing.T) {
	tests := []struct{ view, kb, want float32 }{
		{852, 516, 336}, // keyboard up
		{852, 852, 0},   // just below
		{852, 1200, 0},  // hidden (end frame off screen)
		{600, 516, 84},  // a form sheet: the keyboard covers part of it
		{400, 516, 0},   // view above the keyboard
		{852, float32(math.NaN()), 0},
	}
	for _, tt := range tests {
		if got := KeyboardBottom(tt.view, tt.kb); got != tt.want {
			t.Errorf("KeyboardBottom(%v, %v) = %v, want %v", tt.view, tt.kb, got, tt.want)
		}
	}
	if got := KeyboardInsets(852, 516); got != (geom.Insets{Bottom: 336}) {
		t.Errorf("KeyboardInsets = %v", got)
	}
}
