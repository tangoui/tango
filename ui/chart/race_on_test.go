//go:build race

package chart

// raceEnabled: the race detector slows rendering several times over, so
// timing gates only run without it.
const raceEnabled = true
