package asset

import "testing"

func TestCanonicalAssetPathsAllocateNothing(t *testing.T) {
	for _, tc := range []struct{ path, want string }{
		{"/", ""},
		{"/app.js", "app.js"},
		{"/assets/icons/normal/file.svg", "assets/icons/normal/file.svg"},
		{"/assets/图标.svg", "assets/图标.svg"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			allocs := testing.AllocsPerRun(100, func() {
				got, ok := cleanPath(tc.path)
				if !ok || got != tc.want {
					panic("wrong path")
				}
			})
			if allocs != 0 {
				t.Fatalf("canonical path allocated %.0f times", allocs)
			}
		})
	}
}

func BenchmarkAssetPath(b *testing.B) {
	for _, path := range []string{"/app.js", "/assets/icons/normal/file.svg", "assets/icons/normal/file.svg", "/a//b/./c"} {
		b.Run(path, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, ok := cleanPath(path); !ok {
					b.Fatal("path rejected")
				}
			}
		})
	}
}
