#!/usr/bin/env bash
# Builds the two minimal Tango apps documented in site/guide/size.md for
# darwin/arm64, linux/amd64 and windows/amd64 and prints their sizes.
#
#   bash scripts/size-report.sh          run from anywhere inside the repo
#
# Apps (sources are written to a temporary module that points at this
# checkout with a replace directive):
#   web  one window loading a URL (system WebView)
#   ui   uihost.Open + one ui.Button (Go self-drawn GUI)
#
# Flags: CGO_ENABLED=0, -trimpath -ldflags='-s -w -buildid='. Sizes are the
# raw stripped executables plus their gzip -9 size (an estimate of what a
# compressed download adds up to). Installers (.dmg etc.) are produced by
# `tan package`; see site/guide/size.md.
set -euo pipefail

repo="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

gomod() {
	local dir="$1"
	cat >"$dir/go.mod" <<EOF
module example.com/min

go 1.25

require github.com/tangoui/tango v0.0.0

replace github.com/tangoui/tango => $repo
EOF
}

mkdir -p "$work/web" "$work/ui"
cat >"$work/web/main.go" <<'EOF'
package main

import "github.com/tangoui/tango"

func main() {
	app := tango.New(tango.Options{Name: "Min"})
	app.OnReady(func() {
		app.NewSurface(tango.SurfaceOptions{Title: "Min", URL: "https://example.com"})
	})
	app.Run()
}
EOF
cat >"$work/ui/main.go" <<'EOF'
package main

import (
	"github.com/tangoui/tango"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/uihost"
)

var clicks int

func page(c *ui.Context) {
	if ui.Button(c, "Click").Primary().Clicked() {
		clicks++
	}
}

func main() {
	app := tango.New(tango.Options{Name: "Min"})
	app.OnReady(func() {
		uihost.Open(app, uihost.Options{Surface: tango.SurfaceOptions{Title: "Min"}}, page, ui.RunnerOptions{})
	})
	app.Run()
}
EOF

for app in web ui; do
	gomod "$work/$app"
	cp "$repo/go.sum" "$work/$app/go.sum"
	(cd "$work/$app" && GOWORK=off GOFLAGS=-mod=mod go mod tidy >/dev/null 2>&1)
done

mib() { awk -v b="$1" 'BEGIN { printf "%.2f MiB", b / 1048576 }'; }

echo "$(go version), tango $(git -C "$repo" rev-parse --short HEAD), $(date +%Y-%m-%d)"
echo "flags: CGO_ENABLED=0 -trimpath -ldflags='-s -w -buildid='"
printf '%-4s %-14s %12s %10s %14s\n' app target bytes size "gzip -9"
for app in web ui; do
	for target in darwin/arm64 linux/amd64 windows/amd64; do
		goos="${target%/*}" goarch="${target#*/}"
		out="$work/out-$app-$goos-$goarch"
		(cd "$work/$app" && GOWORK=off CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
			go build -trimpath -ldflags='-s -w -buildid=' -o "$out" .)
		bytes=$(wc -c <"$out" | tr -d ' ')
		gz=$(gzip -9 -c "$out" | wc -c | tr -d ' ')
		printf '%-4s %-14s %12s %10s %14s\n' "$app" "$target" "$bytes" "$(mib "$bytes")" "$(mib "$gz")"
	done
done
