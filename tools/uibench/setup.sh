#!/bin/sh
# setup.sh <nsgo source directory>
#
# Links the NSGo source as third_party/nsgo (the nsgo program's go.mod
# replaces github.com/nsgo-dev/nsgo with it) and records which source it
# is in third_party/nsgo.version: the first version heading of its
# CHANGELOG.md and a hash of its Go files, since the copy is not a git
# checkout. MyGo comes from the module cache at the version in mygo/go.mod.
set -eu

if [ $# -ne 1 ] || [ ! -f "$1/go.mod" ]; then
	echo "usage: $0 <nsgo source directory>" >&2
	exit 2
fi
here=$(cd "$(dirname "$0")" && pwd)
src=$(cd "$1" && pwd)

mkdir -p "$here/third_party"
ln -sfn "$src" "$here/third_party/nsgo"

version=$(grep -m1 -E '^## \[' "$src/CHANGELOG.md" | sed -E 's/^## \[([^]]*)\].*/\1/')
hash=$(cd "$src" && find . -type f -name '*.go' | LC_ALL=C sort | xargs shasum | shasum | cut -d' ' -f1)
{
	echo "source $src"
	echo "changelog $version"
	echo "go-tree-sha1 $hash"
} >"$here/third_party/nsgo.version"
cat "$here/third_party/nsgo.version"
