package fake

import (
	"errors"
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestPrint(t *testing.T) {
	b := New()
	if _, err := b.NewSurface(1, platform.SurfaceOptions{URL: "/"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.NewCanvasSurface(2, platform.SurfaceOptions{}, platform.SurfaceExtras{}, &nopSink{}); err != nil {
		t.Fatal(err)
	}
	var got []error
	collect := func(err error) { got = append(got, err) }

	b.PrintPage(1, platform.PrintOptions{Title: "T", Copies: 2}, collect)
	b.PrintPage(2, platform.PrintOptions{}, collect) // canvas: no page
	b.PrintPage(9, platform.PrintOptions{}, collect) // unknown
	b.SetPrintErr(platform.ErrCanceled)
	b.PrintDocument(0, []byte("%PDF-1.4"), platform.PrintOptions{}, collect)
	b.PrintDocument(0, nil, platform.PrintOptions{}, collect)
	for b.Step(false) {
		if len(got) == 5 {
			break
		}
	}
	// Synchronous answers come first (canvas, unknown, empty document),
	// the queued ones after them (page, document).
	if len(got) != 5 || !errors.Is(got[0], platform.ErrUnsupported) || !errors.Is(got[1], platform.ErrUnsupported) ||
		got[2] == nil || got[3] != nil || !errors.Is(got[4], platform.ErrCanceled) {
		t.Fatalf("answers: %v", got)
	}
	jobs := b.PrintJobs()
	if len(jobs) != 2 || jobs[0].Surface != 1 || jobs[0].Opts.Copies != 2 || string(jobs[1].PDF) != "%PDF-1.4" {
		t.Fatalf("%+v", jobs)
	}
}
