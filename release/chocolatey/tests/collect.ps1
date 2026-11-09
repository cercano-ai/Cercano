# Read only fixture-owned evidence, never inherited profiles.
param([ValidatePattern('^[a-zA-Z0-9-]+$')][string]$RunName = 'transactions-02')
$ErrorActionPreference = 'Stop'
$run = "C:\CercanoRunner\phase1\$RunName"
$evidence = [ordered]@{}
Get-ChildItem $run -File -Recurse | Where-Object { $_.Extension -in @('.json','.log','.csv') -or $_.Name -like '*-metadata.txt' -or $_.Name -in @('hooks.txt','process.txt') } | ForEach-Object {
  $evidence[$_.FullName.Substring($run.Length+1)] = [IO.File]::ReadAllText($_.FullName)
}
$evidence | ConvertTo-Json -Depth 8
