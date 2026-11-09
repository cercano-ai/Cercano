# Regression for Windows PowerShell Start-Process losing ExitCode without a retained handle.
$ErrorActionPreference='Stop'
$exe='C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe'
$p=Start-Process $exe -ArgumentList @('-NoProfile','-NonInteractive','-Command','exit 0') -PassThru
$handle=$p.Handle
if(!$p.WaitForExit(5000)){throw 'Unexpected child timeout: no termination authorized'}
if($null -eq $p.ExitCode -or $p.ExitCode -ne 0){throw 'Retained handle did not preserve exact exit'}
[pscustomobject]@{exit=$p.ExitCode;retainedHandle=($handle -ne 0);pass=$true}|ConvertTo-Json
