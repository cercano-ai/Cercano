# Native x64 lifecycle test of the Cercano Chocolatey packages in a PRIVATE
# Chocolatey root (copy of choco under this folder). The real
# C:\ProgramData\chocolatey and the user's Cercano are never used: every choco
# call runs the private choco.exe with ChocolateyInstall pointing at the copy,
# only the local feed is a source, and every cercano process gets throwaway
# profile folders. Fixture processes are started and stopped by this script.
# Needs pwsh 7 and Chocolatey program files: the installed ones (only copied)
# or, with -ChocoSource, the tools\chocolateyInstall folder of an official
# chocolatey .nupkg whose SHA-256 you have checked.
# Feed: a folder with cercano, cercano-agent and cercano-cli .nupkg at 0.0.1
# and 0.0.2 (build-windows-release.py, render_packages.py, choco pack).
# Usage: pwsh -File private-root-lifecycle.ps1 -Feed <nupkg dir> -Work <new dir> [-ChocoSource <dir>]
# Results: <Work>\results.md, full choco output: <Work>\choco.log
#Requires -Version 7
param(
    [Parameter(Mandatory)] [string] $Feed,
    [Parameter(Mandatory)] [string] $Work,
    [string] $ChocoSource = 'C:\ProgramData\chocolatey',
    # Use the machine's real Chocolatey instead of a private copy: only on a
    # disposable or dedicated test machine, from an elevated shell, with no
    # cercano package installed. The script removes what it installs.
    [switch] $UseInstalledChocolatey
)
$ErrorActionPreference = 'Stop'

$profileDir = Join-Path $Work 'profile'
$log = Join-Path $Work 'results.md'
if ($UseInstalledChocolatey) {
    $root = [Environment]::GetEnvironmentVariable('ChocolateyInstall', 'Machine')
    if (-not $root) { throw 'Chocolatey is not installed (no machine ChocolateyInstall).' }
    $admin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
    if (-not $admin) { throw 'Run from an elevated shell with -UseInstalledChocolatey.' }
    if (Get-ChildItem (Join-Path $root 'lib') -Filter 'cercano*' -ErrorAction SilentlyContinue) {
        throw "A cercano package is already installed in $root; refusing to touch it."
    }
    New-Item -ItemType Directory -Force $profileDir | Out-Null
} else {
    $root = Join-Path $Work 'choco root é'          # spaces + non-ASCII on purpose
    if (Test-Path $root) { throw "work root exists: $root" }
    New-Item -ItemType Directory -Force $root, $profileDir | Out-Null
    foreach ($item in 'choco.exe', 'choco.exe.manifest', 'helpers', 'tools', 'extensions', 'config', 'redirects', 'LICENSE.txt', 'CREDITS.txt') {
        $src = Join-Path $ChocoSource $item
        if (Test-Path $src) { Copy-Item -Recurse $src (Join-Path $root $item) }
    }
}
$env:ChocolateyInstall = $root
$choco = Join-Path $root 'choco.exe'

# Synthetic user data that must survive everything.
$sentinel = Join-Path $profileDir '.config\cercano\sentinel.txt'
New-Item -ItemType Directory -Force (Split-Path $sentinel) | Out-Null
Set-Content $sentinel "synthetic user data $(Get-Date -Format o)"
$sentinelHash = (Get-FileHash $sentinel).Hash

$results = [System.Collections.Generic.List[string]]::new()
$fail = 0
function Note([string] $text) { $script:results.Add($text); Write-Host $text }
function Check([string] $name, [bool] $ok, [string] $detail = '') {
    if (-not $ok) { $script:fail++ }
    Note ("- {0} {1}{2}" -f ($(if ($ok) { 'PASS' } else { 'FAIL' })), $name, $(if ($detail) { ": $detail" } else { '' }))
}
function Choco([string[]] $arguments) {
    $out = & $choco @arguments --source "$Feed" -y --no-progress 2>&1 | Out-String
    Add-Content -Encoding utf8 (Join-Path $Work 'choco.log') ("`n===== choco $($arguments -join ' ') (exit $LASTEXITCODE)`n" + $out)
    return [pscustomobject]@{ Exit = $LASTEXITCODE; Out = $out }
}
function Version([string] $exe) {
    $path = Join-Path (Join-Path $root 'bin') $exe
    if (-not (Test-Path $path)) { return '<no shim>' }
    $p = New-FixtureStartInfo $path @('--version')
    $p.RedirectStandardOutput = $true
    $proc = [Diagnostics.Process]::Start($p); $text = $proc.StandardOutput.ReadToEnd(); $proc.WaitForExit()
    return $text.Trim()
}
function New-FixtureStartInfo([string] $path, [string[]] $arguments) {
    $si = [Diagnostics.ProcessStartInfo]::new($path)
    foreach ($a in $arguments) { $si.ArgumentList.Add($a) }
    $si.UseShellExecute = $false
    foreach ($name in 'USERPROFILE', 'HOME', 'APPDATA', 'LOCALAPPDATA') { $si.Environment[$name] = $profileDir }
    $si.Environment['TEMP'] = $profileDir; $si.Environment['TMP'] = $profileDir
    return $si
}
$fixtures = @()
function Start-Agent([string] $path) {
    # `cercano acp` waits on stdin; closing stdin is the cooperative stop.
    $si = New-FixtureStartInfo $path @('acp')
    $si.RedirectStandardInput = $true; $si.RedirectStandardError = $true; $si.RedirectStandardOutput = $true
    $p = [Diagnostics.Process]::Start($si)
    $script:fixtures += $p
    Start-Sleep -Seconds 2
    return $p
}
function Stop-Agent($p) {
    $p.StandardInput.Close()
    if (-not $p.WaitForExit(10000)) { $p.Kill($true); $p.WaitForExit(); return 'killed (fixture did not exit on stdin close)' }
    return "exited $($p.ExitCode)"
}
function Installed([string] $id) {
    $nuspec = Join-Path $root "lib\$id\$id.nuspec"
    if (-not (Test-Path $nuspec)) { return '<none>' }
    return ([xml](Get-Content -Raw $nuspec)).package.metadata.version
}

try {
    Note "# Chocolatey lifecycle, native x64"
    Note ""
    Note "Run $(Get-Date -Format o). Windows $([Environment]::OSVersion.Version), PowerShell $($PSVersionTable.PSVersion), choco $(& $choco --version). Private root: ``$root``. Elevated: $(([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator))."
    Note ""

    Note "## Install and closed upgrade"
    $r = Choco @('install', 'cercano', '--version', '0.0.1')
    Check 'convenience install 0.0.1' ($r.Exit -eq 0) "exit $($r.Exit)"
    Note "- Finding: ``install cercano --version 0.0.1`` resolved components to agent $(Installed 'cercano-agent'), cli $(Installed 'cercano-cli') (feed has 0.0.1 and 0.0.2)"
    $r = Choco @('uninstall', 'cercano', 'cercano-agent', 'cercano-cli')
    Check 'clean uninstall of the set' ($r.Exit -eq 0 -and (Installed 'cercano-agent') -eq '<none>') "exit $($r.Exit)"
    $r = Choco @('install', 'cercano-agent', 'cercano-cli', '--version', '0.0.1')
    $r = Choco @('install', 'cercano', '--version', '0.0.1')
    Check 'components pinned at 0.0.1 under cercano 0.0.1' ((Installed 'cercano-agent') -eq '0.0.1' -and (Installed 'cercano-cli') -eq '0.0.1' -and (Installed 'cercano') -eq '0.0.1') "agent $(Installed 'cercano-agent'), cli $(Installed 'cercano-cli')"
    Check 'shims report 0.0.1' ((Version 'cercano.exe') -eq 'cercano v0.0.1' -and (Version 'cercano-cli.exe') -eq 'cercano-cli v0.0.1') "$(Version 'cercano.exe') / $(Version 'cercano-cli.exe')"
    $shims = @(Get-ChildItem (Join-Path $root 'bin') -Filter 'cercano*.exe' | ForEach-Object Name | Sort-Object)
    Check 'exactly two cercano shims' (($shims -join ',') -eq 'cercano-cli.exe,cercano.exe') ($shims -join ', ')

    $r = Choco @('upgrade', 'cercano', '--version', '0.0.2')
    Check 'closed upgrade to 0.0.2' ($r.Exit -eq 0 -and (Installed 'cercano-agent') -eq '0.0.2' -and (Installed 'cercano-cli') -eq '0.0.2') "exit $($r.Exit), agent $(Installed 'cercano-agent'), cli $(Installed 'cercano-cli')"
    Check 'shims report 0.0.2' ((Version 'cercano.exe') -eq 'cercano v0.0.2' -and (Version 'cercano-cli.exe') -eq 'cercano-cli v0.0.2')

    Note ""
    Note "## Running agent (started through the shim)"
    $r = Choco @('uninstall', 'cercano', 'cercano-agent', 'cercano-cli')
    $r = Choco @('install', 'cercano-agent', 'cercano-cli', '--version', '0.0.1')
    $agent = Start-Agent (Join-Path $root 'bin\cercano.exe')
    $children = @(Get-CimInstance Win32_Process -Filter "ParentProcessId=$($agent.Id)")
    Note "- Shim PID $($agent.Id) children: $(($children | ForEach-Object { "$($_.ProcessId) $($_.ExecutablePath)" }) -join '; ')"
    $r = Choco @('upgrade', 'cercano-agent', '--version', '0.0.2')
    Check 'agent upgrade refused while running' ($r.Exit -ne 0) "exit $($r.Exit)"
    Check 'refusal message names the process' ($r.Out -match 'cannot be changed while these processes are running')
    Check 'rolled back to 0.0.1' ((Installed 'cercano-agent') -eq '0.0.1') "lib version $(Installed 'cercano-agent')"
    Check 'running agent untouched' (-not $agent.HasExited)
    $r2 = Choco @('upgrade', 'cercano-cli', '--version', '0.0.2')
    Check 'CLI upgrade allowed while agent runs' ($r2.Exit -eq 0 -and (Installed 'cercano-cli') -eq '0.0.2') "exit $($r2.Exit)"
    Note "- Agent stop: $(Stop-Agent $agent)"
    $r = Choco @('upgrade', 'cercano-agent', '--version', '0.0.2')
    Check 'ordinary retry after exit succeeds' ($r.Exit -eq 0 -and (Version 'cercano.exe') -eq 'cercano v0.0.2') "exit $($r.Exit), $(Version 'cercano.exe')"

    Note ""
    Note "## Running agent started from lib directly"
    $r = Choco @('uninstall', 'cercano-agent')
    $r = Choco @('install', 'cercano-agent', '--version', '0.0.1')
    $agent = Start-Agent (Join-Path $root 'lib\cercano-agent\tools\cercano.exe')
    $r = Choco @('upgrade', 'cercano-agent', '--version', '0.0.2')
    Check 'refused (direct launch)' ($r.Exit -ne 0 -and (Installed 'cercano-agent') -eq '0.0.1') "exit $($r.Exit), lib $(Installed 'cercano-agent')"
    Note "- Agent stop: $(Stop-Agent $agent)"

    Note ""
    Note "## Locked payload (read handle, share-read only)"
    $exe = Join-Path $root 'lib\cercano-agent\tools\cercano.exe'
    $lock = [IO.File]::Open($exe, 'Open', 'Read', 'Read')
    $r = Choco @('upgrade', 'cercano-agent', '--version', '0.0.2')
    Check 'locked upgrade fails' ($r.Exit -ne 0) "exit $($r.Exit)"
    Note "- Installed after locked attempt: $(Installed 'cercano-agent'); shim says $(Version 'cercano.exe')"
    $lock.Dispose()
    $r = Choco @('upgrade', 'cercano-agent', '--version', '0.0.2')
    Check 'retry after unlock succeeds' ($r.Exit -eq 0 -and (Version 'cercano.exe') -eq 'cercano v0.0.2') "exit $($r.Exit), $(Version 'cercano.exe')"

    Note ""
    Note "## Uninstall while running"
    foreach ($via in 'bin\cercano.exe', 'lib\cercano-agent\tools\cercano.exe') {
        $agent = Start-Agent (Join-Path $root $via)
        $r = Choco @('uninstall', 'cercano-agent')
        Check "uninstall refused while running ($via)" ($r.Exit -ne 0 -and (Installed 'cercano-agent') -eq '0.0.2' -and -not $agent.HasExited) "exit $($r.Exit), lib $(Installed 'cercano-agent'), shim $(Test-Path (Join-Path $root 'bin\cercano.exe'))"
        Note "- Agent stop: $(Stop-Agent $agent)"
        $r = Choco @('uninstall', 'cercano-agent')
        Check 'uninstall retry after exit succeeds' ($r.Exit -eq 0 -and (Installed 'cercano-agent') -eq '<none>' -and -not (Test-Path (Join-Path $root 'lib-bkp\cercano-agent\tools\cercano.exe'))) "exit $($r.Exit)"
        $r = Choco @('install', 'cercano-agent', '--version', '0.0.2')
    }
    $r = Choco @('uninstall', 'cercano-agent')

    Note ""
    Note "## CLI only (remote agent case) and final uninstall"
    $r = Choco @('uninstall', 'cercano', 'cercano-cli')
    $r = Choco @('install', 'cercano-cli', '--version', '0.0.2')
    Check 'cli alone installs without the agent' ($r.Exit -eq 0 -and (Installed 'cercano-agent') -eq '<none>' -and -not (Test-Path (Join-Path $root 'bin\cercano.exe'))) "exit $($r.Exit)"
    $r = Choco @('uninstall', 'cercano-cli')
    $left = @(Get-ChildItem (Join-Path $root 'bin') -Filter 'cercano*' -ErrorAction SilentlyContinue)
    Check 'uninstall removes package and shims' ($r.Exit -eq 0 -and (Installed 'cercano-cli') -eq '<none>' -and $left.Count -eq 0) "exit $($r.Exit), leftover shims $($left.Count)"
    Check 'synthetic user data unchanged' ((Get-FileHash $sentinel).Hash -eq $sentinelHash)
}
finally {
    foreach ($p in $fixtures) { if (-not $p.HasExited) { $p.Kill($true); $p.WaitForExit(); Note "- cleanup: killed leftover fixture $($p.Id)" } }
    Note ""
    Note "**$fail failed.**"
    $results | Set-Content -Encoding utf8 $log
}
exit [int]($fail -gt 0)
