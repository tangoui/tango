//go:build !darwin || ios

package main

import "github.com/tangoui/tango"

func enableMockCapture(*tango.Surface) {}
