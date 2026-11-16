//go:build !darwin

package macoslite

import (
	"errors"
	"testing"
)

func TestNewUnsupported(t *testing.T) {
	_, err := New(Options{Title: "Lite Counter", Width: 360, Height: 260}, nopView{})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}
