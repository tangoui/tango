#!/bin/sh
# build.sh: release builds of the three scene programs and the runner into
# bin/, the way the brief fixes it: GOWORK=off CGO_ENABLED=0 -trimpath
# -ldflags "-s -w". A framework that does not build without cgo is built
# with CGO_ENABLED=1 instead; bin/build-info.txt says how each was built
# and the runner copies that into every result.
set -eu

# One toolchain for every framework: MyGo and NSGo need go1.27.3, and the
# Tango module alone would build with whatever go is installed.
export GOTOOLCHAIN="${GOTOOLCHAIN_UIBENCH:-go1.27.3}"

here=$(cd "$(dirname "$0")" && pwd)
bin="$here/bin"
mkdir -p "$bin"
: >"$bin/build-info.txt"

if [ ! -e "$here/third_party/nsgo/go.mod" ]; then
	echo "build.sh: run ./setup.sh <nsgo source directory> first" >&2
	exit 1
fi

for fw in tango mygo nsgo; do
	cd "$here/$fw"
	if GOWORK=off CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$bin/$fw" . 2>"$bin/$fw.cgo0.log"; then
		mode="CGO_ENABLED=0"
		rm -f "$bin/$fw.cgo0.log"
	else
		echo "build.sh: $fw failed with CGO_ENABLED=0 (bin/$fw.cgo0.log), retrying with CGO_ENABLED=1" >&2
		GOWORK=off CGO_ENABLED=1 go build -trimpath -ldflags "-s -w" -o "$bin/$fw" .
		mode="CGO_ENABLED=1 (CGO_ENABLED=0 failed)"
	fi
	size=$(wc -c <"$bin/$fw" | tr -d ' ')
	echo "$fw $mode $(go version "$bin/$fw" | cut -d' ' -f2) -trimpath -ldflags=-s_-w bytes=$size" >>"$bin/build-info.txt"
done

cd "$here/runner"
GOWORK=off CGO_ENABLED=0 go build -trimpath -o "$bin/runner" .
cat "$bin/build-info.txt"
