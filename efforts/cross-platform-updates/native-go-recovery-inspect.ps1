$ErrorActionPreference='Stop'
$r='C:\CercanoRunner\phase1\native-go-20261106201208'
function SharedText([string]$p){$s=[IO.File]::Open($p,[IO.FileMode]::Open,[IO.FileAccess]::Read,([IO.FileShare]::ReadWrite -bor [IO.FileShare]::Delete)); try{$reader=New-Object IO.StreamReader($s); try{$reader.ReadToEnd()}finally{$reader.Dispose()}}finally{$s.Dispose()}}
$ps=@(Get-CimInstance Win32_Process | Where-Object { $_.ProcessId -ne $PID -and ($_.Name -like 'cercano-native-go-20261106201208-*' -or $_.Name -eq 'choco.exe' -or ($_.CommandLine -like '*native-go-20261106201208*')) } | Select-Object ProcessId,ParentProcessId,Name,ExecutablePath,CommandLine,CreationDate)
$files=@(Get-ChildItem -LiteralPath $r -Recurse -File -Force | Select-Object FullName,Length,LastWriteTimeUtc)
$e=@(); foreach($n in 'native-go-status.txt','native-go-failure.txt','native-go-commands.json','native-go-results.json','native-go-transcript.txt'){ $p=Join-Path $r $n; if(Test-Path -LiteralPath $p){$e+= [pscustomobject]@{path=$p;content=(SharedText $p)}} }
[pscustomobject]@{timestamp=(Get-Date).ToUniversalTime().ToString('o');observerPid=$PID;processes=$ps;files=$files;evidence=$e} | ConvertTo-Json -Depth 16
