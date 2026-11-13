<#
.SYNOPSIS
  Runs Tango's Windows smoke programs and summarizes PASS/FAIL.

.DESCRIPTION
  The Windows counterpart of scripts/smoke.sh, used locally and by CI
  (.github/workflows/ci.yml); see docs/ci.md.

    pwsh scripts/smoke.ps1                  all Windows smoke programs
    pwsh scripts/smoke.ps1 desktop NAME..   only the named programs
    pwsh scripts/smoke.ps1 list             print the program names

  Each program opens real windows for a few seconds and quits by itself.
  The exit status is non-zero if any program failed, timed out or did not
  build.

  Environment (all optional):
    SMOKE_OUT                  logs, screenshots, summary.txt (default build\smoke)
    SMOKE_TIMEOUT              hard limit per program in seconds (default 120)
    SMOKE_SCREENSHOT=1         take a screenshot while each program runs
    SMOKE_SHOT_DELAY           seconds after start for that screenshot (default 2)
    TANGO_SMOKE_TIMEOUT_SCALE  stretches the programs' own timeouts (CI uses 3)
    TANGO_SMOKE_SKIP           check groups notify-demo skips (e.g. notify)
#>
param(
    [string]$Mode = 'desktop',
    [Parameter(ValueFromRemainingArguments = $true)][string[]]$Names = @()
)

$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
Set-Location $repo

$out = if ($env:SMOKE_OUT) { $env:SMOKE_OUT } else { Join-Path $repo 'build\smoke' }
New-Item -ItemType Directory -Force -Path $out | Out-Null
$out = (Resolve-Path $out).Path
$limit = if ($env:SMOKE_TIMEOUT) { [int]$env:SMOKE_TIMEOUT } else { 120 }
$shotDelay = if ($env:SMOKE_SHOT_DELAY) { [int]$env:SMOKE_SHOT_DELAY } else { 2 }

$programs = @(
    @{ Name = 'windows-smoke'; Pkg = './internal/platform/windows/internal/smoke'; Args = @() },
    @{ Name = 'smokecanvas'; Pkg = './internal/platform/windows/internal/smokecanvas'; Args = @('-smoke') },
    @{ Name = 'hello'; Pkg = './examples/hello'; Args = @('-smoke') },
    @{ Name = 'app-demo'; Pkg = './examples/app-demo'; Args = @('-smoke') },
    @{ Name = 'system-demo'; Pkg = './examples/system-demo'; Args = @() },
    @{ Name = 'notify-demo'; Pkg = './examples/notify-demo'; Args = @() }
)

$results = New-Object System.Collections.Generic.List[string]
$script:failed = 0

function Record([string]$name, [string]$status, [string]$detail) {
    $results.Add(('{0,-14} {1,-8} {2}' -f $name, $status, $detail))
    if ($status -ne 'PASS') { $script:failed++ }
}

function Save-Screenshot([string]$file) {
    try {
        Add-Type -AssemblyName System.Windows.Forms, System.Drawing
        $b = [System.Windows.Forms.SystemInformation]::VirtualScreen
        $bmp = New-Object System.Drawing.Bitmap $b.Width, $b.Height
        $g = [System.Drawing.Graphics]::FromImage($bmp)
        $g.CopyFromScreen($b.Left, $b.Top, 0, 0, $bmp.Size)
        $bmp.Save($file, [System.Drawing.Imaging.ImageFormat]::Png)
        $g.Dispose(); $bmp.Dispose()
    } catch {
        Write-Host "    (screenshot failed: $_)"
    }
}

function Show-Log([string]$file) {
    if (Test-Path $file) { Get-Content $file | ForEach-Object { "    $_" } | Write-Host }
}

function Invoke-Smoke($p) {
    $name = $p.Name
    $exe = Join-Path $out "bin\$name.exe"
    Write-Host "==> build $name ($($p.Pkg))"
    $buildLog = Join-Path $out "$name.build.log"
    # go writes progress to stderr; keep it out of PowerShell's error stream.
    $ErrorActionPreference = 'Continue'
    & go build -o $exe $p.Pkg *> $buildLog
    $buildRc = $LASTEXITCODE
    $ErrorActionPreference = 'Stop'
    if ($buildRc -ne 0) {
        Show-Log $buildLog
        Record $name 'FAIL' "build failed, see $buildLog"
        return
    }
    Remove-Item $buildLog -ErrorAction SilentlyContinue

    $log = Join-Path $out "$name.log"
    $errLog = Join-Path $out "$name.stderr.log"
    Write-Host "==> run $name $($p.Args -join ' ')"
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    $start = @{ FilePath = $exe; PassThru = $true; NoNewWindow = $true
        RedirectStandardOutput = $log; RedirectStandardError = $errLog
    }
    if ($p.Args.Count -gt 0) { $start.ArgumentList = $p.Args }
    $proc = Start-Process @start
    $null = $proc.Handle # without a cached handle ExitCode can come back empty
    if ($env:SMOKE_SCREENSHOT -eq '1') {
        if (-not $proc.WaitForExit($shotDelay * 1000)) { Save-Screenshot (Join-Path $out "$name.png") }
    }
    $timedOut = -not $proc.WaitForExit($limit * 1000)
    if ($timedOut) {
        try { $proc.Kill() } catch {}
        $proc.WaitForExit(5000) | Out-Null
    }
    $proc.WaitForExit() # flushes the redirected streams and sets ExitCode
    $secs = [int]$sw.Elapsed.TotalSeconds
    if ((Test-Path $errLog) -and (Get-Item $errLog).Length -gt 0) {
        Add-Content -Path $log -Value (Get-Content $errLog)
    }
    Remove-Item $errLog -ErrorAction SilentlyContinue
    Show-Log $log
    if ($timedOut) {
        Record $name 'TIMEOUT' "killed after ${limit}s"
    } elseif ($proc.ExitCode -eq 0) {
        Record $name 'PASS' "${secs}s"
    } else {
        Record $name 'FAIL' "exit $($proc.ExitCode) after ${secs}s, see $log"
    }
}

switch ($Mode) {
    'list' { $programs | ForEach-Object { $_.Name }; exit 0 }
    'desktop' { }
    default { Write-Error "smoke.ps1: unknown mode '$Mode' (desktop, list)"; exit 2 }
}

New-Item -ItemType Directory -Force -Path (Join-Path $out 'bin') | Out-Null
$selected = $programs | Where-Object { $Names.Count -eq 0 -or $Names -contains $_.Name }
if (-not $selected) { Write-Error "no smoke program matched: $Names"; exit 2 }
foreach ($p in $selected) { Invoke-Smoke $p }

$lines = @("smoke summary (Windows desktop, $((Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')))")
$lines += $results | ForEach-Object { "  $_" }
if ($script:failed -eq 0) { $lines += "ALL PASS ($($results.Count))" } else { $lines += "FAILED: $($script:failed) of $($results.Count)" }
$lines | Set-Content -Path (Join-Path $out 'summary.txt') -Encoding utf8
$lines | Write-Host
if ($env:GITHUB_STEP_SUMMARY) {
    Add-Content -Path $env:GITHUB_STEP_SUMMARY -Value (@('```') + $lines + @('```'))
}
if ($script:failed -ne 0) { exit 1 }
exit 0
