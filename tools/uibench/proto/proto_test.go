package proto

import (
	"bufio"
	"bytes"
	"testing"
	"time"
)

func reset(b *bytes.Buffer) {
	out = bufio.NewWriter(b)
	ready, frames, work, intervals, renderer = false, 0, 0, nil, ""
}

// The renderer line carries the time (Unix ns) and comes at ready, with
// every per-second report, and at once when the path changes.
func TestLines(t *testing.T) {
	var b bytes.Buffer
	reset(&b)
	t0 := time.Unix(100, 0)
	now = func() time.Time { return t0.Add(1500 * time.Millisecond) }
	defer func() { now = time.Now }()
	Scene("grid", 800, 600)
	Renderer("native") // before ready: kept for the ready line
	frameAt(t0, 600)   // ready: not counted
	for i := 1; i <= 4; i++ {
		Renderer("native") // unchanged: no line
		frameAt(t0.Add(time.Duration(i)*250*time.Millisecond), 600)
	}
	Renderer("gpu") // changed between frames: a line at once
	Renderer("gpu")
	want := "bench scene grid 800x600\n" +
		"bench ready\nbench renderer native 100000000000\n" +
		"bench frames 4.0\nbench work 2400.0\nbench intervals_us 250000,250000,250000,250000\nbench renderer native 101000000000\n" +
		"bench renderer gpu 101500000000\n"
	if b.String() != want {
		t.Fatalf("got\n%s\nwant\n%s", b.String(), want)
	}
}

// A program that has not named its renderer by ready prints no renderer
// line until it does.
func TestRendererAfterReady(t *testing.T) {
	var b bytes.Buffer
	reset(&b)
	t0 := time.Unix(7, 0)
	now = func() time.Time { return t0 }
	defer func() { now = time.Now }()
	frameAt(t0, 1)
	Renderer("metal")
	want := "bench ready\nbench renderer metal 7000000000\n"
	if b.String() != want {
		t.Fatalf("got\n%s\nwant\n%s", b.String(), want)
	}
}
