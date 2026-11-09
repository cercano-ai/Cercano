$ErrorActionPreference='Stop'
$r='C:\CercanoRunner\phase1\native-go-20261106201208'
$prefix='cercano-native-go-20261106201208-'
function SharedText([string]$p){$s=[IO.File]::Open($p,[IO.FileMode]::Open,[IO.FileAccess]::Read,([IO.FileShare]::ReadWrite -bor [IO.FileShare]::Delete)); try{$reader=New-Object IO.StreamReader($s); try{$reader.ReadToEnd()}finally{$reader.Dispose()}}finally{$s.Dispose()}}
$ps=@(Get-CimInstance Win32_Process | Where-Object {$_.ProcessId -ne $PID -and ($_.Name -like "$prefix*" -or $_.Name -eq 'choco.exe' -or $_.CommandLine -like '*native-go-20261106201208*')} | Select-Object ProcessId,ParentProcessId,Name,ExecutablePath,CommandLine,CreationDate)
$owned=@();foreach($case in 'closed','running','exe-locked','resource-locked'){$id=$prefix+$case;foreach($part in 'lib','lib-bkp','lib-bad'){$p="C:\ProgramData\chocolatey\$part\$id";if(Test-Path -LiteralPath $p){$owned+=@(Get-ChildItem -LiteralPath $p -Recurse -File -Force)}}; $p="C:\ProgramData\chocolatey\bin\$id.exe";if(Test-Path -LiteralPath $p){$owned+=Get-Item -LiteralPath $p}}
foreach($d in @(Get-ChildItem -LiteralPath 'C:\ProgramData\chocolatey\.chocolatey' -Directory -Force | Where-Object {$_.Name -like "$prefix*"})){$owned+=@(Get-ChildItem -LiteralPath $d.FullName -Recurse -File -Force)}
$hashes=@($owned | ForEach-Object {[pscustomobject]@{path=$_.FullName;length=$_.Length;sha256=(Get-FileHash -LiteralPath $_.FullName).Hash}})
$text=@(Get-ChildItem -LiteralPath $r -Recurse -File | Where-Object {$_.Extension -in '.json','.jsonl','.txt','.log','.ps1','.go','.csv','.nuspec'} | ForEach-Object {try{[pscustomobject]@{path=$_.FullName;length=$_.Length;lastWriteUtc=$_.LastWriteTimeUtc;content=(SharedText $_.FullName)}}catch{[pscustomobject]@{path=$_.FullName;error=$_.Exception.Message}}})
$metadata=@($owned | Where-Object {$_.Extension -in '.nuspec','.version','.files','.arguments','.json','.config'} | ForEach-Object {[pscustomobject]@{path=$_.FullName;content=(SharedText $_.FullName)}})
$feed=@(Get-ChildItem -LiteralPath "$r\feed" -File | Get-FileHash | Select-Object Path,Hash)
$sentinels=@(Get-ChildItem -LiteralPath "$r\native-go-synthetic-data" -File | Get-FileHash | Select-Object Path,Hash)
[pscustomobject]@{timestamp=(Get-Date).ToUniversalTime().ToString('o');processes=$ps;installedFiles=$hashes;packageMetadata=$metadata;evidence=$text;feedHashes=$feed;syntheticHashes=$sentinels} | ConvertTo-Json -Depth 18
