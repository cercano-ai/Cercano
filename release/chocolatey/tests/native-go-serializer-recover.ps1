# One-time, explicitly authorized harness cleanup; never a package policy.
$ErrorActionPreference='Stop'
$run='C:\CercanoRunner\phase1\native-go-20261106201208'
Add-Type @'
using System;
using System.Runtime.InteropServices;
public static class OwnedHarnessHandle {
 [DllImport("kernel32.dll",SetLastError=true)] public static extern IntPtr OpenProcess(uint access,bool inherit,uint pid);
 [DllImport("kernel32.dll",SetLastError=true)] public static extern bool GetProcessTimes(IntPtr h,out long creation,out long exit,out long kernel,out long user);
 [DllImport("kernel32.dll",SetLastError=true)] public static extern bool TerminateProcess(IntPtr h,uint code);
 [DllImport("kernel32.dll")] public static extern bool CloseHandle(IntPtr h);
 [DllImport("kernel32.dll")] public static extern uint WaitForSingleObject(IntPtr h,uint ms);
}
'@
$h=[OwnedHarnessHandle]::OpenProcess(0x100001 -bor 0x1000,$false,7440)
if($h -eq [IntPtr]::Zero){throw 'Cannot acquire original process handle; no termination'}
try {
 [long]$c=0;[long]$e=0;[long]$k=0;[long]$u=0
 if(![OwnedHarnessHandle]::GetProcessTimes($h,[ref]$c,[ref]$e,[ref]$k,[ref]$u)){throw 'GetProcessTimes failed'}
 $p=Get-CimInstance Win32_Process -Filter 'ProcessId=7440'
 $expected='C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass -File C:\CercanoRunner\phase1\native-go-20261106201208\native-go-probe.ps1 -Run C:\CercanoRunner\phase1\native-go-20261106201208'
 $all=@(Get-CimInstance Win32_Process)
 $active=@($all | Where-Object {$_.Name -ieq 'choco.exe' -or $_.ExecutablePath -like '*cercano-native-go-*' -or ($_.Name -like 'native-go-*.exe')})
 $creation=[DateTime]::FromFileTimeUtc($c)
 $evidence=[pscustomobject]@{creation=$creation.ToString('o');process=($p|Select-Object ProcessId,ParentProcessId,ExecutablePath,CommandLine,CreationDate);active=@($active|Select-Object ProcessId,ExecutablePath,CommandLine)}
 $evidence|ConvertTo-Json -Depth 4|Set-Content "$run\serializer-pre-stop.json"
 # Read full original transcript before stopping, sharing with its writer.
 function ReadShared($path){$s=[IO.File]::Open($path,'Open','Read',([IO.FileShare]::ReadWrite -bor [IO.FileShare]::Delete));try{$r=New-Object IO.StreamReader($s);try{$r.ReadToEnd()}finally{$r.Dispose()}}finally{$s.Dispose()}}
 [IO.File]::WriteAllText("$run\serializer-pre-stop-transcript.txt",(ReadShared "$run\native-go-transcript.txt"))
 if($creation.ToString('yyyy-MM-ddTHH:mm:ss.fffZ') -cne '2026-11-06T20:12:13.535Z' -or $p.ExecutablePath -ine 'C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe' -or $p.CommandLine -cne $expected -or $active.Count -ne 0){$evidence|ConvertTo-Json -Depth 4;throw 'Identity/ownership differs: no termination'}
 if(![OwnedHarnessHandle]::TerminateProcess($h,91)){throw 'Handle termination failed'}
 if([OwnedHarnessHandle]::WaitForSingleObject($h,5000) -ne 0){throw 'Termination wait failed'}
 [IO.File]::WriteAllText("$run\serializer-final-transcript.txt",(ReadShared "$run\native-go-transcript.txt"))
 'Exact original harness terminated by retained verified OS handle; exit requested 91'|Set-Content "$run\serializer-cleanup-status.txt"
 $evidence|ConvertTo-Json -Depth 4
} finally {[void][OwnedHarnessHandle]::CloseHandle($h)}
