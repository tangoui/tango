//go:build !darwin || !cgo

package main

// footprint is not measured off Apple platforms (or without cgo).
func footprint() uint64 { return 0 }
