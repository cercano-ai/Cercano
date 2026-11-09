#!/usr/bin/env python3
"""Clone-only transport; uploads a UTF-8 script then invokes absolute -File.
No host shell, credentials, persistent policy changes, or alternate VM target.
Usage: python3 guest.py script.ps1 [capture.txt]
"""
import base64
import pathlib
import subprocess
import sys

CLONE = '{SANITIZED_VM_2}'
PS = r'C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe'
DEST = r'C:\CercanoRunner\phase1\entry.ps1'

def main():
    content = pathlib.Path(sys.argv[1]).read_text(encoding='utf-8-sig').encode('utf-8-sig')
    argv = ['prlctl', 'exec', CLONE, PS, '-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass']
    def execute(text):
        command = argv + ['-EncodedCommand', base64.b64encode(text.encode('utf-16le')).decode()]
        for attempt in range(4):
            result = subprocess.run(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            if result.returncode == 0:
                return
            if result.returncode != 255:
                break
        raise RuntimeError(f'Upload failed: {result.stdout!r} {result.stderr!r}')
    execute(r"New-Item -ItemType Directory -Force C:\CercanoRunner\phase1 | Out-Null; " + f"[IO.File]::WriteAllBytes('{DEST}',[byte[]]@())")
    # Keep each encoded argument below the guest transport command-line bound.
    for offset in range(0, len(content), 700):
        payload = base64.b64encode(content[offset:offset+700]).decode('ascii')
        execute(f"$s=[IO.File]::OpenWrite('{DEST}');$s.Seek({offset},0)|Out-Null;$b=[Convert]::FromBase64String('{payload}');$s.Write($b,0,$b.Length);$s.Dispose()")
    result = subprocess.run(argv + ['-File', DEST] + sys.argv[3:], stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    if len(sys.argv) > 2:
        pathlib.Path(sys.argv[2]).write_bytes(result.stdout)
    sys.stdout.buffer.write(result.stdout)
    return result.returncode

if __name__ == '__main__':
    sys.exit(main())
