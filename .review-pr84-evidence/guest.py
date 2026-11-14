import subprocess,base64,sys,os
from pathlib import Path
s=Path(sys.argv[1]).read_text() if sys.argv[1]!='-' else sys.stdin.read()
args=['prlctl','exec','cercano-windows-packaging-20261106-1117-full']
if '--system' not in sys.argv: args += ['--user',os.environ['PR84_USER'],'--password',os.environ['PR84_PASSWORD']]
args += ['powershell.exe','-NoProfile','-NonInteractive','-EncodedCommand',base64.b64encode(s.encode('utf-16le')).decode()]
r=subprocess.run(args,capture_output=True,text=True)
print(r.stdout,end=''); print(r.stderr,end='',file=sys.stderr);print('PRL_EXIT='+str(r.returncode));sys.exit(r.returncode)
