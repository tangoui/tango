#!/bin/sh
# build.sh: release builds of the three scene programs and the runner into
# bin/, with one pinned toolchain and environment (spec §5):
#
#	GOTOOLCHAIN=go1.27.3 CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 GOWORK=off
#	scene programs: go build -trimpath -ldflags "-s -w"; runner: -trimpath
#
# The building is the runner's -build mode (runner/manifest.go), so that it
# is tested Go: it runs each go build exactly as recorded, fails on the
# first error (no CGO_ENABLED=1 fallback, no other toolchain), checks every
# binary's go version and settings, and writes bin/manifest.json (source
# versions, commands, environment, SHA-256, `go version -m`). The runner
# refuses to measure unless that manifest verifies.
set -eu

here=$(cd "$(dirname "$0")" && pwd)

if [ ! -e "$here/third_party/nsgo/go.mod" ]; then
	echo "build.sh: run ./setup.sh <nsgo source directory> first" >&2
	exit 1
fi

# Exactly this toolchain (no "+auto"): a module that needs a newer go fails
# instead of switching toolchains for one framework. Keep it equal to
# pinnedToolchain in runner/manifest.go, which the builder checks.
export GOTOOLCHAIN=go1.27.3 CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 GOWORK=off GOFLAGS=

cd "$here/runner"
go run . -build -bin "$here/bin"
