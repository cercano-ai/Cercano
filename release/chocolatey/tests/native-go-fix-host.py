#!/usr/bin/env python3
"""Serial bounded transport, exact disposable clone only."""
import base64,json,pathlib,subprocess,sys
ROOT=pathlib.Path(__file__).resolve().parents[3]
OUT=ROOT/'efforts/cross-platform-updates'
ARGV=['prlctl','exec','{SANITIZED_VM_2}',r'C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe','-NoProfile','-NonInteractive','-ExecutionPolicy','Bypass']
def ps(text,timeout=60):
 r=subprocess.run(ARGV+['-EncodedCommand',base64.b64encode(text.encode('utf-16le')).decode()],capture_output=True,timeout=timeout)
 return dict(exit=r.returncode,stdout=r.stdout.decode(errors='replace'),stderr=r.stderr.decode(errors='replace'))
def upload(path, source):
 text="[IO.File]::WriteAllBytes('"+path+"',[Convert]::FromBase64String([Console]::In.ReadToEnd()))"
 r=subprocess.run(ARGV+['-EncodedCommand',base64.b64encode(text.encode('utf-16le')).decode()],input=base64.b64encode(pathlib.Path(source).read_bytes()),capture_output=True,timeout=20)
 if r.returncode: raise RuntimeError((r.stdout,r.stderr))
if __name__=='__main__':
 text=pathlib.Path(sys.argv[1]).read_text(); result=ps(text,int(sys.argv[3]) if len(sys.argv)>3 else 60)
 (OUT/sys.argv[2]).write_text(json.dumps(result,indent=2)); print(json.dumps(result,indent=2))
 sys.exit(bool(result['exit']))
