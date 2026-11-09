$ErrorActionPreference = 'Stop'
Add-Type @'
using System; using System.Runtime.InteropServices;
public class NativeArch {
 [DllImport("kernel32.dll", SetLastError=true)] public static extern bool IsWow64Process2(IntPtr p, out ushort process, out ushort native);
}
'@
foreach ($exe in @('C:\ProgramData\chocolatey\bin\choco.exe','C:\Windows\Microsoft.NET\Framework64\v4.0.30319\csc.exe')) {
 $p = Start-Process $exe -ArgumentList '/?' -PassThru -RedirectStandardOutput 'C:\CercanoRunner\phase1\arch-help.txt'
 try { [uint16]$m=0;[uint16]$n=0;if (![NativeArch]::IsWow64Process2($p.Handle,[ref]$m,[ref]$n)) { throw 'IsWow64Process2 failed' }; "$exe PID=$($p.Id) process=0x$($m.ToString('X4')) native=0x$($n.ToString('X4'))" } finally { if (!$p.WaitForExit(15000)) { throw 'Help process did not exit; do not kill' } }
}
