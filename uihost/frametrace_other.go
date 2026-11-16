//go:build !darwin

package uihost

// attachTimeline: only the Metal presenter (macOS) has a timeline; a frame
// trace elsewhere reports frame, render, present and gap only.
func attachTimeline(r any, t *frameTrace) {}
