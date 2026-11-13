package main

import (
	"github.com/tangoui/tango/ui/input"
	"strings"
)

func browserKey(code string) input.Key {
	code = strings.TrimPrefix(strings.TrimPrefix(code, "Key"), "Digit")
	code = strings.Replace(code, "Arrow", "", 1)
	code = strings.Replace(code, "Control", "Ctrl", 1)
	k, _ := input.KeyNamed(code)
	return k
}
