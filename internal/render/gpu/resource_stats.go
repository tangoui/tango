package gpu

// ResourceStats is an internal snapshot of GPU-owned residency and per-frame
// churn. It deliberately excludes shared WindowServer/IOSurface accounting.
type ResourceStats struct {
	AtlasPages        int
	AtlasBytes        int64
	ImageCount        int
	ImageBytes        int64
	RenderTargetBytes int64
	Uploads           uint64
	Evictions         uint64
	Reuploads         uint64
}

type resourceStats struct {
	ResourceStats
	frameUploads, frameEvictions, frameReuploads uint64
}

func (s *resourceStats) BeginFrame() {
	s.frameUploads = 0
	s.frameEvictions = 0
	s.frameReuploads = 0
}
func (s *resourceStats) EndFrame() {}
func (s *resourceStats) Snapshot() ResourceStats {
	v := s.ResourceStats
	v.Uploads = s.frameUploads
	v.Evictions = s.frameEvictions
	v.Reuploads = s.frameReuploads
	return v
}
func (s *resourceStats) AddAtlasPage(bytes int64) {
	s.AtlasBytes = saturatingAdd(s.AtlasBytes, bytes)
	if bytes > 0 {
		s.AtlasPages++
	} else if bytes < 0 && s.AtlasPages > 0 {
		s.AtlasPages--
	}
}
func (s *resourceStats) AddImage(bytes int64) {
	s.ImageBytes = saturatingAdd(s.ImageBytes, bytes)
	if bytes > 0 {
		s.ImageCount++
	} else if bytes < 0 && s.ImageCount > 0 {
		s.ImageCount--
	}
}
func (s *resourceStats) AddUpload()   { s.frameUploads++ }
func (s *resourceStats) AddEviction() { s.frameEvictions++ }
func (s *resourceStats) AddReupload() { s.frameReuploads++ }

func saturatingAdd(v, delta int64) int64 {
	if delta > 0 && v > int64(^uint64(0)>>1)-delta {
		return int64(^uint64(0) >> 1)
	}
	if delta < 0 && v < -delta {
		return 0
	}
	return v + delta
}
