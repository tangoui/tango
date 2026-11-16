package kv

import (
	"fmt"
	"testing"
)

func benchmarkMemoryStore(b *testing.B, n int) *Store {
	b.Helper()
	s, err := New("")
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < n; i++ {
		if err := s.Set(fmt.Sprint(i), "initial"); err != nil {
			b.Fatal(err)
		}
	}
	return s
}

func BenchmarkMemorySet(b *testing.B) {
	for _, n := range []int{10, 1000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			s := benchmarkMemoryStore(b, n)
			b.ReportAllocs()
			for b.Loop() {
				if err := s.Set("0", "updated"); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkMemoryGet(b *testing.B) {
	s := benchmarkMemoryStore(b, 1000)
	b.ReportAllocs()
	for b.Loop() {
		if s.Get("0") != "initial" {
			b.Fatal("missing value")
		}
	}
}

func TestMemorySetAllocationsDoNotScaleWithStoreSize(t *testing.T) {
	s, err := New("")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1000; i++ {
		if err := s.Set(fmt.Sprint(i), "initial"); err != nil {
			t.Fatal(err)
		}
	}
	allocs := testing.AllocsPerRun(20, func() {
		if err := s.Set("0", "updated"); err != nil {
			panic(err)
		}
	})
	small, _ := New("")
	if err := small.Set("0", "initial"); err != nil {
		t.Fatal(err)
	}
	smallAllocs := testing.AllocsPerRun(20, func() {
		if err := small.Set("0", "updated"); err != nil {
			panic(err)
		}
	})
	if allocs != smallAllocs {
		t.Fatalf("Set allocations scale with store size: small=%.0f, large=%.0f", smallAllocs, allocs)
	}
	if err := s.Set("0", make(chan int)); err == nil || s.Get("0") != "updated" {
		t.Fatal("invalid update altered the store")
	}
}
