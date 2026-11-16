# Temporary branch-only focused capture. No threshold changes or reruns.
$ErrorActionPreference = 'Stop'
$dest = Join-Path $env:RUNNER_TEMP 'sparse-probe'
python .github/windows-sparse-probe.py
if ($LASTEXITCODE -ne 0) { throw 'overlay generation failed' }
$binary = Join-Path $dest 'editor.test.exe'
$overlay = Join-Path $dest 'overlay.json'
go test -c "-overlay=$overlay" -o $binary ./ui/editor
if ($LASTEXITCODE -ne 0) { throw 'overlay compilation failed' }

$wpr = Get-Command wpr.exe -ErrorAction SilentlyContinue
$xperf = Get-Command xperf.exe -ErrorAction SilentlyContinue
if (-not $xperf) {
    $kits = Join-Path ${env:ProgramFiles(x86)} 'Windows Kits'
    $found = Get-ChildItem $kits -Filter xperf.exe -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($found) { $xperf = $found.FullName }
} else { $xperf = $xperf.Source }
"wpr=$($wpr.Source) xperf=$xperf" | Tee-Object (Join-Path $dest 'tools.log')
if (-not $wpr) { throw 'ETW capture unavailable: wpr.exe not installed' }
& $wpr.Source -profiles | Out-File (Join-Path $dest 'wpr-profiles.log')
& $wpr.Source -start GeneralProfile -filemode
if ($LASTEXITCODE -ne 0) { throw 'ETW capture unavailable: WPR start failed (no native test was run)' }
$etl = Join-Path $dest 'focused.etl'
try {
    & $binary '-test.run=^TestMatchManyCursorsBounded$' '-test.count=1' '-test.v=true' *> (Join-Path $dest 'focused.log')
    $testResult = $LASTEXITCODE
} finally {
    & $wpr.Source -stop $etl *> (Join-Path $dest 'wpr-stop.log')
    $stopResult = $LASTEXITCODE
}
Get-Content (Join-Path $dest 'focused.log')
"testExit=$testResult wprStop=$stopResult" | Tee-Object (Join-Path $dest 'result.log')
if ($stopResult -ne 0) { throw 'WPR stop failed; capture incomplete' }
if ($xperf) {
    & $xperf -i $etl -o (Join-Path $dest 'events.csv') -a dumper *> (Join-Path $dest 'xperf.log')
    "xperfExit=$LASTEXITCODE" | Add-Content (Join-Path $dest 'result.log')
} else {
    # Preserve native event XML, including registered payload names and clock
    # metadata, when the optional WPT dumper is not installed on the runner.
    try {
        Get-WinEvent -Path $etl -Oldest -ErrorAction Stop | ForEach-Object {
            if ($_.Opcode -in 36,50 -or $_.ProviderName -match 'Kernel-(Thread|Process)') { $_.ToXml() }
        } | Out-File -Encoding utf8 (Join-Path $dest 'schedule-events.xml')
    } catch { $_ | Out-File (Join-Path $dest 'decode-error.log') }
}
# A failing measured test is retained as data; this diagnostic step does not
# convert it to a pass. Acceptance remains the separate original-gate step.
if ($testResult -ne 0) { exit $testResult }
