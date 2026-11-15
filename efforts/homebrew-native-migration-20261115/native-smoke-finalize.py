#!/usr/bin/env python3
"""Read-only persistence/process checks, then graceful clone shutdown; retains fixtures."""
import pathlib,subprocess,time
H=pathlib.Path(__file__).resolve().parent
s='''set -euo pipefail
R=/opt/cercano-native-smoke-20261115-1357
shasum -a 256 "$R/safety.sb"
cat "$R/home/.config/cercano/config.yaml"
sudo -u _cercanorehearsal env -i HOME="$R/home" PATH=/usr/bin:/bin TMPDIR="$R/tmp" CERCANO_DISABLE_CREDENTIALS=1 sandbox-exec -f "$R/safety.sb" /usr/bin/python3 - <<'PY'
import pathlib,sqlite3,socket
r=pathlib.Path('/opt/cercano-native-smoke-20261115-1357')
c=sqlite3.connect('file:'+str(r/'home/.config/cercano/conversations.db')+'?mode=ro&immutable=1',uri=True)
assert c.execute('pragma integrity_check').fetchone()==('ok',)
dump='\n'.join(c.iterdump())
for marker in ('SYNTHETIC_SMOKE_PROMPT_1357','SYNTHETIC_SMOKE_RESPONSE_1357'):
 assert marker in dump, 'missing persisted marker: '+marker
 print('Persisted conversation marker:',marker)
print('Conversation DB integrity: ok')
c.close()
for address in [('127.0.0.1',49391),('127.0.0.1',49392)]:
 s=socket.socket();s.settimeout(1)
 try: s.connect(address)
 except ConnectionRefusedError: print('Fixture endpoint closed:',address)
 else: raise AssertionError('fixture endpoint still open: '+str(address))
 finally: s.close()
s=socket.socket()
try: s.bind(('127.0.0.1',49393))
except PermissionError: print('Unlisted loopback bind denied as expected')
else: raise AssertionError('sandbox allowed unlisted port')
finally: s.close()
PY
# No signals or broad kill: fail if a matching executable remains after owned cleanup.
if ps -axo uid=,pid=,comm= | grep -E '(^|/)(cercano|cercano-cli|cercano-model-host)$'; then
 echo 'BLOCKER: residual executable; inspect ownership before cleanup'; exit 70
fi
test -d /opt/cercano-snapshot-recovery-20261115
sync
'''.replace("dump='\n'.join", "dump='\\n'.join")
subprocess.run(['python3',str(H/'native-smoke.py'),'guest'],input=s,text=True,check=True)
subprocess.run(['python3',str(H/'native-smoke.py'),'snapshot-list'],check=True)
# Guest shutdown closes transport; nonzero transport result is not itself failure.
subprocess.run(['python3',str(H/'native-smoke.py'),'guest'],input='/sbin/shutdown -h now\n',text=True)
for _ in range(15):
 p=subprocess.run(['python3',str(H/'native-smoke.py'),'list','-i'],capture_output=True,text=True,check=True)
 print(p.stdout,end='')
 if 'State: stopped' in p.stdout: break
 time.sleep(2)
else: raise SystemExit('BLOCKER: clone shutdown not confirmed; do not force/reset')
