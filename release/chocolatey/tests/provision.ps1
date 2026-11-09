$ErrorActionPreference = 'Stop'
$root = 'C:\CercanoRunner\phase1'
Start-Transcript "$root\provision.log" -Force
Get-ExecutionPolicy -List | Format-Table
[Security.Principal.WindowsIdentity]::GetCurrent().Name
Get-CimInstance Win32_OperatingSystem | Select-Object Caption,Version,BuildNumber,OSArchitecture | Format-List
"PROCESSOR_ARCHITECTURE=$env:PROCESSOR_ARCHITECTURE"
# Inspect only known bootstrap-owned locations before deciding whether to resume.
foreach ($p in @('C:\ProgramData\chocolatey','C:\SANITIZED_GUEST_TEMP\chocolatey\chocoInstall','C:\CercanoRunner\provision-20261106-1117')) {
  "INSPECT $p"; if (Test-Path $p) { Get-ChildItem $p | Select-Object Name,Length,Mode | Format-Table }
}
$script = 'C:\CercanoRunner\provision-20261106-1117\install-chocolatey.ps1'
$pkg = 'C:\SANITIZED_GUEST_TEMP\chocolatey\chocoInstall\chocolatey.zip'
Get-FileHash $script -Algorithm SHA256 | Format-List
if ((Get-FileHash $script).Hash -ne '44E045ED5350758616D664C5AF631E7F2CD10165F5BF2BD82CBF3A0BB8F63462') { throw 'Bootstrap changed' }
if (Test-Path 'C:\ProgramData\chocolatey\bin\choco.exe') { throw 'Unexpected pre-existing installation: inspect first' }
if (Test-Path 'C:\ProgramData\chocolatey') { throw 'Partial destination exists: inspect before removal' }
# Official previously downloaded installer; only this process has Bypass.
& $script
if (!(Test-Path 'C:\ProgramData\chocolatey\bin\choco.exe')) { throw 'Installation absent' }
& 'C:\ProgramData\chocolatey\bin\choco.exe' --version
Get-FileHash 'C:\ProgramData\chocolatey\bin\choco.exe' | Format-List
Get-ExecutionPolicy -List | Format-Table
Stop-Transcript
