#!/usr/bin/env python3
"""Bounded guest-only rehearsal driver; every subprocess is recorded."""
import subprocess,json,pathlib,sys,base64,datetime,shlex
ROOT=pathlib.Path(__file__).resolve().parent
VM='08196962-608d-4fde-8284-49a5f020fea6'
def run(argv,timeout=900):
 p=subprocess.run(argv,capture_output=True,text=True,timeout=timeout)
 with (ROOT/'commands.jsonl').open('a') as f:f.write(json.dumps(dict(time=datetime.datetime.now().isoformat(),argv=argv,status=p.returncode,stdout=p.stdout,stderr=p.stderr))+'\n')
 print(p.stdout,end='');print(p.stderr,end='',file=sys.stderr)
 if p.returncode: raise SystemExit(p.returncode)
 return p.stdout
def guest(s):
 import time
 time.sleep(0.5)
 # prlctl reparses arguments; avoid mixed quote concatenation entirely.
 encoded=s.encode().hex()
 return run(['prlctl','exec',VM,'/bin/bash','-c','"echo '+encoded+' | /usr/bin/xxd -r -p | /bin/bash"'])
def put(local,remote):
 b=base64.b64encode(pathlib.Path(local).read_bytes()).decode()
 guest("test ! -e '"+remote+"'; : > '"+remote+".b64'")
 for i in range(0,len(b),180): guest("printf %s '"+b[i:i+180]+"' >> '"+remote+".b64'")
 guest("/usr/bin/base64 -D < '"+remote+".b64' > '"+remote+"'")
if __name__=='__main__':
 if sys.argv[1]=='guest':guest(sys.argv[2])
 elif sys.argv[1]=='put':put(sys.argv[2],sys.argv[3])
 else:run(sys.argv[1:])
