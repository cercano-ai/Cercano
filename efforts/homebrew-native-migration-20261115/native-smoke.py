#!/usr/bin/env python3
"""Acceptance-clone-only transport; separate append-only smoke evidence."""
import datetime,json,pathlib,subprocess,sys
ROOT=pathlib.Path(__file__).resolve().parent
VM='5df50e96-bb17-4248-aa6e-50ae559cd765'
def run(args):
 p=subprocess.run(args,capture_output=True,text=True,timeout=300)
 with (ROOT/'native-smoke-commands.jsonl').open('a') as f:
  f.write(json.dumps(dict(time=datetime.datetime.now().isoformat(),argv=args,status=p.returncode,stdout=p.stdout,stderr=p.stderr))+'\n')
 print(p.stdout,end='');print(p.stderr,end='',file=sys.stderr)
 return p.returncode
if __name__=='__main__':
 if sys.argv[1]=='guest':
  s=sys.stdin.read()
  sys.exit(run(['prlctl','exec',VM,'/bin/bash','-c','"echo '+s.encode().hex()+' | /usr/bin/xxd -r -p | /bin/bash"']))
 if sys.argv[1] not in ('start','list','snapshot-list'): raise SystemExit('unsupported operation')
 sys.exit(run(['prlctl',sys.argv[1],VM,*sys.argv[2:]]))
