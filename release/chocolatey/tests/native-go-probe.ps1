# Isolated synthetic native Go PE experiment, NOT a production package template.
param([Parameter(Mandatory=$true)][ValidatePattern('^C:\\CercanoRunner\\phase1\\native-go-[0-9]+$')][string]$Run)
$ErrorActionPreference='Stop'
. "$PSScriptRoot\native-go-plain-json.ps1"
$choco='C:\ProgramData\chocolatey\bin\choco.exe'
$feed="$Run\feed"; New-Item -ItemType Directory $feed | Out-Null
Start-Transcript "$Run\native-go-transcript.txt"
$records=New-Object Collections.ArrayList
$results=New-Object Collections.ArrayList
$known=@{}
$script:saveFailed=$false
function Save {
 if($script:saveFailed){throw 'Save already failed; no retry'}
 'SAVING' | Set-Content "$Run\native-go-status.txt"
 try {
  [IO.File]::WriteAllText("$Run\native-go-commands.json",(ConvertTo-PlainJson @($records)))
  [IO.File]::WriteAllText("$Run\native-go-results.json",(ConvertTo-PlainJson @($results)))
  'RUNNING' | Set-Content "$Run\native-go-status.txt"
 } catch {$script:saveFailed=$true; 'SAVE_FAILED'|Set-Content "$Run\native-go-status.txt";throw}
}
function Manager([string]$label,[string[]]$a) {
 $start=(Get-Date).ToUniversalTime().ToString('o')
 & $choco @a *> "$Run\$label.log"; $exit=$LASTEXITCODE
 [void]$records.Add([pscustomobject]@{label=$label;start=$start;end=(Get-Date).ToUniversalTime().ToString('o');executable=$choco;arguments=$a;exit=$exit;output=[IO.File]::ReadAllText("$Run\$label.log")}); Save
 Write-Host "MANAGER $label exit=$exit"; return $exit
}
function Snapshot([string]$id,[string]$label) {
 $files=@(); $roots=@("C:\ProgramData\chocolatey\lib\$id","C:\ProgramData\chocolatey\lib-bkp\$id","C:\ProgramData\chocolatey\lib-bad\$id")
 $roots+=@(Get-ChildItem 'C:\ProgramData\chocolatey\.chocolatey' -Directory -ErrorAction SilentlyContinue | Where-Object {$_.Name -like "$id.*"} | ForEach-Object {$_.FullName})
 $items=@(); foreach($root in $roots){if(Test-Path $root){$items+=@(Get-ChildItem -LiteralPath $root -Recurse -File -Force)}}
 $items+=@(Get-ChildItem 'C:\ProgramData\chocolatey\bin' -File | Where-Object {$_.Name -like "$id.*"})
 foreach($item in $items){$hash=(Get-FileHash -LiteralPath $item.FullName).Hash; $files+=[pscustomobject]@{path=$item.FullName;length=$item.Length;sha256=$hash;knownSourceMatches=@($known[$hash])}}
 $meta=@(& $choco list --limit-output); $metaExit=$LASTEXITCODE
 $shim="C:\ProgramData\chocolatey\bin\$id.exe"; $target="C:\ProgramData\chocolatey\lib\$id\tools\$id.exe"
 $targetEncoded=$false
 if(Test-Path $shim){$bytes=[IO.File]::ReadAllBytes($shim); $targetEncoded=([Text.Encoding]::Unicode.GetString($bytes).Contains($target) -or [Text.Encoding]::UTF8.GetString($bytes).Contains($target))}
 $s=[pscustomobject]@{label=$label;timestamp=(Get-Date).ToUniversalTime().ToString('o');files=$files;metadata=$meta;metadataExit=$metaExit;shim=$shim;expectedTarget=$target;shimContainsExpectedTarget=$targetEncoded}
 ConvertTo-PlainJson $s | Set-Content "$Run\$label-snapshot.json"; return $s
}
function Launch([string]$path,[string]$version,[string]$label){
 $id=[IO.Path]::GetFileNameWithoutExtension($path)
 $payload="C:\ProgramData\chocolatey\lib\$id\tools\$id.exe"
 if((Get-FileHash $payload).Hash -cne (Get-FileHash "$Run\native-go-$version.exe").Hash){throw 'Exact payload hash mismatch'}
 $resource=Join-Path (Split-Path $payload -Parent) 'resource.txt'
 if([IO.File]::ReadAllText($resource) -cne "resource-$version"){throw 'Exact resource bytes mismatch'}
 $output=@(& $path); $exit=$LASTEXITCODE; $expected="native-go-fixture version=$version resource=resource-$version"
 $pass=($exit -eq 0 -and $output.Count -eq 1 -and $output[0] -ceq $expected)
 [void]$results.Add([pscustomobject]@{kind='fresh-launch';label=$label;path=$path;exit=$exit;output=$output;expected=$expected;pass=$pass});Save
 if(!$pass){throw "Exact version/resource assertion failed: $label"}
}
function Identity($proc,[string]$exe){
 $proc.Refresh(); $alive=!$proc.HasExited
 $cim=Get-CimInstance Win32_Process -Filter "ProcessId=$($proc.Id)"
 [pscustomobject]@{pid=$proc.Id;alive=$alive;executable=$cim.ExecutablePath;creationDate=$cim.CreationDate.ToUniversalTime().ToString('o');exactPath=($alive -and $cim.ExecutablePath -ieq $exe)}
}
try {
 [pscustomobject]@{identity=[Security.Principal.WindowsIdentity]::GetCurrent().Name;os=[Environment]::OSVersion.ToString();policy=@(Get-ExecutionPolicy -List);chocoVersion=(& $choco --version)} | ConvertTo-Json -Depth 5 | Set-Content "$Run\native-go-environment.json"
 $data="$Run\native-go-synthetic-data"; New-Item -ItemType Directory $data | Out-Null
 foreach($name in 'config','conversation','fake-credential'){[IO.File]::WriteAllText("$data\$name.txt","synthetic-only-$name")}
 $before=@(Get-ChildItem $data -File | Get-FileHash | Select-Object Path,Hash); ConvertTo-Json -InputObject $before | Set-Content "$Run\native-go-data-before.json"
 foreach($case in 'closed','running','exe-locked','resource-locked') {
  $id='cercano-'+(Split-Path $Run -Leaf)+'-'+$case
  if((Test-Path "C:\ProgramData\chocolatey\lib\$id") -or (Test-Path "C:\ProgramData\chocolatey\bin\$id.exe")){throw 'Preexisting ownership boundary'}
  $control="$Run\$case"; New-Item -ItemType Directory $control | Out-Null
  foreach($version in '1.0.0','2.0.0'){
   $build="$control\$version"; New-Item -ItemType Directory "$build\tools" -Force | Out-Null
   Copy-Item "$Run\native-go-$version.exe" "$build\tools\$id.exe"
   [IO.File]::WriteAllText("$build\tools\resource.txt","resource-$version")
   $install=@'
$ErrorActionPreference='Stop'
$event=[pscustomobject]@{version='__VERSION__';time=(Get-Date).ToUniversalTime().ToString('o');pid=$PID;exeHash=(Get-FileHash -LiteralPath '__EXE__').Hash;resourceHash=(Get-FileHash -LiteralPath '__RESOURCE__').Hash}
$event | ConvertTo-Json -Compress | Add-Content -LiteralPath '__EVENTS__'
'@
   $install=$install.Replace('__VERSION__',$version).Replace('__EXE__',"C:\ProgramData\chocolatey\lib\$id\tools\$id.exe").Replace('__RESOURCE__',"C:\ProgramData\chocolatey\lib\$id\tools\resource.txt").Replace('__EVENTS__',"$control\native-go-install-events.jsonl")
   [IO.File]::WriteAllText("$build\tools\chocolateyInstall.ps1",$install)
   [IO.File]::WriteAllText("$build\$id.nuspec","<?xml version=`"1.0`"?><package><metadata><id>$id</id><version>$version</version><authors>Local synthetic fixture</authors><description>Isolated native Go locking experiment only</description></metadata><files><file src=`"tools\**`" target=`"tools`" /></files></package>")
   foreach($f in Get-ChildItem $build -Recurse -File){$h=(Get-FileHash $f.FullName).Hash; $known[$h]=@($known[$h])+@("$case/$version/"+$f.FullName.Substring($build.Length+1))}
   if((Manager "$case-pack-$version" @('pack',"$build\$id.nuspec","--outputdirectory=$feed",'--limit-output')) -ne 0){throw 'pack failed'}
  }
  ConvertTo-PlainJson $known | Set-Content "$Run\native-go-known-hashes.json"
  if((Manager "$case-install-A" @('install',$id,'--version=1.0.0',"--source=$feed",'-y','--no-progress','--limit-output','--execution-timeout=60')) -ne 0){throw 'install A failed'}
  $exe="C:\ProgramData\chocolatey\lib\$id\tools\$id.exe"; $shim="C:\ProgramData\chocolatey\bin\$id.exe"
  $a=Snapshot $id "$case-A"; Launch $exe '1.0.0' "$case-A-direct"; Launch $shim '1.0.0' "$case-A-shim"
  $proc=$null; $lock=$null; $identityBefore=$null; $identityAfter=$null
  try {
   if($case -eq 'running'){
    $proc=Start-Process -FilePath $exe -ArgumentList @('--hold',"$control\native-go-control") -PassThru -RedirectStandardOutput "$control\native-go-process.txt" -RedirectStandardError "$control\native-go-process-error.txt"
    # Retain the query handle while alive: Start-Process can otherwise report null ExitCode after exit.
    $processQueryHandle=$proc.Handle
    for($i=0;$i -lt 100 -and !(Test-Path "$control\native-go-control.ready");$i++){Start-Sleep -Milliseconds 100}
    if(!(Test-Path "$control\native-go-control.ready")){throw 'ready missing'}
    $ready=Get-Content "$control\native-go-control.ready" -Raw | ConvertFrom-Json
    $identityBefore=Identity $proc $exe
    if(!$identityBefore.exactPath -or $ready.pid -ne $proc.Id -or $ready.executable -ine $exe -or $ready.version -ne '1.0.0' -or $ready.resource -cne 'resource-1.0.0'){throw 'owned native process identity not established'}
   }
   if($case -in 'exe-locked','resource-locked'){
    $lockPath=if($case -eq 'exe-locked'){$exe}else{Split-Path $exe -Parent | Join-Path -ChildPath 'resource.txt'}
    $lock=[IO.File]::Open($lockPath,[IO.FileMode]::Open,[IO.FileAccess]::Read,[IO.FileShare]::Read)
   }
   $code=Manager "$case-upgrade-held" @('upgrade',$id,'--version=2.0.0',"--source=$feed",'-y','--no-progress','--limit-output','--execution-timeout=60','--debug','--verbose')
   if($proc){$identityAfter=Identity $proc $exe; if(!$identityAfter.exactPath -or $identityAfter.creationDate -cne $identityBefore.creationDate){throw 'Running A identity changed'}}
   $held=Snapshot $id "$case-before-release"
   [void]$results.Add([pscustomobject]@{kind='held-operation';case=$case;package=$id;exit=$code;identityBefore=$identityBefore;identityAfterBeforeRelease=$identityAfter;lockHeld=($null -ne $lock);lockPath=$lockPath;eventsBeforeRelease=@(Read-InstallEvents "$control\native-go-install-events.jsonl");expectedActionableFailure=($case -ne 'closed');nonzeroFailure=($code -ne 0) });Save
   if($case -eq 'closed'){if($code -ne 0){throw 'Closed B upgrade failed'};Launch $exe '2.0.0' 'closed-held-B-direct';Launch $shim '2.0.0' 'closed-held-B-shim'}
   if($case -in 'exe-locked','resource-locked'){if($code -eq 0){throw 'Lock failed to produce actionable manager failure'}}
  } finally {
   if($lock){$lock.Dispose()}
   if($proc){[IO.File]::WriteAllText("$control\native-go-control.stop",'cooperative owned teardown'); if(!$proc.WaitForExit(15000)){throw 'Safety boundary: owned process did not cooperate; no kill authorized'}; $stopOutput=[IO.File]::ReadAllText("$control\native-go-process.txt"); [void]$results.Add([pscustomobject]@{kind='cooperative-exit';case=$case;pid=$proc.Id;exit=$proc.ExitCode;output=$stopOutput});Save; if($proc.ExitCode -ne 0 -or !$stopOutput.Contains('cooperative-stop')){throw 'Cooperative exit assertion failed'}}
  }
  # Ordinary closed retry, no force, custom restore, or package version rewriting.
  $retry=Manager "$case-closed-retry" @('upgrade',$id,'--version=2.0.0',"--source=$feed",'-y','--no-progress','--limit-output','--execution-timeout=60')
  if($retry -ne 0){throw 'Closed ordinary manager retry failed; retain state'}
  $b=Snapshot $id "$case-B"; Launch $exe '2.0.0' "$case-B-direct"; Launch $shim '2.0.0' "$case-B-shim"
  & $shim --shimgen-log *> "$control\native-go-shim-target.log"
  if((Manager "$case-cleanup" @('uninstall',$id,'-y','--no-progress','--limit-output','--execution-timeout=60')) -ne 0){throw 'cleanup failed'}
  $clean=Snapshot $id "$case-cleaned"
  if((Test-Path $exe) -or (Test-Path $shim)){throw 'cleanup left payload/shim'}
 }
 $after=@(Get-ChildItem $data -File | Get-FileHash | Select-Object Path,Hash); ConvertTo-Json -InputObject $after | Set-Content "$Run\native-go-data-after.json"
 if(($before|ConvertTo-Json) -cne ($after|ConvertTo-Json)){throw 'Synthetic sentinel mutation'}
 Get-ChildItem $feed -File | Get-FileHash | ConvertTo-Json | Set-Content "$Run\native-go-feed-hashes.json"
 Save
 'COMPLETE' | Set-Content "$Run\native-go-status.txt"
} catch { $after=@(Get-ChildItem $data -File | Get-FileHash | Select-Object Path,Hash); ConvertTo-Json -InputObject $after | Set-Content "$Run\native-go-data-after-failure.json"; if(!$script:saveFailed){'FAILED'|Set-Content "$Run\native-go-status.txt"}; $_ | Format-List * -Force | Out-String | Set-Content "$Run\native-go-failure.txt"; throw }
finally {Stop-Transcript}
