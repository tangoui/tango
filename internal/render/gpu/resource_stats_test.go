package gpu

import "testing"

func TestResourceStatsFrameBoundary(t *testing.T) {
	var s resourceStats
	s.BeginFrame()
	s.AddAtlasPage(256)
	s.AddAtlasPage(256)
	s.AddImage(512)
	s.AddImage(512)
	s.ImageCount++
	s.RenderTargetBytes = 2048
	for range 7 {
		s.AddUpload()
	}
	s.AddEviction()
	for range 2 {
		s.AddReupload()
	}
	got := s.Snapshot()
	if got.AtlasPages != 2 || got.AtlasBytes != 512 || got.ImageCount != 3 || got.ImageBytes != 1024 || got.RenderTargetBytes != 2048 || got.Uploads != 7 || got.Evictions != 1 || got.Reuploads != 2 {
		t.Fatalf("snapshot = %+v", got)
	}
	// A frame snapshot is stable after the boundary; the next frame starts
	// with counters for that frame, not a hidden reset of residency totals.
	s.EndFrame()
	s.BeginFrame()
	if got := s.Snapshot(); got.Uploads != 0 || got.Evictions != 0 || got.Reuploads != 0 {
		t.Fatalf("per-frame counters leaked: %+v", got)
	}
	if got := s.Snapshot(); got.AtlasBytes != 512 || got.ImageBytes != 1024 {
		t.Fatalf("residency reset at frame boundary: %+v", got)
	}
}

func TestResourceStatsSaturatingRelease(t *testing.T) {
	var s resourceStats
	s.AddImage(1024)
	s.AddImage(-2048)
	if got := s.Snapshot().ImageBytes; got != 0 {
		t.Fatalf("image bytes underflowed: %d", got)
	}
	s.AddAtlasPage(256 * 256)
	s.AddAtlasPage(-2 * 256 * 256)
	if got := s.Snapshot().AtlasBytes; got != 0 {
		t.Fatalf("atlas bytes underflowed: %d", got)
	}
}

func TestResourceStatsSnapshotCopies(t *testing.T) {
	var s resourceStats
	s.AddUpload()
	got := s.Snapshot()
	s.AddUpload()
	if got.Uploads != 1 || s.Snapshot().Uploads != 2 {
		t.Fatalf("snapshot aliases mutable counters: got=%+v now=%+v", got, s.Snapshot())
	}
}
