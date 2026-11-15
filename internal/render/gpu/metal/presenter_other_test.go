//go:build !darwin

package metal

import (
	"errors"
	"testing"
)

func TestDeviceWaitUnsupported(t *testing.T) {
	var d Device
	w, ok := any(&d).(interface{ Wait() error })
	if !ok {
		t.Fatal("Device stub must expose Wait() error")
	}
	if err := w.Wait(); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Wait = %v, want ErrUnsupported", err)
	}
}

func TestNewPresenterUnsupported(t *testing.T) {
	r, present, err := NewPresenter(1)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
	if r != nil || present != nil {
		t.Fatal("NewPresenter returned a renderer or present func with an error")
	}
}
