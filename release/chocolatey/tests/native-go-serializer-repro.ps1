param([string]$Run)
$ErrorActionPreference='Stop'
# Child-only self exit deadline: no external process termination.
Add-Type @'
using System; using System.Threading;
public static class Deadline { public static Timer timer; public static void Arm(){timer=new Timer(_=>Environment.Exit(124),null,7000,Timeout.Infinite);} }
'@
[Deadline]::Arm()
Write-Host 'BEGIN raw Get-Content extended properties reproduction'
$x=@(Get-Content "$Run\closed\native-go-install-events.jsonl")
Write-Host ('Extended properties: '+($x[0].PSObject.Properties.Name -join ','))
ConvertTo-Json -InputObject @([pscustomobject]@{eventsBeforeRelease=$x}) -Depth 12
Write-Host 'RAW COMPLETE'
