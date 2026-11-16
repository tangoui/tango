package front

import "testing"

func BenchmarkLoadShader(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		m, err := Load("testdata/ok")
		if err != nil || m.Vertex == nil || m.Fragment == nil {
			b.Fatal("invalid shader", err)
		}
	}
}
