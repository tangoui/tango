#!/usr/bin/env bash
# Measures the idle memory of examples/macoslite-counter on macOS.
#
#   bash scripts/macoslite-memory.sh [--rounds N]     default 3 rounds
#
# Each round starts the counter, waits for its first frame, lets it idle for
# 7 s and reads the process' physical footprint (the number Activity
# Monitor's "Memory" column shows; `footprint`, not RSS). It prints each
# round, the median, the stripped binary size, and whether any WebKit
# process belongs to the counter (there must be none).
#
# The screen must be unlocked: a locked session does not draw windows and the
# numbers would not be comparable, so the script refuses to run then.
set -euo pipefail

rounds=3
if [[ "${1:-}" == "--rounds" ]]; then rounds="${2:?--rounds needs a number}"; fi

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

samples=()
for ((i = 1; i <= rounds; i++)); do
	log="$work/run$i.log"
	MACOSLITE_FIRST_DRAW=1 "$bin" >"$log" 2>&1 &
	pid=$!
	for _ in $(seq 50); do
		grep -q first-draw "$log" && break
		sleep 0.1
	done
	grep -q first-draw "$log" || { echo "round $i: no first frame" >&2; exit 1; }
	sleep 7
	bytes=$(footprint -p "$pid" -j "$work/fp$i.json" >/dev/null 2>&1 &&
		python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print(d["processes"][0]["footprint"])' "$work/fp$i.json")
	webkit=$( (pgrep -P "$pid" -f WebKit || true) | wc -l | tr -d ' ')
	kill "$pid"
	wait "$pid" 2>/dev/null || true
	pid=""
	samples+=("$bytes")
	echo "round $i: footprint $(mib "$bytes") MiB, WebKit children $webkit"
	sleep 1
done

median=$(printf '%s\n' "${samples[@]}" | sort -n | awk '{ a[NR] = $1 } END { print a[int((NR + 1) / 2)] }')
echo "median footprint: $(mib "$median") MiB over $rounds rounds"
echo "binary size: $(mib "$size") MiB (stripped, CGO_ENABLED=0)"
