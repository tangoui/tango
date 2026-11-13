//go:build !darwin || ios

package main

import (
	"fmt"

	"github.com/tangoui/tango"
)

// runSmoke has no self-check off macOS yet: the window opened, which is
// all it verifies.
func runSmoke(*tango.App, *tango.Surface) bool {
	fmt.Println("SKIP a11y self-check: macOS only")
	return true
}
