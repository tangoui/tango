"""Temporary low-frequency Windows markers; no per-token instrumentation."""
import json
import os
from pathlib import Path

root = Path.cwd()
out = Path(os.environ['RUNNER_TEMP']) / 'sparse-probe'
out.mkdir(exist_ok=True)
replacements = {}

def save(rel, text):
    dest = out / Path(rel).name
    dest.write_text(text, encoding='utf-8')
    replacements[str((root / rel).resolve())] = str(dest.resolve())

rel = 'ui/editor/brackets_test.go'
s = Path(rel).read_text(encoding='utf-8')
s = s.replace('"fmt"', '"fmt"\n"encoding/json"\n"os"\n"unsafe"\n"golang.org/x/sys/windows"\n"github.com/tangoui/tango/ui/editor/highlight"', 1)
old = 'defer runtime.UnlockOSThread()\n\tfor _, c := range'
assert s.count(old) == 1
s = s.replace(old, '''defer runtime.UnlockOSThread()
    sparseRows = make([]sparseRow, 0, 4096)
    sparseTID = windows.GetCurrentThreadId()
    highlight.SparseFrameProbe = func(kind string, deadline time.Time) { sparseRecord(kind, deadline, -1) }
    defer func() {
        highlight.SparseFrameProbe = nil
        t.Logf("SPARSE-META pid=%d tid=%d frequency=%d", os.Getpid(), sparseTID, sparseFrequency)
        for _, row := range sparseRows { data, err := json.Marshal(row); if err != nil { panic(err) }; t.Logf("SPARSE %s", data) }
    }()
    for _, c := range''')
old = 'tr.beginFrame()\n\t\t\ttr.unsure = false\n\t\t\tstart := time.Now()'
assert s.count(old) == 1
s = s.replace(old, '''tr.beginFrame()
            tr.unsure = false
            sparseLines, sparseFrame = c.lines, frame
            sparseRecord("frame.begin", time.Time{}, tr.known())
            start := time.Now()''')
old = 'wall := time.Since(start)\n'
assert s.count(old) == 1
s = s.replace(old, old + '            sparseRecord("frame.end", start.Add(budget), tr.known())\n', 1)
s += r'''

type sparseRow struct {
    Kind string
    QPC int64
    GoNS int64
    RemainingNS int64
    Lines, Frame, Known int
}
var sparseRows []sparseRow
var sparseTID uint32
var sparseLines, sparseFrame int
var sparseKernel = windows.NewLazySystemDLL("kernel32.dll")
var sparseCounter = sparseKernel.NewProc("QueryPerformanceCounter")
var sparseFreq = sparseKernel.NewProc("QueryPerformanceFrequency")
var sparseFrequency = func() int64 {
    var n int64
    ok, _, err := sparseFreq.Call(uintptr(unsafe.Pointer(&n)))
    if ok == 0 || n <= 0 { panic(err) }
    return n
}()
func sparseRecord(kind string, deadline time.Time, known int) {
    if len(sparseRows) >= cap(sparseRows) { panic("sparse diagnostic capacity exceeded") }
    var n int64
    ok, _, err := sparseCounter.Call(uintptr(unsafe.Pointer(&n)))
    if ok == 0 { panic(err) }
    now := time.Now()
    remaining := int64(0)
    if !deadline.IsZero() { remaining = deadline.Sub(now).Nanoseconds() }
    sparseRows = append(sparseRows, sparseRow{kind, n, now.UnixNano(), remaining, sparseLines, sparseFrame, known})
}
'''
save(rel, s)

rel = 'ui/editor/brackets.go'
s = Path(rel).read_text(encoding='utf-8')
old = 'func (t *bracketTree) ensure(src bracketSource, upTo int, deadline time.Time) bool {'
assert s.count(old) == 1
s = s.replace(old, old + '\n defer func() { sparseRecord("ensure.return", deadline, t.known()) }()', 1)
old = 't.overrun = true // the frame\'s overrun: minOverrun more'
assert s.count(old) == 1
s = s.replace(old, 'sparseRecord("grace.grant", deadline, t.known())\n\t\t' + old, 1)
save(rel, s)

rel = 'ui/editor/highlight/highlight.go'
s = Path(rel).read_text(encoding='utf-8')
old = 'deadline := start.Add(budget)'
assert s.count(old) == 1
s = s.replace(old, old + '\n if SparseFrameProbe != nil { defer SparseFrameProbe("highlight.return", deadline) }', 1)
s += '\nvar SparseFrameProbe func(kind string, deadline time.Time)\n'
save(rel, s)
(out / 'overlay.json').write_text(json.dumps({'Replace': replacements}), encoding='utf-8')
