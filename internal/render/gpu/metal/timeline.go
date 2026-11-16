package metal

import "time"

// Timing is the timeline of one command buffer the Device committed (a
// frame, a present or a readback), or of a present that found no drawable.
// It is diagnostics only: SetTimeline delivers it when the command buffer
// is retired (Execute, present, Wait, ReadPixels or Close waited for it).
//
// Nothing here says when a frame reached the screen: CAMetalLayer gives no
// per-frame display feedback, so there is no displayed time to report.
type Timing struct {
	Kind  string // "frame", "present" or "other" (readback)
	Frame uint64 // Execute frame number (a present: the frame it shows)

	// Commit is when the command buffer was committed (zero for a present
	// that found no drawable and committed nothing).
	Commit time.Time
	// GPUStart and GPUEnd are Metal's GPUStartTime/GPUEndTime (host time
	// base, as durations); GPUKnown is false when Metal reported none, and
	// then both are zero.
	GPUStart, GPUEnd time.Duration
	GPUKnown         bool

	// Drawable reports whether a present got a drawable; DrawableWait is
	// how long nextDrawable blocked (presents only, never part of GPU time).
	Drawable     bool
	DrawableWait time.Duration

	Err error // the command buffer's error, nil when it completed
}
