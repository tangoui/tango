#!/usr/bin/env bash
# Prepare (but do not build or run) the M1 -> final editor performance matrix.
#
# Baseline: M1 commit 0ef80e4.
# Candidate: the current isolated worktree (F + P candidate when invoked there).
#
# This script deliberately defaults to preparation only. It exports M1 into a
# temporary directory, creates an overlay that adds the real Metal Wait API to
# the old package and adapts M1's benchmark host to use it, then prints the
# exact build and arms.sh commands for a later, explicitly authorized run.
set -euo pipefail

HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../../.." && pwd)
REV=${M1_REV:-0ef80e4}
OUT=${OUT:-"$(mktemp -d "${TMPDIR:-/tmp}/tango-editor-m1-final.XXXXXX")"}
M1="$OUT/m1-tree"
OVERLAY="$OUT/m1-overlay.json"
ADAPTER="$OUT/m1-wait-adapter_darwin.go"
PERF="$OUT/m1-perf_test.go"

if [ "${1:-}" != "--prepare-only" ] && [ "${1:-}" != "" ]; then
	echo "usage: $0 [--prepare-only]" >&2
	exit 2
fi

mkdir -p "$M1"
git -C "$ROOT" archive "$REV" | tar -x -C "$M1"

# Both arms must use byte-identical generated TypeScript templates. This is a
# source check only; it does not build or run either arm.
template_hash() {
	sed -n '/^var perfTemplates = /,/^}/p' "$1/ui/editor/perf_test.go" | shasum | cut -c1-12
}
M1_TEMPLATE_HASH=$(template_hash "$M1")
FINAL_TEMPLATE_HASH=$(template_hash "$ROOT")
if [ "$M1_TEMPLATE_HASH" != "$FINAL_TEMPLATE_HASH" ]; then
	echo "perfTemplates mismatch: M1=$M1_TEMPLATE_HASH final=$FINAL_TEMPLATE_HASH" >&2
	exit 1
fi

# M1's package has an unexported wait helper but no exported Device.Wait. The
# overlay replaces the excluded *_other.go file with this darwin-only adapter;
# it does not alter the archived tree or any repository file.
cat >"$ADAPTER" <<'EOF'
//go:build darwin

package metal

import "runtime"

// Wait waits for command buffers already committed by Execute without
// submitting a synthetic clear/1px frame. This is an overlay-only adapter for
// the M1 comparison; it is not written into the M1 source tree.
func (d *Device) Wait() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return ErrClosed
	}
	if d.lost != nil {
		return d.lost
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer autoreleasePool()()
	_, err := d.wait()
	if d.lost != nil {
		return d.lost
	}
	if err != nil {
		return err
	}
	return d.pendingErr
}
EOF

# Adapt only M1's benchmark host. The benchmark names, generator,
# dimensions, scale and rendering path remain those from 0ef80e4.
python3 -I - "$M1/ui/editor/perf_test.go" "$PERF" <<'PY'
from pathlib import Path
import re
import sys

src = Path(sys.argv[1]).read_text()
out = Path(sys.argv[2])
replacement = '''type syncDevice struct {
\td *metal.Device

\tframes, partial int
}

func (s *syncDevice) Execute(f *gpu.Frame) error {
\ts.frames++
\tif len(f.Dirty) > 0 {
\t\ts.partial++
\t}
\treturn s.d.Execute(f)
}

type retainDevice struct{ *syncDevice }

func (r retainDevice) Retained(size image.Point) bool { return r.d.Retained(size) }

func (s *syncDevice) wait() error { return s.d.Wait() }
'''
pattern = re.compile(r'type syncDevice struct \{.*?^func \(s \*syncDevice\) wait\(\) error \{.*?^\}\n', re.M | re.S)
s, n = pattern.subn(replacement, src, count=1)
if n != 1:
    raise SystemExit('M1 syncDevice block was not found exactly once')
s = s.replace(', keep: true', '', 1)
out.write_text(s)
PY

python3 -I - "$OVERLAY" "$M1/ui/editor/perf_test.go" "$PERF" "$M1/ui/gpu/metal/metal_other.go" "$ADAPTER" <<'PY'
import json
from pathlib import Path
import sys

overlay, perf_src, perf_dst, metal_src, metal_dst = map(Path, sys.argv[1:])
overlay.write_text(json.dumps({"Replace": {
    str(perf_src): str(perf_dst),
    str(metal_src): str(metal_dst),
}}, indent=2) + "\n")
PY

# Preparation-only integrity checks: no compiler, test binary, browser or
# benchmark is started. These are source checks, not performance evidence.
[ -s "$OVERLAY" ]
[ -s "$PERF" ]
[ "$(grep -c 'func (d \*Device) Wait' "$ADAPTER")" -eq 1 ]
[ "$(grep -c 'func (s \*syncDevice) wait' "$PERF")" -eq 1 ]

echo "prepared M1 tree: $M1"
echo "prepared overlay: $OVERLAY"
echo "candidate root: $ROOT"
echo "candidate HEAD: $(git -C "$ROOT" rev-parse HEAD)"
echo "perfTemplates hash: $FINAL_TEMPLATE_HASH (same in M1 and final)"
echo "candidate includes reviewed F/P Metal plus diffview Wait benchmark fix; it is not a pure F+P-only tree."
echo
echo "Not run: go test, go test -c, browser, benchmark, or sampling."
echo
echo "Later build commands (do not run until resource handoff):"
echo "  (cd '$M1' && go test -overlay '$OVERLAY' -c -o '$OUT/m1.test' ./ui/editor)"
echo "  (cd '$ROOT' && go test -c -o '$OUT/final.test' ./ui/editor)"
echo
echo "Later shared-gate matrix (only exact shared million-line gates; 20 valid rounds):"
printf '%s\n' \
"  OUT='$OUT/m1-final' AA=m1-aa '$ROOT/ui/editor/perfab/arms.sh' \\" \
"    '^BenchmarkTypeAt1MLines/(mid|end|mid-bg)\$@300x;^BenchmarkScroll/scale2\$@300x;^BenchmarkOpen100MB\$@10x' 20 \\" \
"    'm1=$M1:$OUT/m1.test:' 'm1-aa=$M1:$OUT/m1.test:' \\" \
"    'final=$ROOT:$OUT/final.test:' 'final-alloff=$ROOT:$OUT/final.test:all'"
echo
echo "M1 keeps default features; final-alloff is a separate final default-to-alloff arm."
echo "Do not compare M1 against ScrollPartial/TypeTS20k/TypeAt1MLinesCPU or final-only long-line gates."
