//go:build !darwin

package metal

import (
	"errors"
	"testing"
)

func TestNewPresenterUnsupported(t *testing.T) {
	r, present, err := NewPresenter(1)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
	if r != nil || present != nil {
		t.Fatal("NewPresenter returned a renderer or present func with an error")
	}
}
