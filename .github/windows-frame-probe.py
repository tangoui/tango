"""Temporary Windows diagnostics overlay; never changes product source."""
import json
import os
from pathlib import Path

root = Path.cwd()
out = Path(os.environ['RUNNER_TEMP']) / 'frame-diag'
out.mkdir(exist_ok=True)
replacements = {}

def save(rel, text):
    dest = out / Path(rel).name
    dest.write_text(text)
    replacements[str((root / rel).resolve())] = str(dest.resolve())

rel = 'ui/editor/brackets_test.go'
s = Path(rel).read_text()
s = s.replace('"fmt"', '"fmt"\n"context"\n"runtime/trace"\n"unsafe"\n"golang.org/x/sys/windows"', 1)
s = s.replace('tr.beginFrame()\n\t\t\ttr.unsure = false\n\t\t\tstart := time.Now()', '''tr.beginFrame()
            tr.unsure = false
            frameDiag = frameDiagStats{}
            knownBefore, finalBefore := tr.known(), st.Model.Highlighter().FinalLines(snap)
            trace.Logf(context.Background(), "frame", "lines=%d frame=%d", c.lines, frame)
            region := trace.StartRegion(context.Background(), "bounded-frame")
            cpu0, cycles0 := frameThreadCounters()
            qpc0 := frameQPC()
            start := time.Now()''', 1)
s = s.replace('wall := time.Since(start)\n', '''wall := time.Since(start)
            qpcTicks := frameQPC() - qpc0
            qpcElapsed := time.Duration(qpcTicks * 1000000000 / frameQPCFreq())
            cpu1, cycles1 := frameThreadCounters()
            region.End()
            t.Logf("DIAG lines=%d frame=%d wall=%v qpc=%v thread100ns=%d cycles=%d known=%d->%d final=%d->%d stats=%+v", c.lines, frame, wall, qpcElapsed, cpu1-cpu0, cycles1-cycles0, knownBefore, tr.known(), finalBefore, st.Model.Highlighter().FinalLines(snap), frameDiag)
''', 1)
s += r'''

type frameDiagStats struct {
    EnsureNS, AdvanceNS, TokensNS, OpenNS, CloseNS int64
    EnsureCalls, AdvanceCalls, TokensCalls, OpenCalls, CloseCalls int
    DeadlineDriftNS, MaxDeadlineDriftNS int64
}
var frameDiag frameDiagStats
var diagKernel32 = windows.NewLazySystemDLL("kernel32.dll")
var diagTimes = diagKernel32.NewProc("GetThreadTimes")
var diagCycles = diagKernel32.NewProc("QueryThreadCycleTime")
var diagQPC = diagKernel32.NewProc("QueryPerformanceCounter")
var diagQPF = diagKernel32.NewProc("QueryPerformanceFrequency")

func frameQPC() int64 {
    var n int64
    ok, _, err := diagQPC.Call(uintptr(unsafe.Pointer(&n)))
    if ok == 0 { panic(err) }
    return n
}
func frameQPCFreq() int64 {
    var n int64
    ok, _, err := diagQPF.Call(uintptr(unsafe.Pointer(&n)))
    if ok == 0 || n <= 0 { panic(err) }
    return n
}

// CPU times are raw 100ns units, NOT a claim of 100ns resolution.
// Cycle counts are raw: Microsoft's contract prohibits conversion to time.
func frameThreadCounters() (uint64, uint64) {
    handle, err := windows.GetCurrentThread()
    if err != nil { panic(err) }
    var creation, exit, kernel, user windows.Filetime
    ok, _, err := diagTimes.Call(uintptr(handle), uintptr(unsafe.Pointer(&creation)), uintptr(unsafe.Pointer(&exit)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)))
    if ok == 0 { panic(err) }
    var cycles uint64
    ok, _, err = diagCycles.Call(uintptr(handle), uintptr(unsafe.Pointer(&cycles)))
    if ok == 0 { panic(err) }
    return (uint64(kernel.HighDateTime)<<32 | uint64(kernel.LowDateTime)) + (uint64(user.HighDateTime)<<32 | uint64(user.LowDateTime)), cycles
}
'''
assert 'region.End()' in s
save(rel, s)

for rel, funcs in {
    'ui/editor/brackets.go': [('func (t *bracketTree) ensure(', 'Ensure'), ('func (t *bracketTree) scanOpen(', 'Open'), ('func (t *bracketTree) scanClose(', 'Close')],
    'ui/editor/bracketpairs.go': [('func (s modelBrackets) advance(', 'Advance'), ('func (s modelBrackets) tokens(', 'Tokens')],
}.items():
    s = Path(rel).read_text()
    for prefix, field in funcs:
        index = s.index('{', s.index(prefix)) + 1
        body = f'\n diagStart := time.Now(); frameDiag.{field}Calls++; defer func() {{ frameDiag.{field}NS += time.Since(diagStart).Nanoseconds() }}()\n'
        s = s[:index] + body + s[index:]
    if rel.endswith('bracketpairs.go'):
        old = 'hl.Highlight(snap, upTo, time.Until(deadline))'
        assert s.count(old) == 1
        s = s.replace(old, '''hl.FrameProbeBefore(deadline)
            hl.Highlight(snap, upTo, time.Until(deadline))
            drift := hl.FrameProbeDrift().Nanoseconds()
            frameDiag.DeadlineDriftNS += drift
            frameDiag.MaxDeadlineDriftNS = max(frameDiag.MaxDeadlineDriftNS, drift)''')
    save(rel, s)

rel = 'ui/editor/highlight/highlight.go'
s = Path(rel).read_text()
old = 'deadline := start.Add(budget)'
assert s.count(old) == 1
s = s.replace(old, old + '\n frameProbeDrift = deadline.Sub(frameProbeTarget)', 1)
s += '''
var frameProbeTarget time.Time
var frameProbeDrift time.Duration
func (h *Highlighter) FrameProbeBefore(deadline time.Time) { frameProbeTarget = deadline }
func (h *Highlighter) FrameProbeDrift() time.Duration { return frameProbeDrift }
'''
save(rel, s)
(out / 'overlay.json').write_text(json.dumps({'Replace': replacements}))
print(out / 'overlay.json')
