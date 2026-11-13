//go:build !darwin

package main

import "github.com/tangoui/tango"

func userQuit(*tango.App) bool { return false }

func appKitEvents(*tango.App, string) bool { return false }
