//go:build !darwin && !android

package services

// backendLaneStats leaves the backend lane counters zero: only the
// macOS, iOS and Android backends count them.
func backendLaneStats(*Stats) {}
