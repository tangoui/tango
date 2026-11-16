#!/usr/bin/env bash
# Interleaved multi-arm performance measurement of the editor benchmarks
# (ui/editor/perf_test.go); run.sh is its two-arm form.
#
#	ui/editor/perfab/arms.sh BENCH_REGEX ROUNDS LABEL=DIR:BIN:OFF ...
#
# Every arm is a test binary BIN of the ui/editor package of the module
# root DIR (build it with `go test -c -o BIN ./ui/editor` in DIR), run with
# TANGO_EDITOR_PERF_OFF=OFF (perf_ab_test.go; empty for none). A round runs
# every arm once, each a fresh process, the order rotated by one arm per
# round (round 1: arms 1..n, round 2: arms 2..n,1, ...), so no arm always
# runs first or after the same one. The first arm is the reference of the
# summary; give the reference twice (two labels, same DIR:BIN:OFF) to
# measure the A/A noise of the same rounds.
#
# BENCH_REGEX is a -test.bench pattern, or several separated by ";", each
# optionally with its own benchtime after "@" (one process each, back to
# back within the arm's run), e.g.
#
#	'^Benchmark(TypeAt1MLines|Scroll)$/^(mid|end|mid-bg|scale2)$@1000x;^BenchmarkOpen100MB$@10x'
#
# Before and after every arm's run the 1-minute load average (uptime) is
# recorded; a round is discarded as soon as a sample is >= LOAD_MAX (the
# arms left in it are not run) and run again, after waiting (30 s sleeps,
# at most 20 minutes) for the load to drop. Logs: $OUT/<label>/<round>.log, $OUT/load.log, $OUT/rounds.txt.
# At the end summarize.py prints the medians and ratios to the reference
# ($OUT/summary.md).
#
# Environment:
#	OUT        log directory (default: a new directory under $TMPDIR)
#	BENCHTIME  -test.benchtime of the patterns without "@" (default 300x)
#	LOAD_MAX   discard threshold (default 6)
#	AA         label of the reference's A/A copy (summarize.py's noise)
#	AA_REF     the arm AA copies, when it is not the reference (summarize.py)
#	COST=1     the arms turn features off: judge the reference's cost
#	           (summarize.py)
#	CPUPROFILE 1: write a CPU profile of every run next to its log
#	RESUME     1: keep the rounds already in $OUT/rounds.txt and add
#	           rounds up to ROUNDS (same arms; to reach a lower noise)
#
# All arms must use the same generated document: perfFile caches it in
# $TMPDIR/tango-editor-perf under a hash of perfTemplates, which must be
# identical in every DIR (checked). OFF needs a DIR with perf_ab_test.go
# (milestone 1 has none; checked).
set -euo pipefail

if [ $# -lt 4 ]; then
	echo "usage: $0 BENCH_REGEX ROUNDS LABEL=DIR:BIN:OFF LABEL=DIR:BIN:OFF ..." >&2
	exit 2
fi
REGEX=$1
ROUNDS=$2
shift 2
HERE=$(cd "$(dirname "$0")" && pwd)
OUT=${OUT:-$(mktemp -d "${TMPDIR:-/tmp}/perfab.XXXXXX")}
LOAD_MAX=${LOAD_MAX:-6}
BENCHTIME=${BENCHTIME:-300x}
mkdir -p "$OUT"

LABELS=() DIRS=() BINS=() OFFS=()
for arm in "$@"; do
	label=${arm%%=*}
	rest=${arm#*=}
	IFS=: read -r dir bin off <<<"$rest"
	if [ -z "$label" ] || [ "$label" = "$arm" ] || [ ! -d "$dir" ] || [ ! -x "$bin" ]; then
		echo "bad arm $arm (want LABEL=DIR:BIN:OFF with an existing DIR and BIN)" >&2
		exit 2
	fi
	LABELS+=("$label") DIRS+=("$(cd "$dir" && pwd)") BINS+=("$bin") OFFS+=("${off:-}")
done
N=${#LABELS[@]}

load1() { uptime | sed -E 's/.*load averages?: *([0-9.]+).*/\1/'; }
lock1() {
	if ! command -v ioreg >/dev/null 2>&1; then
		echo unknown
		return
	fi
	if ioreg -n Root -d 1 2>/dev/null | grep -q 'CGSSessionScreenIsLocked.*Yes'; then
		echo locked
	else
		echo unlocked
	fi
}
high() { awk -v l="$1" -v m="$LOAD_MAX" 'BEGIN { exit !(l >= m) }'; }

# waitload sleeps (30 s at a time, at most 40 times) until the load is
# below LOAD_MAX.
waitload() {
	local i l
	for i in $(seq 40); do
		l=$(load1)
		if ! high "$l"; then
			return 0
		fi
		echo "$(date +%T) load $l >= $LOAD_MAX, waiting ($i/40)" | tee -a "$OUT/load.log" >&2
		sleep 30
	done
	echo "load stayed >= $LOAD_MAX for 20 minutes" | tee -a "$OUT/load.log" >&2
	return 1
}

templates() { sed -n '/^var perfTemplates = /,/^}/p' "$1/ui/editor/perf_test.go" | shasum | cut -c1-12; }
want=$(templates "${DIRS[0]}")
for i in $(seq 0 $((N - 1))); do
	if [ "$(templates "${DIRS[$i]}")" != "$want" ]; then
		echo "perfTemplates of ${DIRS[$i]} differ from ${DIRS[0]}: not the same document" >&2
		exit 1
	fi
	if [ -n "${OFFS[$i]}" ] && ! grep -q TANGO_EDITOR_PERF_OFF "${DIRS[$i]}/ui/editor/perf_ab_test.go" 2>/dev/null; then
		echo "arm ${LABELS[$i]}: ${DIRS[$i]} has no TANGO_EDITOR_PERF_OFF" >&2
		exit 1
	fi
	mkdir -p "$OUT/${LABELS[$i]}"
done

{
	echo "date: $(date '+%F %T')"
	for i in $(seq 0 $((N - 1))); do
		echo "arm ${LABELS[$i]}: dir=${DIRS[$i]} bin=${BINS[$i]} sha=$(shasum "${BINS[$i]}" | cut -c1-12) off=${OFFS[$i]}"
	done
	echo "bench: $REGEX benchtime=$BENCHTIME rounds=$ROUNDS load_max=$LOAD_MAX"
	echo "go: $(go version 2>/dev/null || echo ?)"
} | tee -a "$OUT/meta.txt"

# run i r: runs arm i for round r into $OUT/<label>/<r>.log. Returns 1 when
# a load sample was >= LOAD_MAX.
run() {
	local i=$1 r=$2 log l0 l1 s0 s1 part pat bt k=0 parts prof=()
	log=$OUT/${LABELS[$i]}/$r.log
	l0=$(load1)
	s0=$(lock1)
	: >"$log"
	IFS=';' read -r -a parts <<<"$REGEX"
	for part in "${parts[@]}"; do
		k=$((k + 1))
		pat=${part%@*}
		bt=$BENCHTIME
		if [ "$pat" != "$part" ]; then bt=${part##*@}; fi
		if [ "${CPUPROFILE:-}" = 1 ]; then prof=(-test.cpuprofile "$OUT/${LABELS[$i]}/$r.$k.cpu.prof"); fi
		# The working directory is the package's own, as `go test` runs it.
		(cd "${DIRS[$i]}/ui/editor" && TANGO_EDITOR_PERF_OFF=${OFFS[$i]} "${BINS[$i]}" -test.run '^$' \
			-test.bench "$pat" -test.benchtime "$bt" -test.count 1 -test.v ${prof[@]+"${prof[@]}"}) >>"$log" 2>&1 || {
			echo "run ${LABELS[$i]} round $r failed:" >&2
			tail -20 "$log" >&2
			exit 1
		}
	done
	l1=$(load1)
	s1=$(lock1)
	echo "$r ${LABELS[$i]} $l0 $l1 $s0 $s1" >>"$OUT/load.log"
	printf '%s round %s %s: load %s -> %s, screen %s -> %s\n' "$(date +%T)" "$r" "${LABELS[$i]}" "$l0" "$l1" "$s0" "$s1" >&2
	if high "$l0" || high "$l1"; then return 1; fi
	return 0
}

if [ "${RESUME:-}" = 1 ] && [ -s "$OUT/rounds.txt" ]; then
	r=$(($(tail -1 "$OUT/rounds.txt") + 1))
else
	: >"$OUT/rounds.txt"
	r=1
fi
tries=0
while [ "$r" -le "$ROUNDS" ]; do
	tries=$((tries + 1))
	if [ "$tries" -gt $((2 * ROUNDS + 5)) ]; then # bounded: a busy machine stops the run
		echo "too many discarded rounds" >&2
		exit 1
	fi
	waitload
	ok=1
	for j in $(seq 0 $((N - 1))); do
		# A load sample >= LOAD_MAX discards the round: stop it at once
		# rather than running the remaining arms for nothing.
		if ! run $(((j + r - 1) % N)) "$r"; then
			ok=0
			break
		fi
	done
	if [ "$ok" = 0 ]; then
		echo "$(date +%T) round $r discarded (load >= $LOAD_MAX), running it again" | tee -a "$OUT/load.log" >&2
		for l in "${LABELS[@]}"; do
			if [ -e "$OUT/$l/$r.log" ]; then mv "$OUT/$l/$r.log" "$OUT/$l/$r.discarded$tries.log"; fi
		done
		continue
	fi
	echo "$r" >>"$OUT/rounds.txt"
	r=$((r + 1))
done

python3 -I "$HERE/summarize.py" "$OUT" "${LABELS[@]}" | tee "$OUT/summary.md"
