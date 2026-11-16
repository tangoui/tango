#!/usr/bin/env bash
# Measures the idle memory of examples/macoslite-counter on macOS.
#
#   bash scripts/macoslite-memory.sh [--rounds N]     default 3 rounds
#
# Each round starts the counter, times launch to its first frame, lets it
# idle for 7 s and reads the process' physical footprint (the number
# Activity Monitor's "Memory" column shows; `footprint`, not RSS). It prints
# each round, the medians and the stripped binary size.
#
# It also reports which heavy system images the process mapped. macoslite
# itself never creates a WebView or a Metal device, but AppKit maps
# Metal.framework as a dependency and, on macOS 15+, maps WebKit.framework
# through WritingToolsUI once a window becomes key. Neither starts a
# WebContent process; the script prints whether one is running for this
# binary so an accidental WebView would show up.
#
# The screen must be unlocked: a locked session does not draw windows and the
# numbers would not be comparable, so the script refuses to run then.
set -euo pipefail

rounds=3
if [[ "${1:-}" == "--rounds" ]]; then rounds="${2:?--rounds needs a number}"; fi
[[ "$rounds" =~ ^[1-9][0-9]*$ ]] || { echo "--rounds needs a positive integer" >&2; exit 2; }
command -v python3 >/dev/null || { echo "python3 is required" >&2; exit 1; }

[[ "$(uname -s)" == Darwin ]] || { echo "macOS only" >&2; exit 1; }
if ioreg -n Root -d1 | grep -q '"CGSSessionScreenIsLocked"=Yes'; then
	echo "screen is locked; unlock it and run again" >&2
	exit 1
fi

repo="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
pid=""
trap '[[ -n "$pid" ]] && kill "$pid" 2>/dev/null; rm -rf "$work"' EXIT

bin="$work/macoslite-counter"
(cd "$repo" && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o "$bin" ./examples/macoslite-counter)
size=$(stat -f %z "$bin")

mib() { awk -v b="$1" 'BEGIN { printf "%.1f", b / 1048576 }'; }
now_ms() { python3 -c 'import time; print(int(time.time() * 1000))'; }
median() { printf '%s\n' "$@" | sort -n | awk '{ a[NR] = $1 } END { print a[int((NR + 1) / 2)] }'; }

samples=()
firsts=()
for ((i = 1; i <= rounds; i++)); do
	log="$work/run$i.log"
	start=$(now_ms)
	MACOSLITE_FIRST_DRAW=1 "$bin" >"$log" 2>&1 &
	pid=$!
	for _ in $(seq 1000); do
		grep -q first-draw "$log" && break
		sleep 0.005
	done
	grep -q first-draw "$log" || { echo "round $i: no first frame within 5 s" >&2; exit 1; }
	first=$(($(now_ms) - start))
	sleep 7
	footprint -p "$pid" -j "$work/fp$i.json" >/dev/null 2>&1 || { echo "round $i: footprint failed" >&2; exit 1; }
	bytes=$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print(d["processes"][0]["footprint"])' "$work/fp$i.json")
	images=$(vmmap -w "$pid" 2>/dev/null | grep -oE '/System/Library/Frameworks/(WebKit|Metal)\.framework' | sort -u |
		sed -E 's|.*/([A-Za-z]+)\.framework|\1|' | paste -sd, - || true)
	webcontent=$( (pgrep -f "com.apple.WebKit.WebContent.*$(basename "$bin")" || true) | wc -l | tr -d ' ')
	kill "$pid"
	wait "$pid" 2>/dev/null || true
	pid=""
	samples+=("$bytes")
	firsts+=("$first")
	echo "round $i: footprint $(mib "$bytes") MiB, first frame ${first} ms, mapped images [${images:-none}], WebContent processes $webcontent"
	sleep 1
done

echo "median footprint: $(mib "$(median "${samples[@]}")") MiB over $rounds rounds"
echo "median first frame: $(median "${firsts[@]}") ms (launch to first drawn frame)"
echo "binary size: $(mib "$size") MiB (stripped, CGO_ENABLED=0)"
