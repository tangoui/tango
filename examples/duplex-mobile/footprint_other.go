//go:build (!darwin && !android) || !cgo

package main

// footprint is not measured off Apple platforms and Android (or without cgo).
func footprint() uint64 { return 0 }
