//go:build !darwin

package services

// backendLaneStats leaves the scheme handler counters zero: only the
// macOS and iOS backends count them.
func backendLaneStats(*Stats) {}
