//go:build !race

package highlight

const raceFactor = 1

// raceEnabled reports a -race build.
const raceEnabled = false
