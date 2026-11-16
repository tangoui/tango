package proto

import (
	"bufio"
	"bytes"
	"testing"
	"time"
)

func TestLines(t *testing.T) {
	var b bytes.Buffer
	out = bufio.NewWriter(&b)
	Renderer("native")
	t0 := time.Unix(100, 0)
	frameAt(t0, 600) // ready: not counted
	for i := 1; i <= 4; i++ {
		frameAt(t0.Add(time.Duration(i)*250*time.Millisecond), 600)
	}
	want := "bench renderer native\nbench ready\nbench frames 4.0\nbench work 2400.0\nbench intervals_us 250000,250000,250000,250000\n"
	if b.String() != want {
		t.Fatalf("got\n%s\nwant\n%s", b.String(), want)
	}
}
