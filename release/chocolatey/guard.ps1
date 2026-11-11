# Shared running-process check for the Cercano Chocolatey component packages.
# Rendered into each component as tools\cercano-guard.ps1 and dot-sourced by
# its scripts. Windows PowerShell 5.1 compatible.
#
# Evidence:
# - Phase 1 (efforts/cross-platform-updates/native-go-next-results.md on
#   branch handoff/windows-replacement-sanitized):
#   ordinary replacement does NOT refuse while the old executable runs (it
#   moves to lib-bkp and Chocolatey reports success). A throw from
#   chocolateyBeforeModify.ps1 is only a warning, and the audit shows it can
#   skip the backup that rollback needs, so BeforeModify only warns. A throw
#   from chocolateyInstall.ps1 failed the upgrade (-1) and rolled back.
# - x64 private-root run (release/chocolatey/README.md): the same install
#   refusal holds, and chocolateyUninstall.ps1 refusing keeps the package when
#   uninstall is attempted while the agent runs.
#
# The check never signals, stops or launches a process, and reads no user data.
# It is a snapshot, not a race-free exclusion: a launch after the check is not
# caught. Concurrent-launch behavior is unqualified (see the effort handoff).

if (-not ('CercanoGuard.FinalPath' -as [type])) {
    Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
using System.Text;
using Microsoft.Win32.SafeHandles;
namespace CercanoGuard {
    public static class FinalPath {
        [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
        static extern SafeFileHandle CreateFileW(string name, uint access, uint share, IntPtr security, uint disposition, uint flags, IntPtr template);
        [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
        static extern uint GetFinalPathNameByHandleW(SafeFileHandle handle, StringBuilder path, uint size, uint flags);
        // Resolves 8.3 names, junctions, symlinks and subst drives to one
        // canonical form. Returns null when the path cannot be opened.
        public static string Get(string path) {
            using (var handle = CreateFileW(path, 0, 7, IntPtr.Zero, 3, 0x02000000, IntPtr.Zero)) {
                if (handle.IsInvalid) return null;
                var buffer = new StringBuilder(1024);
                uint length = GetFinalPathNameByHandleW(handle, buffer, (uint)buffer.Capacity, 0);
                if (length == 0 || length >= buffer.Capacity) return null;
                var result = buffer.ToString();
                if (result.StartsWith(@"\\?\UNC\")) return @"\\" + result.Substring(8);
                if (result.StartsWith(@"\\?\")) return result.Substring(4);
                return result;
            }
        }
    }
}
'@
}

function Get-CercanoPackageProcess {
    param(
        [Parameter(Mandatory)] [string] $PackageId,
        [Parameter(Mandatory)] [string[]] $Executables,
        # Tests only: a fixed process list instead of the live one.
        [object[]] $Processes
    )

    $chocoRoot = $env:ChocolateyInstall
    if (-not $chocoRoot) {
        throw "ChocolateyInstall is not set; refusing to guess the package location."
    }
    $finalRoot = [CercanoGuard.FinalPath]::Get($chocoRoot)
    if (-not $finalRoot) { throw "Cannot resolve the Chocolatey folder '$chocoRoot'." }

    # Every place this package's files live during install, upgrade or
    # rollback, in canonical form. The trailing separator stops
    # 'cercano-agent' matching 'cercano-agent-something'.
    $roots = foreach ($area in 'lib', 'lib-bkp', 'lib-bad') {
        $areaPath = [CercanoGuard.FinalPath]::Get((Join-Path $chocoRoot $area))
        if (-not $areaPath) { $areaPath = Join-Path $finalRoot $area }
        (Join-Path $areaPath $PackageId).TrimEnd('\') + '\'
    }
    # The public shims are package-generated executables too, and a shim
    # process stays alive as the parent of the program it started (measured).
    $binPath = [CercanoGuard.FinalPath]::Get((Join-Path $chocoRoot 'bin'))
    if (-not $binPath) { $binPath = Join-Path $finalRoot 'bin' }
    $shims = foreach ($exe in $Executables) { Join-Path $binPath $exe }

    if ($null -eq $Processes) {
        # Enumeration failure throws: unknown is not the same as nothing running.
        $Processes = Get-CimInstance -ClassName Win32_Process -ErrorAction Stop
    }

    # Only these file names can run from this package (it ships no other
    # executable), so unrelated processes are never examined.
    foreach ($process in $Processes) {
        if ($Executables -notcontains $process.Name) { continue }
        $path = $process.ExecutablePath
        $final = if ($path) { [CercanoGuard.FinalPath]::Get($path) } else { $null }
        if (-not $final) {
            # Same file name, but its location is unreadable (typically another
            # user's process seen without elevation) or unresolvable. Fail
            # closed rather than assume it is someone else's copy.
            [pscustomobject]@{ ProcessId = $process.ProcessId; Name = $process.Name; Path = $null }
            continue
        }
        $owned = $false
        foreach ($root in $roots) {
            if ($final.StartsWith($root, [StringComparison]::OrdinalIgnoreCase)) { $owned = $true }
        }
        foreach ($shim in $shims) {
            if ([string]::Equals($final, $shim, [StringComparison]::OrdinalIgnoreCase)) { $owned = $true }
        }
        if ($owned) {
            [pscustomobject]@{ ProcessId = $process.ProcessId; Name = $process.Name; Path = $final }
        }
    }
}

function Assert-CercanoPackageNotRunning {
    param(
        [Parameter(Mandatory)] [string] $PackageId,
        [Parameter(Mandatory)] [string[]] $Executables,
        [switch] $WarnOnly,
        [object[]] $Processes
    )

    $found = @(Get-CercanoPackageProcess -PackageId $PackageId -Executables $Executables -Processes $Processes)
    if ($found.Count -eq 0) { return }

    $lines = foreach ($item in $found) {
        if ($item.Path) { "  PID $($item.ProcessId): $($item.Path)" }
        else { "  PID $($item.ProcessId): $($item.Name) (location not readable; run choco from an elevated shell to check it, or exit it)" }
    }
    $message = @(
        "$PackageId cannot be changed while these processes are running:"
        $lines
        "Exit them, then run the same choco command again. Nothing was stopped for you."
    ) -join [Environment]::NewLine

    if ($WarnOnly) { Write-Warning $message } else { throw $message }
}
