#!/usr/bin/env python3
"""Clone-only transport; deliberately cannot target the preserved source VM."""
import sys,time
from rehearse import run
VM='5df50e96-bb17-4248-aa6e-50ae559cd765'
if sys.argv[1]=='guest':
 time.sleep(.5)
 s=sys.stdin.read()
 run(['prlctl','exec',VM,'/bin/bash','-c','"echo '+s.encode().hex()+' | /usr/bin/xxd -r -p | /bin/bash"'])
else: run(['prlctl',sys.argv[1],VM,*sys.argv[2:]])
