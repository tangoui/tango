#!/usr/bin/env bash
# Paired A/B performance measurement of the editor benchmarks
# (ui/editor/perf_test.go): arms.sh with two arms.
#
#	ui/editor/perfab/run.sh A_DIR B_DIR BENCH_REGEX ROUNDS
#
# A_DIR and B_DIR are module roots (a tree exported with
# `git archive <rev> | tar -x -C dir`, or the working tree). The script
# builds the ui/editor test binary of each tree once (`go test -c`), then
# runs them alternately, A B, B A, A B ... (arms.sh rotates the order),
# ROUNDS times, each run a fresh process; every round records the uptime
# load before and after each run and is discarded and run again when one
# is >= 6. It prints per benchmark and metric (p50/p95/p99/max, fps,
# open-ms, cpu-ms, gpuwait-ms ...) the medians of the rounds and the B/A
# ratio (summarize.py). BENCH_REGEX is described in arms.sh.
#
# Environment (and those of arms.sh: OUT, BENCHTIME, LOAD_MAX, CPUPROFILE):
#	A_OFF      TANGO_EDITOR_PERF_OFF for the A runs (perf_ab_test.go)
#	B_OFF      TANGO_EDITOR_PERF_OFF for the B runs
#	A_LABEL    names in the summary (default: A, B)
#	B_LABEL
#	A_BIN      a prebuilt A test binary (skips building A); same for B_BIN,
#	B_BIN      so several comparisons use the very same build
#	AA=1       also run A a second time (label "<A_LABEL>-aa") in every
#	           round: its ratio to A is the noise the verdicts account for
set -euo pipefail

if [ $# -ne 4 ]; then
	echo "usage: $0 A_DIR B_DIR BENCH_REGEX ROUNDS" >&2
	exit 2
fi
HERE=$(cd "$(dirname "$0")" && pwd)
OUT=${OUT:-$(mktemp -d "${TMPDIR:-/tmp}/perfab.XXXXXX")}
export OUT
mkdir -p "$OUT"
A_LABEL=${A_LABEL:-A}
B_LABEL=${B_LABEL:-B}
if [ -z "${A_BIN:-}" ]; then A_BIN=$OUT/a.test; (cd "$1" && go test -c -o "$A_BIN" ./ui/editor); fi
if [ -z "${B_BIN:-}" ]; then B_BIN=$OUT/b.test; (cd "$2" && go test -c -o "$B_BIN" ./ui/editor); fi
arms=("$A_LABEL=$1:$A_BIN:${A_OFF:-}" "$B_LABEL=$2:$B_BIN:${B_OFF:-}")
if [ "${AA:-}" = 1 ]; then
	arms+=("$A_LABEL-aa=$1:$A_BIN:${A_OFF:-}")
	export AA=$A_LABEL-aa
else
	unset AA
fi
exec "$HERE/arms.sh" "$3" "$4" "${arms[@]}"
