$ErrorActionPreference='Stop'
$ProgressPreference='SilentlyContinue'
$root='C:\Users\Public\pr84-20261113'
$env:HOME="$root\state\home"; $env:USERPROFILE=$env:HOME
$env:APPDATA="$root\state\appdata"; $env:LOCALAPPDATA="$root\state\localappdata"
$env:TEMP="$root\state\temp"; $env:TMP=$env:TEMP
$env:GOPATH="$root\tools\gopath"; $env:GOCACHE="$root\tools\gocache"
$env:GOTOOLCHAIN='local'; $env:CGO_ENABLED='0'
$env:PATH="$root\tools\go\bin;$env:PATH"
@($env:HOME,$env:APPDATA,$env:LOCALAPPDATA,$env:TEMP,$env:GOPATH,$env:GOCACHE,"$root\logs","$root\bin") | ForEach-Object {New-Item -ItemType Directory -Force $_ | Out-Null}
