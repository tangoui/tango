package services

import (
	"bytes"
	"testing"
)

func TestFillSource(t *testing.T) {
	// Without mod: byte(i) in every frame (what bench.js expects).
	b := make([]byte, 300)
	fillSource(b, 600, 0, 0)
	for i, v := range b {
		if v != byte(i) {
			t.Fatalf("mod 0: byte %d = %d", i, v)
		}
	}
	// With mod: counted across frames.
	const size, frames, mod, add = 1000, 5, 253, 7
	var whole []byte
	for k := range frames {
		f := make([]byte, size)
		fillSource(f, k*size, mod, add)
		whole = append(whole, f...)
	}
	want := make([]byte, size*frames)
	for j := range want {
		want[j] = byte((j + add) % mod)
	}
	if !bytes.Equal(whole, want) {
		t.Fatal("mod 253 add 7: stream differs")
	}
	f := make([]byte, 4)
	fillSource(f, 249, 251, 0)
	if !bytes.Equal(f, []byte{249, 250, 0, 1}) {
		t.Fatal(f)
	}
}
