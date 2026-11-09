# Read-only inspection after bounded transport timeout. No cleanup while earlier runner lives.
$ErrorActionPreference='Stop'
$run='C:\CercanoRunner\phase1\native-go-20261106201208'
$id='cercano-native-go-20261106201208-closed'
$choco='C:\ProgramData\chocolatey\bin\choco.exe'
$files=@(Get-ChildItem "C:\ProgramData\chocolatey\lib\$id" -Recurse -File -Force | ForEach-Object {[pscustomobject]@{path=$_.FullName;sha256=(Get-FileHash -LiteralPath $_.FullName).Hash;length=$_.Length}})
$exe="C:\ProgramData\chocolatey\lib\$id\tools\$id.exe"
$b=[IO.File]::ReadAllBytes($exe); $pe=[BitConverter]::ToInt32($b,60);$machine=[BitConverter]::ToUInt16($b,$pe+4)
$processes=@(Get-CimInstance Win32_Process | Where-Object {$_.Name -like 'cercano-native-go-*' -or ($_.Name -eq 'powershell.exe' -and $_.CommandLine -like '*native-go-20261106201208*')} | Select-Object ProcessId,ParentProcessId,Name,ExecutablePath,CommandLine,CreationDate)
$inspection=[pscustomobject]@{timestamp=(Get-Date).ToUniversalTime().ToString('o');processes=$processes;metadata=@(& $choco list --limit-output);files=$files;machineHex=('0x{0:X4}' -f $machine);policy=@(Get-ExecutionPolicy -List);syntheticData=@(Get-ChildItem "$run\native-go-synthetic-data" -File | Get-FileHash | Select-Object Path,Hash);feed=@(Get-ChildItem "$run\feed" -File | Get-FileHash | Select-Object Path,Hash);stopReason='Prior runner remains present after bounded transport timeout; no process termination or concurrent manager mutation authorized; retain fixture state.'}
$inspection | ConvertTo-Json -Depth 12
