package main

import (
	"os"
	"testing"
)

// TestMain lets tests run the test binary as tan: with TAN_TEST_MAIN=1 it
// runs main with os.Args[1:] as the tan arguments (see selfEnv).
func TestMain(m *testing.M) {
	if os.Getenv("TAN_TEST_MAIN") == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}
