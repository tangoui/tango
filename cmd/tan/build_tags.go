package main

import (
	"os"
	"slices"
	"strings"
)

// devTag links Tango's development features (the hot-reload dev host,
// the automation channel, asset.FromEnv's dev-server proxy, i18n and
// panic diagnostics) into a build. Only tan dev adds it; every other build,
// tan build -debug and plain go build included, is a production build.
const devTag = "tango_dev"

// releaseTag is the former opt-in release tag. It has no effect any more
// (production is the default); tan build still passes it so build scripts
// and tests that look for it keep working.
const releaseTag = "tango_release"

// buildTags returns the -tags value for a go build of the app: the tags
// the user set in GOFLAGS (-tags=a,b), which an explicit -tags flag would
// otherwise override, plus tango_dev for tan dev (dev) or, for tan build,
// without tango_dev and with the compatibility tag tango_release.
func buildTags(dev bool) string {
	tags := slices.DeleteFunc(goflagsTags(os.Getenv("GOFLAGS")), func(t string) bool {
		return t == devTag || t == releaseTag
	})
	if dev {
		tags = append(tags, devTag)
	} else {
		tags = append(tags, releaseTag)
	}
	return strings.Join(tags, ",")
}

// goflagsTags extracts the build tags from a GOFLAGS value.
func goflagsTags(goflags string) []string {
	var tags []string
	for _, f := range strings.Fields(goflags) {
		v, ok := strings.CutPrefix(f, "-tags=")
		if !ok {
			v, ok = strings.CutPrefix(f, "--tags=")
		}
		if !ok {
			continue
		}
		tags = nil // the last -tags wins, as in go
		for _, t := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' }) {
			if t != "" && !slices.Contains(tags, t) {
				tags = append(tags, t)
			}
		}
	}
	return tags
}
