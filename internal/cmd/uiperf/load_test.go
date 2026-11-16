package main

import (
	"errors"
	"testing"
	"time"
)

func TestLoadGate(t *testing.T) {
	vals := []float64{7.2, 6.5, 5.1}
	i := 0
	waits := 0
	err := loadGate(func() (float64, error) { v := vals[i]; i++; return v, nil }, func(time.Duration) { waits++ }, 6, time.Minute)
	if err != nil || waits != 2 {
		t.Fatalf("err=%v waits=%d", err, waits)
	}
	calls := 0
	err = loadGate(func() (float64, error) { calls++; return 8, nil }, func(time.Duration) {}, 6, 2*time.Second)
	if err == nil {
		t.Fatal("expected timeout")
	}
	_ = errors.New
}
func TestParseLoadavg(t *testing.T) {
	got, err := parseLoadavg("{ 11.53 20.60 27.67 }")
	if err != nil || got != 11.53 {
		t.Fatalf("%v %v", got, err)
	}
}
