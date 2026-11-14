package vt

import (
	"slices"
	"testing"
)

func FuzzParser(f *testing.F) {
	for _, c := range parserCases {
		f.Add([]byte(c.in), uint16(1))
		f.Add([]byte(c.in), uint16(3))
	}
	f.Fuzz(func(t *testing.T, data []byte, split uint16) {
		whole := &recorder{}
		NewParser(whole).Write(data)

		chunked := &recorder{}
		p := NewParser(chunked)
		size := int(split%64) + 1
		for i := 0; i < len(data); i += size {
			p.Write(data[i:min(i+size, len(data))])
		}
		if !slices.Equal(whole.out, chunked.out) {
			t.Fatalf("chunks of %d differ from one Write:\n got %q\nwant %q", size, chunked.out, whole.out)
		}
	})
}
