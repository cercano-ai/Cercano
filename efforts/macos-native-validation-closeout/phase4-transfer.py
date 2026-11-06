#!/usr/bin/env python3
"""Exact tracked-source snapshot; one-file, guest-only, bounded capability transfer."""
import hashlib, http.server, io, json, pathlib, secrets, subprocess, tarfile, tempfile, threading
ROOT = pathlib.Path(__file__).resolve().parents[2]
VM = 'd5633dbc-c8a1-49fc-a420-df6d5d60d952'
def run(args):
    print('+', *args, flush=True)
    p = subprocess.run(args, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=120)
    print(p.stdout, flush=True)
    p.check_returncode()
    return p.stdout
rev = subprocess.check_output(['git','rev-parse','HEAD'], cwd=ROOT, text=True).strip()
files = subprocess.check_output(['git','ls-files'], cwd=ROOT, text=True).splitlines()
assets = {'prototypes.yaml','cl100k_base.tiktoken','catalog.json','schema.sql','tier_recommendations.yaml','ddg_search.py','real_conversation.json'}
selected = [p for p in files if (p.startswith('source/server/') and (p.endswith('.go') or pathlib.Path(p).name in assets | {'go.mod','go.sum'} or '/internal/skills/catalog/' in p)) or (p.startswith('source/third_party/') and pathlib.Path(p).suffix in {'.go','.mod','.sum','.s','.h','.c'}) or (p.startswith('efforts/macos-native-validation-closeout/phase4-') and p.endswith('.py'))]
# Only committed bytes: never pick up local logs, environment files or credentials.
manifest = {}
data = io.BytesIO()
with tarfile.open(fileobj=data, mode='w:gz') as tar:
    for path in selected:
        body = subprocess.check_output(['git','show',rev+':'+path], cwd=ROOT)
        manifest[path] = hashlib.sha256(body).hexdigest()
        info = tarfile.TarInfo(path); info.size=len(body); info.mode=0o600
        tar.addfile(info, io.BytesIO(body))
    body = json.dumps({'revision':rev,'files':manifest},sort_keys=True,indent=2).encode()
    info=tarfile.TarInfo('phase4-manifest.json'); info.size=len(body); info.mode=0o600
    tar.addfile(info,io.BytesIO(body))
payload=data.getvalue(); digest=hashlib.sha256(payload).hexdigest()
archive=ROOT / ('.phase4-'+digest+'.tar.gz')
archive.write_bytes(payload)
print('ARCHIVE', archive.name, 'REVISION',rev,'FILES',len(selected),'SIZE',len(payload),'SHA256',digest,flush=True)
cap='/' + secrets.token_urlsafe(32)
class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        if self.client_address[0] != '10.211.55.4' or self.path != cap:
            self.send_error(404); return
        self.send_response(200); self.send_header('Content-Length',str(len(payload))); self.end_headers(); self.wfile.write(payload)
    def log_message(self,*args): pass
server=http.server.HTTPServer(('10.211.55.2',0), Handler)
thread=threading.Thread(target=server.serve_forever,daemon=True); thread.start()
# No listing, no filesystem mapping; one exact in-memory archive only. Deadline
# closes the listener even if the guest command hangs or verification fails.
timer=threading.Timer(150,server.shutdown); timer.start()
base=['prlctl','exec',VM,'/usr/bin/sudo','-u','test','/usr/bin/env','HOME=/tmp/cnp.h8sjTc/home','XDG_CONFIG_HOME=/tmp/cnp.h8sjTc/config']
destination='/tmp/cnp.h8sjTc/phase4-'+digest+'.tar.gz'
try:
    # Capability intentionally not printed to persistent evidence.
    p=subprocess.run(base+['/usr/bin/curl','--fail','--silent','--show-error','--max-time','90','--output',destination,'http://10.211.55.2:'+str(server.server_port)+cap],capture_output=True,text=True,timeout=110)
    print('guest curl exit',p.returncode,p.stdout,p.stderr,flush=True); p.check_returncode()
    out=run(base+['/usr/bin/shasum','-a','256',destination])
    if digest not in out.split(): raise RuntimeError('complete guest SHA mismatch: extraction forbidden')
    print('COMPLETE HOST/GUEST SHA256 MATCH; extraction now permitted',flush=True)
finally:
    timer.cancel(); server.shutdown(); server.server_close(); thread.join()
    print('TRANSFER LISTENER CLOSED',flush=True)
print('GUEST_ARCHIVE',destination,flush=True)
