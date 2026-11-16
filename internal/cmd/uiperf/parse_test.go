package main

import "testing"

func TestParseBenchLine(t *testing.T) {
	got, err := parseBenchLine("BenchmarkComponents/Table/Large100k-10  \t  1234\t 812345 ns/op\t 0.1250 dirty/op\t 2048 B/op\t 17 allocs/op")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Table" || got.Scene != "Large100k" || got.NsOp != 812345 || got.Dirty != .125 || got.BOp != 2048 || got.AllocsOp != 17 {
		t.Fatalf("got %+v", got)
	}
}
func TestParseBudgetLine(t *testing.T) {
	got, err := parseBudgetLine("PERF Button/Interact 2500000 2000000 over")
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != "Button/Interact" || got.NS != 2500000 || got.Budget != 2000000 || got.Status != "over" {
		t.Fatalf("got %+v", got)
	}
}

func TestParseBenchLineMetricOrderAndHyphen(t *testing.T) {
	got, err := parseBenchLine("BenchmarkComponents/Date-Range/Large-100k-10  3  17 allocs/op  2048 B/op  0.125 dirty/op  812345 ns/op")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Date-Range" || got.Scene != "Large-100k" || got.NsOp != 812345 || got.Dirty != .125 || got.BOp != 2048 || got.AllocsOp != 17 {
		t.Fatalf("got %+v", got)
	}
}
func TestMedianEven(t *testing.T) {
	if got := median([]float64{9, 1, 5, 3}); got != 4 {
		t.Fatalf("got %v", got)
	}
}
