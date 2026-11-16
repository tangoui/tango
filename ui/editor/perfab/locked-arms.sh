#!/usr/bin/env bash
# Run one matrix command under an atomic OUT lock.
#
# Usage:
#   OUT=/path/to/out locked-arms.sh ./ui/editor/perfab/arms.sh ...
#
# This wrapper is intentionally separate from arms.sh so an active matrix is
# never changed in place. A second invocation sharing OUT is rejected while
# the recorded owner PID is alive. Stale locks are not removed automatically.
set -euo pipefail

if [ "$#" -lt 1 ]; then
	echo "usage: OUT=/path/to/out $0 COMMAND [ARG ...]" >&2
	exit 2
fi

OUT=${OUT:?OUT must name the matrix output directory}
LOCK=${MATRIX_LOCK:-$OUT/.matrix.lock}
mkdir -p "$OUT"

if ! mkdir "$LOCK" 2>/dev/null; then
	owner=$(cat "$LOCK/owner.pid" 2>/dev/null || echo unknown)
	if [ "$owner" != unknown ] && kill -0 "$owner" 2>/dev/null; then
		echo "matrix already locked by live owner PID $owner: $LOCK" >&2
		exit 73
	fi
	echo "stale matrix lock exists: $LOCK (owner PID $owner); inspect and remove it explicitly" >&2
	exit 74
fi

owner=$$
printf '%s\n' "$owner" >"$LOCK/owner.pid"
{
	echo "pid=$owner"
	echo "ppid=$PPID"
	echo "started=$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
	echo "command=$*"
} >"$LOCK/owner.meta"

cleanup() {
	if [ -f "$LOCK/owner.pid" ] && [ "$(cat "$LOCK/owner.pid" 2>/dev/null || true)" = "$owner" ]; then
		rm -rf "$LOCK"
	fi
}
trap cleanup EXIT INT TERM

"$@"
