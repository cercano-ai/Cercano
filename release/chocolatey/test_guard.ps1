# Tests for guard.ps1 against a throwaway fake Chocolatey root in TEMP.
# Fixture processes are copies of ping.exe started and stopped by this test
# only; the real Chocolatey installation and other processes are never touched.
# Checks look only at this test's fixtures, so other cercano.exe processes on
# the machine do not change the result.
# Run: powershell -NoProfile -ExecutionPolicy Bypass -File release\chocolatey\test_guard.ps1
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'guard.ps1')

$failures = 0
function Check([string] $name, [bool] $condition) {
    if ($condition) { Write-Host "ok   $name" }
    else { Write-Host "FAIL $name"; $script:failures++ }
}
function Has($found, $process) { [bool]($found | Where-Object ProcessId -eq $process.Id) }
function Agent { @(Get-CercanoPackageProcess -PackageId 'cercano-agent' -Executables @('cercano.exe')) }

$base = Join-Path ([IO.Path]::GetTempPath()) ("cercano-guard-test-" + [guid]::NewGuid().ToString('N'))
$fake = Join-Path $base 'choco'
$junction = Join-Path $base 'choco-junction'
$started = @()
$savedRoot = $env:ChocolateyInstall
try {
    New-Item -ItemType Directory -Force -Path (Join-Path $fake 'lib\cercano-agent\tools') | Out-Null
    $env:ChocolateyInstall = $fake
    $ping = Join-Path $env:SystemRoot 'System32\PING.EXE'

    function Start-Fixture([string] $relativeDir, [string] $exe) {
        $dir = Join-Path $fake $relativeDir
        New-Item -ItemType Directory -Force -Path $dir | Out-Null
        $path = Join-Path $dir $exe
        Copy-Item $ping $path
        $p = Start-Process -FilePath $path -ArgumentList '-n', '60', '127.0.0.1' -WindowStyle Hidden -PassThru
        $script:started += $p
        return $p
    }

    $lookalike = Start-Fixture 'lib\cercano-agent-other\tools' 'cercano.exe'
    $elsewhere = Start-Fixture 'not-chocolatey' 'cercano.exe'
    $installed = Start-Fixture 'lib\cercano-agent\tools' 'cercano.exe'
    $backup = Start-Fixture 'lib-bkp\cercano-agent\tools' 'cercano.exe'
    $shim = Start-Fixture 'bin' 'cercano.exe'
    $other = Start-Fixture 'lib\cercano-cli\tools' 'cercano-cli.exe'
    Start-Sleep -Milliseconds 500

    $found = Agent
    Check 'installed copy is detected' (Has $found $installed)
    Check 'old copy moved to lib-bkp is detected' (Has $found $backup)
    Check 'running public shim is detected' (Has $found $shim)
    Check 'prefix lookalike package dir is not ours' (-not (Has $found $lookalike))
    Check 'same name outside Chocolatey is not ours' (-not (Has $found $elsewhere))
    Check 'the other component is not ours' (-not (Has $found $other))

    # Regression: ChocolateyInstall written differently from the process path.
    New-Item -ItemType Junction -Path $junction -Target $fake | Out-Null
    $env:ChocolateyInstall = $junction
    $found = Agent
    Check 'detected when ChocolateyInstall is a junction' ((Has $found $installed) -and (Has $found $shim))
    $short = (New-Object -ComObject Scripting.FileSystemObject).GetFolder($fake).ShortPath
    if ($short -ne $fake) {
        $env:ChocolateyInstall = $short
        Check "detected when ChocolateyInstall is an 8.3 path ($short)" (Has (Agent) $installed)
    } else { Write-Host "skip 8.3 path check (no short names on this volume)" }
    $env:ChocolateyInstall = $fake

    $threw = $null
    try { Assert-CercanoPackageNotRunning -PackageId 'cercano-agent' -Executables @('cercano.exe') }
    catch { $threw = $_.Exception.Message }
    Check 'install backstop throws while running' ($null -ne $threw)
    Check 'message names the PID and says how to retry' ($threw -match "PID $($backup.Id)" -and $threw -match 'run the same choco command again')

    $warnings = $null
    Assert-CercanoPackageNotRunning -PackageId 'cercano-agent' -Executables @('cercano.exe') -WarnOnly -WarningVariable warnings -WarningAction SilentlyContinue
    Check 'BeforeModify mode warns instead of throwing' ($warnings.Count -eq 1)
    Check 'fixture processes were left running by the guard' (-not ($backup.HasExited -or $shim.HasExited))

    foreach ($p in $started) { if (-not $p.HasExited) { $p.Kill(); $p.WaitForExit() } }
    $found = Agent
    Check 'after exit: none of the fixtures reported' (-not ($started | Where-Object { Has $found $_ }))
    $started = @()

    # Injected process lists: deterministic, independent of the machine.
    $unknown = [pscustomobject]@{ ProcessId = 999001; Name = 'cercano.exe'; ExecutablePath = $null }
    $unrelated = [pscustomobject]@{ ProcessId = 999002; Name = 'weird.exe'; ExecutablePath = '\??\C:\' + ('x' * 300) + '\weird.exe' }
    $gone = [pscustomobject]@{ ProcessId = 999003; Name = 'cercano.exe'; ExecutablePath = (Join-Path $fake 'lib\cercano-agent\tools\missing\cercano.exe') }
    $found = @(Get-CercanoPackageProcess -PackageId 'cercano-agent' -Executables @('cercano.exe') -Processes @($unknown, $unrelated))
    Check 'same name with unreadable location fails closed' ($found.Count -eq 1 -and $found[0].ProcessId -eq 999001 -and $null -eq $found[0].Path)
    Check 'odd path of an unrelated process is ignored' (-not ($found | Where-Object ProcessId -eq 999002))
    $found = @(Get-CercanoPackageProcess -PackageId 'cercano-agent' -Executables @('cercano.exe') -Processes @($gone))
    Check 'same name with unresolvable path fails closed' ($found.Count -eq 1 -and $null -eq $found[0].Path)
    $found = @(Get-CercanoPackageProcess -PackageId 'cercano-agent' -Executables @('cercano.exe') -Processes @())
    Check 'empty process list: nothing reported' ($found.Count -eq 0)

    $env:ChocolateyInstall = $null
    $threw = $null
    try { Get-CercanoPackageProcess -PackageId 'cercano-agent' -Executables @('cercano.exe') | Out-Null }
    catch { $threw = $_.Exception.Message }
    Check 'missing ChocolateyInstall fails closed' ($threw -match 'ChocolateyInstall is not set')
}
finally {
    foreach ($p in $started) { if (-not $p.HasExited) { $p.Kill(); $p.WaitForExit() } }
    $env:ChocolateyInstall = $savedRoot
    if (Test-Path $junction) { (Get-Item $junction).Delete() }
    Remove-Item -Recurse -Force $base -ErrorAction SilentlyContinue
}

if ($failures) { Write-Host "$failures failed"; exit 1 }
Write-Host 'all guard checks passed'
