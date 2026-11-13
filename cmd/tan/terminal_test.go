package main

import (
	"os"
	"testing"
)

func TestIsTerminalDevNull(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if isTerminal(f.Fd()) {
		t.Errorf("%s reported as a terminal", os.DevNull)
	}
}

func TestIsTerminalFileAndPipe(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "x")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if isTerminal(f.Fd()) {
		t.Error("regular file reported as a terminal")
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if isTerminal(r.Fd()) {
		t.Error("pipe reported as a terminal")
	}
}
