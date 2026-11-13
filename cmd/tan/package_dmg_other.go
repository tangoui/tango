//go:build !darwin

package main

import (
	"errors"
	"time"
)

// Disk images are made on macOS only (packageDarwin stops earlier).

var errNotDarwin = errors.New("only on macOS")

func catalogInfo(string) (uint32, time.Time, error) { return 0, time.Time{}, errNotDarwin }
func setCustomIconFlag(string) error                { return errNotDarwin }
func setHidden(string) error                        { return errNotDarwin }
