//go:build darwin && !ios && tango_dev

package darwin

import "testing"

func TestLaneProbeCounters(t *testing.T) {
	var c laneCounters
	if c.track("/index.html") != nil || c.track("/__tango/lane/other") != nil {
		t.Fatal("non-lane paths are tracked")
	}

	// A refused down request: counted as a request, not a connection.
	d := c.track("/__tango/lane/down")
	d.respond(403)
	d.data()
	d.done()
	// An accepted one: a connection, active until done, its data blocks.
	a := c.track("/__tango/lane/down")
	a.respond(200)
	a.data()
	a.data()
	if s := c.stats(); s.DownActive != 1 || s.DownConns != 1 {
		t.Fatalf("while open: %+v", s)
	}
	a.done()
	a.done() // repeated: stop after finish must not count twice
	// Up: a refused and an accepted request.
	u := c.track("/__tango/lane/up")
	u.respond(403)
	u.done()
	u = c.track("/__tango/lane/up")
	u.respond(204)
	if s := c.stats(); s.UpActive != 1 {
		t.Fatalf("up request open: %+v", s)
	}
	u.done()
	u.done()                // repeated: must not count twice
	var nilTrack *laneTrack // untracked tasks call these too
	nilTrack.respond(200)
	nilTrack.data()
	nilTrack.done()

	want := LaneStats{DownRequests: 2, DownConns: 1, DownActive: 0, DownWrites: 2, UpRequests: 2, UpAccepted: 1}
	if s := c.stats(); s != want {
		t.Fatalf("stats %+v, want %+v", s, want)
	}
}
