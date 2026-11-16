"""Single-variable experiment: enforce HighlightFor's budget using QPC.
Temporary branch-only overlay, not product implementation.
"""
import json
import os
from pathlib import Path

source = Path('ui/editor/highlight/highlight.go').resolve()
text = source.read_text(encoding='utf-8')
old = 'start := time.Now()\n\th.mu.Lock()'
assert text.count(old) == 1
text = text.replace(old, 'preciseStart := probeQPC()\n\t' + old)
assert text.count('deadline := start.Add(budget)') == 1
text = text.replace('deadline := start.Add(budget)', '')
assert text.count('if !time.Now().Before(deadline) {') == 1
text = text.replace('if !time.Now().Before(deadline) {', 'if time.Duration((probeQPC()-preciseStart)*1000000000/probeFrequency) >= budget {')
text = text.replace('"context"', '"context"\n"unsafe"\n"golang.org/x/sys/windows"', 1)
text += r'''
var probeKernel = windows.NewLazySystemDLL("kernel32.dll")
var probeCounter = probeKernel.NewProc("QueryPerformanceCounter")
var probeFreq = probeKernel.NewProc("QueryPerformanceFrequency")
var probeFrequency = func() int64 {
    var n int64
    ok, _, err := probeFreq.Call(uintptr(unsafe.Pointer(&n)))
    if ok == 0 || n <= 0 { panic(err) }
    return n
}()
func probeQPC() int64 {
    var n int64
    ok, _, err := probeCounter.Call(uintptr(unsafe.Pointer(&n)))
    if ok == 0 { panic(err) }
    return n
}
'''
out = Path(os.environ['RUNNER_TEMP']) / 'precise-probe'
out.mkdir(exist_ok=True)
replacement = out / 'highlight.go'
replacement.write_text(text, encoding='utf-8')
(out / 'overlay.json').write_text(json.dumps({'Replace': {str(source): str(replacement.resolve())}}), encoding='utf-8')
