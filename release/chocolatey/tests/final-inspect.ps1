$ErrorActionPreference = 'Stop'
Add-Type @'
using System; using System.Runtime.InteropServices;
public class NativeArch {
 [DllImport("kernel32.dll", SetLastError=true)] public static extern bool IsWow64Process2(IntPtr p, out ushort process, out ushort native);
}
'@
function Arch($p) { [uint16]$m=0;[uint16]$n=0;if (![NativeArch]::IsWow64Process2($p.Handle,[ref]$m,[ref]$n)) { throw 'IsWow64Process2 failed' }; "PID=$($p.Id) process=0x$($m.ToString('X4')) native=0x$($n.ToString('X4'))" }
Arch (Get-Process -Id $PID)
$fixture = 'C:\CercanoRunner\phase1\transactions-02\running\1.0.0\tools\cercano-phase1-running.exe'
$stop = 'C:\CercanoRunner\phase1\architecture-stop'
if (Test-Path $stop) { throw 'Fresh architecture sentinel required' }
$p = Start-Process $fixture -ArgumentList $stop -PassThru -RedirectStandardOutput 'C:\CercanoRunner\phase1\architecture-fixture.txt'
try { Start-Sleep -Seconds 1; Arch $p } finally { Set-Content $stop 'exit'; if (!$p.WaitForExit(15000)) { throw 'Fixture did not exit; do not kill' } }
function PE($path) {
 $b=[IO.File]::ReadAllBytes($path);$off=[BitConverter]::ToInt32($b,60);"PE $path machine=0x$([BitConverter]::ToUInt16($b,$off+4).ToString('X4')) sha256=$((Get-FileHash $path).Hash)"
}
PE 'C:\ProgramData\chocolatey\bin\choco.exe'
PE $fixture
PE 'C:\Windows\Microsoft.NET\Framework64\v4.0.30319\csc.exe'
$choco = 'C:\ProgramData\chocolatey\bin\choco.exe'
$control = 'C:\CercanoRunner\phase1\transactions-04\barrier'
Get-Content "$control\hooks.txt"
Get-Content "$control\process.txt"
$owned = [int](Get-Content "$control\owned-pid.txt")
$p = Get-Process -Id $owned -ErrorAction SilentlyContinue
if ($p -and $p.Path -like '*cercano-phase1-barrier.exe') { Set-Content "$control\stop" 'exit'; if (!$p.WaitForExit(15000)) { throw 'Owned barrier fixture did not exit' } } else { 'Recorded barrier PID no longer identifies running owned fixture; no action taken' }
Set-Content "$control\mode.txt" 'normal'
& $choco list --limit-output
& $choco uninstall cercano-phase1-barrier -y --no-progress --limit-output --execution-timeout=60
if ($LASTEXITCODE -ne 0) { throw 'Barrier cleanup failed' }
& $choco list --limit-output
Get-ChildItem C:\ProgramData\chocolatey\bin -Filter 'cercano-phase1-*'
Get-ChildItem C:\ProgramData\chocolatey\lib -Filter 'cercano-phase1-*'
Get-ExecutionPolicy -List | Format-Table
