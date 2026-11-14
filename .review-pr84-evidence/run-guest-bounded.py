"""Bounded, verified transport; --system explicitly selects existing guest.py channel."""
import argparse
import base64
import gzip
import hashlib
import json
import subprocess
import sys
import time
import uuid
from pathlib import Path

EVIDENCE = Path(__file__).resolve().parent

def main():
    p = argparse.ArgumentParser()
    p.add_argument('name')
    p.add_argument('--system', action='store_true')
    p.add_argument('--timeout', type=int, default=600)
    a = p.parse_args()
    if not a.name.replace('-', '').replace('_', '').isalnum():
        p.error('name must contain only letters, digits, hyphens or underscores')
    raw = (EVIDENCE / 'windows-env.ps1').read_bytes() + b'\n' + (EVIDENCE / (a.name + '.ps1')).read_bytes()
    packed = gzip.compress(raw, mtime=0)
    payload = base64.b64encode(packed).decode('ascii')
    digest = hashlib.sha256(raw).hexdigest()
    packed_digest = hashlib.sha256(packed).hexdigest()
    target = r'C:\Users\Public\pr84-20261113\logs\upload-' + uuid.uuid4().hex
    records = []
    def call(stage, script, timeout=30):
        # Bound the ENTIRE UTF-16LE/base64 argument, not just the payload fragment.
        encoded = base64.b64encode(script.encode('utf-16le'))
        if len(encoded) > 3200:
            raise ValueError('transport command exceeds 3200 encoded characters')
        time.sleep(0.3)
        cmd = [sys.executable, str(EVIDENCE / 'guest.py'), '-']
        if a.system:
            cmd.append('--system')
        try:
            r = subprocess.run(cmd, input=script, text=True, capture_output=True, timeout=timeout)
            code, out, err = r.returncode, r.stdout, r.stderr
        except subprocess.TimeoutExpired as ex:
            code, out, err = 124, (ex.stdout or b'').decode(), (ex.stderr or b'').decode()
        records.append(dict(stage=stage, encoded_chars=len(encoded), exit=code, stdout=out, stderr=err))
        return code
    code = 1
    try:
        for i in range(0, len(payload), 300):
            # Fixed-offset writes are idempotent if a session loses its reply.
            script = "$ErrorActionPreference='Stop';$f=[IO.File]::OpenWrite('" + target + ".b64');$f.Position=" + str(i) + ";$b=[Text.Encoding]::ASCII.GetBytes('" + payload[i:i+300] + "');$f.Write($b,0,$b.Length);$f.Dispose();exit 0"
            for attempt in range(3):
                code = call('chunk-' + str(i//300) + '-attempt-' + str(attempt+1), script)
                if code != 255:
                    break
                time.sleep(1)
            if code:
                return code
        code = call('decode', "$ErrorActionPreference='Stop';$p='" + target + "';$b=[Convert]::FromBase64String([IO.File]::ReadAllText($p+'.b64'));[IO.File]::WriteAllBytes($p+'.gz',$b);if((Get-FileHash ($p+'.gz')).Hash -ne '" + packed_digest + "'){exit 91};$m=New-Object IO.MemoryStream(,$b);$g=New-Object IO.Compression.GZipStream($m,[IO.Compression.CompressionMode]::Decompress);$f=[IO.File]::Create($p+'.ps1');$g.CopyTo($f);$f.Close();$g.Dispose();if((Get-FileHash ($p+'.ps1')).Hash -ne '" + digest + "'){exit 92};Write-Output 'HASH_OK " + digest + "';exit 0")
        if code:
            return code
        code = call('execute', "Invoke-Expression ([IO.File]::ReadAllText('" + target + ".ps1'))", a.timeout)
        print(records[-1]['stdout'])
        print(records[-1]['stderr'], file=sys.stderr)
        return code
    finally:
        cleanup = call('cleanup', "$ErrorActionPreference='Stop';Remove-Item -LiteralPath '" + target + ".b64','" + target + ".gz','" + target + ".ps1' -Force -ErrorAction SilentlyContinue;Write-Output 'UPLOAD_CLEANED';exit 0")
        (EVIDENCE / (a.name + '.transport.json')).write_text(json.dumps(dict(identity_channel='SYSTEM' if a.system else 'environment credentials', raw_bytes=len(raw), sha256=digest, gzip_sha256=packed_digest, wrapper_exit=code, cleanup_exit=cleanup, calls=records), indent=2))
        (EVIDENCE / (a.name + '.log')).write_text(''.join(r['stdout'] + r['stderr'] for r in records) + '\nWRAPPER_EXIT=' + str(code) + '\n')
        print('WRAPPER_EXIT=' + str(code))

if __name__ == '__main__':
    sys.exit(main())
