param([string]$Run)
$ErrorActionPreference='Stop'
. "$PSScriptRoot\native-go-plain-json.ps1"
$clock=[Diagnostics.Stopwatch]::StartNew()
$events=@(Read-InstallEvents "$Run\closed\native-go-install-events.jsonl")
$json=ConvertTo-PlainJson @([pscustomobject]@{kind='held-operation';eventsBeforeRelease=$events})
$back=ConvertFrom-Json $json
if($back.eventsBeforeRelease.Count -ne 2 -or $back.eventsBeforeRelease[1].version -cne '2.0.0'){throw 'Plain events regression'}
# Decorated strings may arrive accidentally: flatten rather than traverse provider properties.
$raw=@(Get-Content "$Run\closed\native-go-install-events.jsonl")
$round=ConvertFrom-Json (ConvertTo-PlainJson @([pscustomobject]@{events=$raw}))
if($round.events[0] -cne [string]$raw[0]){throw 'Decorated string projection mismatch'}
$rejected=$false;try{ConvertTo-PlainJson (Get-Item $Run)|Out-Null}catch{$rejected=$true}
if(!$rejected){throw 'Provider object accepted'}
$cyclic=@{};$cyclic.self=$cyclic;$rejected=$false
try{ConvertTo-PlainJson $cyclic|Out-Null}catch{$rejected=$true}
if(!$rejected){throw 'Cycle not rejected'}
if($clock.ElapsedMilliseconds -gt 7000){throw 'Regression exceeded bound'}
[pscustomobject]@{pass=$true;elapsedMs=$clock.ElapsedMilliseconds;json=$json;providerRejected=$true;cycleRejected=$true}|ConvertTo-Json
