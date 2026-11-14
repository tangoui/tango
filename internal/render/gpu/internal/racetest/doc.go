// Package racetest reports whether the test binary was built with the race
// detector, so the GPU renderer's heavy pixel tests (the CPU executor runs
// the shader per pixel, 10–20× slower under -race) can run a reduced but
// representative set of cases there and keep the full set otherwise.
package racetest
