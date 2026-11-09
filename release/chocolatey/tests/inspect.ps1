$ErrorActionPreference='Stop'
Get-Content 'C:\CercanoRunner\phase1\transactions-01\closed-pack-1.0.0.log'
Get-Content 'C:\CercanoRunner\phase1\provision.log'
Get-FileHash 'C:\Windows\Temp\chocolatey\chocoInstall\chocolatey.zip'
Get-ExecutionPolicy -List | Format-Table
