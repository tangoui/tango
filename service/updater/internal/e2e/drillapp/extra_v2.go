//go:build drillv2

package main

import (
	"sort"
	"strings"
)

// extra is new code in the second release, so that the drill's patch
// covers a real code change, not only a different version string.
func extra(logf func(string, ...any)) {
	words := strings.Fields("the second release adds a little code")
	sort.Strings(words)
	if len(words) > 100 {
		logf("%s", strings.Join(words, " "))
	}
}
