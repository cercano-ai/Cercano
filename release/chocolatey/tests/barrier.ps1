# Harmless package-manager ordering probes, NOT production package templates.
$ErrorActionPreference = 'Stop'
$root = 'C:\CercanoRunner\phase1'
$run = "$root\transactions-04"
if (Test-Path $run) { throw 'Fresh run directory required; do not overwrite evidence' }
New-Item -ItemType Directory $run | Out-Null
$feed = "$run\feed"; New-Item -ItemType Directory $feed | Out-Null
$choco = 'C:\ProgramData\chocolatey\bin\choco.exe'
Start-Transcript "$run\transcript.txt"
Get-ExecutionPolicy -List | Format-Table
[Security.Principal.WindowsIdentity]::GetCurrent().Name
& $choco --version
# Sentinels are synthetic and outside package ownership; no inherited profile access.
$data = "$run\synthetic user data 空間"; New-Item -ItemType Directory $data | Out-Null
'config','conversation','fake-credential' | ForEach-Object { Set-Content -LiteralPath "$data\$_.txt" -Value "synthetic-only-$_" }
Get-ChildItem $data | Get-FileHash | Export-Csv "$run\data-before.csv" -NoTypeInformation
$csc = 'C:\Windows\Microsoft.NET\Framework64\v4.0.30319\csc.exe'
if (!(Test-Path $csc)) { $csc = 'C:\Windows\Microsoft.NET\Framework\v4.0.30319\csc.exe' }
"COMPILER $csc"
$records = New-Object System.Collections.ArrayList
function Choco([string]$label,[string[]]$arguments) {
  Write-Host "COMMAND $choco $($arguments -join ' ')"
  & $choco @arguments *> "$run\$label.log"
  $code = $LASTEXITCODE
  Write-Host "EXIT $label $code"
  [void]$records.Add([pscustomobject]@{label=$label;arguments=$arguments;exit=$code})
  return $code
}
function Snapshot([string]$id,[string]$label) {
  $paths = @("C:\ProgramData\chocolatey\lib\$id", "C:\ProgramData\chocolatey\lib-bad\$id", "C:\ProgramData\chocolatey\lib-bkp\$id")
  $items = @()
  foreach ($p in $paths) { if (Test-Path $p) { $items += Get-ChildItem -LiteralPath $p -File -Recurse | ForEach-Object { [pscustomobject]@{path=$_.FullName;length=$_.Length;sha256=(Get-FileHash -LiteralPath $_.FullName).Hash} } } }
  $shim = "C:\ProgramData\chocolatey\bin\$id.exe"
  if (Test-Path $shim) { $items += [pscustomobject]@{path=$shim;length=(Get-Item $shim).Length;sha256=(Get-FileHash $shim).Hash} }
  ConvertTo-Json -InputObject @($items) -Depth 5 | Set-Content "$run\$label-files.json"
  & $choco list --limit-output *> "$run\$label-metadata.txt"
  "SNAPSHOT $label shim=$(Test-Path $shim) payload=$(Test-Path "C:\ProgramData\chocolatey\lib\$id\tools\$id.exe") resource=$(Test-Path "C:\ProgramData\chocolatey\lib\$id\tools\resource.txt")"
}
$cases = @('barrier')
foreach ($case in $cases) {
  $id = "cercano-phase1-$case"
  $control = "$run\$case"; New-Item -ItemType Directory $control | Out-Null
  foreach ($version in @('1.0.0','2.0.0')) {
    $build = "$control\$version"; New-Item -ItemType Directory "$build\tools" -Force | Out-Null
    $cs = @"
using System; using System.IO; using System.Threading;
public class Fixture { public static void Main(string[] a) {
Console.WriteLine("$id $version " + System.Runtime.InteropServices.RuntimeInformation.ProcessArchitecture);
if(a.Length>0) { File.WriteAllText(a[0]+".ready","ready"); for(int i=0;i<1200 && !File.Exists(a[0]);i++) Thread.Sleep(100); }
} }
"@
    # RuntimeInformation is not a .NET Framework base reference; architecture recorded separately.
    $cs = $cs.Replace(' + System.Runtime.InteropServices.RuntimeInformation.ProcessArchitecture',' + (IntPtr.Size == 8 ? "CLR64" : "CLR32")')
    Set-Content "$build\fixture.cs" $cs
    & $csc /nologo /target:exe /platform:anycpu "/out:$build\tools\$id.exe" "$build\fixture.cs"
    if ($LASTEXITCODE -ne 0) { throw 'Fixture compilation failed' }
    Set-Content "$build\tools\resource.txt" "resource-$version"
    Set-Content "$build\tools\removable.txt" "removable-$version"
    $before = @'
$control = '__CONTROL__'
Add-Content "$control\hooks.txt" "before:$((Get-Date).ToUniversalTime().ToString('o'))"
$mode = Get-Content "$control\mode.txt"
if ($mode -eq 'barrier') {
  $id = Split-Path $control -Leaf
  $exe = Join-Path $PSScriptRoot "cercano-phase1-$id.exe"
  $p = Start-Process -FilePath $exe -ArgumentList "$control\stop" -PassThru -RedirectStandardOutput "$control\process.txt"
  Set-Content "$control\owned-pid.txt" $p.Id
  for($i=0; $i -lt 100 -and !(Test-Path "$control\stop.ready"); $i++) { Start-Sleep -Milliseconds 100 }
  if (!(Test-Path "$control\stop.ready")) { throw 'Owned barrier launch not ready' }
  Add-Content "$control\hooks.txt" "barrier-ready:$($p.Id)"
}
if ($mode -eq 'beforethrow') { throw 'SYNTHETIC: exit fixture before replacement' }
if ($mode -eq 'beforenonzero') { Set-PowerShellExitCode 19; exit 19 }
'@
    Set-Content "$build\tools\chocolateyBeforeModify.ps1" ($before.Replace('__CONTROL__',$control))
    $install = @'
$control = '__CONTROL__'
Add-Content "$control\hooks.txt" 'install:__VERSION__'
if ((Get-Content "$control\mode.txt") -eq 'installfail') { throw 'SYNTHETIC install failure' }
'@
    Set-Content "$build\tools\chocolateyInstall.ps1" ($install.Replace('__CONTROL__',$control).Replace('__VERSION__',$version))
    $uninstall = @'
$control = '__CONTROL__'
Add-Content "$control\hooks.txt" ('uninstall:shim=' + (Test-Path 'C:\ProgramData\chocolatey\bin\__ID__.exe'))
if ((Get-Content "$control\mode.txt") -eq 'uninstallthrow') { throw 'SYNTHETIC uninstall refusal' }
'@
    Set-Content "$build\tools\chocolateyUninstall.ps1" ($uninstall.Replace('__CONTROL__',$control).Replace('__ID__',$id))
    $nuspec = "<?xml version=`"1.0`"?><package><metadata><id>$id</id><version>$version</version><authors>Local fixture</authors><description>Disposable local transaction fixture only</description></metadata><files><file src=`"tools\**`" target=`"tools`" /></files></package>"
    Set-Content "$build\$id.nuspec" $nuspec
    $pack = Choco "$case-pack-$version" @('pack',"$build\$id.nuspec","--outputdirectory=$feed",'--limit-output')
    if ($pack -ne 0) { throw 'Pack failed' }
  }
  Set-Content "$control\mode.txt" 'normal'
  $code = Choco "$case-install-A" @('install',$id,'--version=1.0.0',"--source=$feed",'-y','--no-progress','--limit-output')
  if ($code -ne 0) { throw 'Baseline installation failed' }
  Snapshot $id "$case-before"
  $exe = "C:\ProgramData\chocolatey\lib\$id\tools\$id.exe"
  & $exe
  $proc = $null; $lock = $null
  if ($case -eq 'running') {
    $proc = Start-Process -FilePath $exe -ArgumentList "$control\stop" -PassThru -RedirectStandardOutput "$control\process.txt"
    for($i=0; $i -lt 100 -and !(Test-Path "$control\stop.ready"); $i++) { Start-Sleep -Milliseconds 100 }
    if (!(Test-Path "$control\stop.ready")) { throw 'Owned fixture did not become ready' }
    "OWNED PID $($proc.Id) alive=$(!$proc.HasExited)"
  }
  if ($case -in @('locked','lockedthrow')) { $lock = [IO.File]::Open("C:\ProgramData\chocolatey\lib\$id\tools\resource.txt",[IO.FileMode]::Open,[IO.FileAccess]::Read,[IO.FileShare]::Read) }
  if ($case -eq 'shimlocked') { $lock = [IO.File]::Open("C:\ProgramData\chocolatey\bin\$id.exe",[IO.FileMode]::Open,[IO.FileAccess]::Read,[IO.FileShare]::Read) }
  Set-Content "$control\mode.txt" $(if ($case -eq 'lockedthrow') { 'beforethrow' } else { $case })
  try {
    if ($case -eq 'uninstallthrow') { $code = Choco "$case-operation" @('uninstall',$id,'-y','--no-progress','--limit-output','--execution-timeout=60') }
    else { $code = Choco "$case-operation" @('upgrade',$id,'--version=2.0.0',"--source=$feed",'-y','--no-progress','--limit-output','--execution-timeout=60') }
    if ($case -eq 'barrier') { $proc = Get-Process -Id ([int](Get-Content "$control\owned-pid.txt")); if ($proc.Path -ne $exe) { throw 'Owned process path mismatch' } }
    Snapshot $id "$case-after"
    if ($proc) { "OWNED PID after $($proc.Id) alive=$(!$proc.HasExited)" }
    if (Test-Path $exe) { & $exe }
    Get-Content "$control\hooks.txt"
  } finally {
    if ($lock) { $lock.Dispose() }
    if ($proc) { Set-Content "$control\stop" 'graceful-exit'; if (!$proc.WaitForExit(15000)) { throw 'Owned process did not exit; do not kill' } }
  }
  Set-Content "$control\mode.txt" 'normal'
  if ($case -ne 'uninstallthrow' -and $code -ne 0) {
    # Retry only through manager after releasing synthetic lock; inspect rather than custom restore.
    $retry = Choco "$case-retry" @('upgrade',$id,'--version=2.0.0',"--source=$feed",'-y','--no-progress','--limit-output','--execution-timeout=60')
    Snapshot $id "$case-retry"
  }
  $cleanup = Choco "$case-cleanup" @('uninstall',$id,'-y','--no-progress','--limit-output','--execution-timeout=60')
  Snapshot $id "$case-cleaned"
}
Get-ChildItem $data | Get-FileHash | Export-Csv "$run\data-after.csv" -NoTypeInformation
if ((Get-Content "$run\data-before.csv" -Raw) -ne (Get-Content "$run\data-after.csv" -Raw)) { throw 'Synthetic user data changed' }
$records | ConvertTo-Json -Depth 6 | Set-Content "$run\commands.json"
Get-ChildItem $feed | Get-FileHash | Format-List
Get-ExecutionPolicy -List | Format-Table
Stop-Transcript
