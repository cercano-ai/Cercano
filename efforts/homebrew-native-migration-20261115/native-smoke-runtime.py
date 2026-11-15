#!/usr/bin/env python3
"""Runs as synthetic UID under reviewed outer sandbox; all children contained."""
import os,subprocess,json,time,socket,threading,http.server,pty,select,signal,fcntl,termios,struct,pathlib,hashlib
R=pathlib.Path('/opt/cercano-native-smoke-20261115-1357')
E=R/'evidence-tier-retry'
assert E.is_dir(), 'owned evidence directory must be provisioned before sandbox entry'
B=R/'brew/bin'
def log(x): print(x,flush=True)
def run_gate():
 # Capability probe only: this release warns and falls back on invalid input.
 # Its nonzero exit below is from the sandbox denying default port 50052,
 # NOT proof of fail-closed startup. Valid=1 runs additionally log disabled
 # keychain access; the outer keychain/securityd policy remains mandatory.
 env=dict(os.environ,CERCANO_DISABLE_CREDENTIALS='invalid')
 p=subprocess.run([str(B/'cercano'),'agent'],env=env,capture_output=True,text=True,timeout=15)
 (E/'credential-gate.txt').write_text(p.stdout+p.stderr)
 log({'credential_gate_exit':p.returncode,'output':p.stdout+p.stderr})
 assert p.returncode!=0 and 'CERCANO_DISABLE_CREDENTIALS' in p.stdout+p.stderr, 'BLOCKER: release credential capability unconfirmed'
run_gate()
class Provider(http.server.BaseHTTPRequestHandler):
 def log_message(self,*a): pass
 def do_GET(self): self.reply({'models':[{'name':'fixture-smoke','model':'fixture-smoke','size':1,'details':{'family':'synthetic'}}]})
 def do_POST(self):
  data=self.rfile.read(int(self.headers.get('Content-Length','0')))
  with (E/'provider.jsonl').open('a') as f: f.write(json.dumps({'path':self.path,'body':data.decode()})+'\n')
  self.reply({'model':'fixture-smoke','message':{'role':'assistant','content':'SYNTHETIC_SMOKE_RESPONSE_1357'},'response':'SYNTHETIC_SMOKE_RESPONSE_1357','done':True,'prompt_eval_count':5,'eval_count':5})
 def reply(self,obj):
  data=(json.dumps(obj)+'\n').encode();self.send_response(200);self.send_header('Content-Type','application/json');self.send_header('Content-Length',str(len(data)));self.end_headers();self.wfile.write(data)
server=http.server.ThreadingHTTPServer(('127.0.0.1',49392),Provider)
threading.Thread(target=server.serve_forever,daemon=True).start()
afile=(E/'agent.log').open('w')
agent=subprocess.Popen([str(B/'cercano'),'agent'],stdout=afile,stderr=subprocess.STDOUT)
cli=None
try:
 for i in range(40):
  if agent.poll() is not None: raise RuntimeError('agent exited '+str(agent.returncode))
  try:
   with socket.create_connection(('127.0.0.1',49391),timeout=.3): break
  except OSError: time.sleep(.5)
 else: raise RuntimeError('agent loopback readiness timeout')
 master,slave=pty.openpty();fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',40,120,0,0))
 def terminal_session():
  os.setsid();fcntl.ioctl(slave,termios.TIOCSCTTY,0)
 cli=subprocess.Popen([str(B/'cercano-cli'),'--address','127.0.0.1:49391'],stdin=slave,stdout=slave,stderr=slave,preexec_fn=terminal_session);os.close(slave)
 transcript=b''
 def collect(seconds):
  global transcript
  until=time.time()+seconds
  while time.time()<until:
   if select.select([master],[],[],.2)[0]:
    try: chunk=os.read(master,65536)
    except OSError: break
    transcript+=chunk
    # Bubble Tea terminal queries need deterministic synthetic answers.
    if b'\x1b[6n' in chunk: os.write(master,b'\x1b[1;1R')
    if b'\x1b]11;?' in chunk: os.write(master,b'\x1b]11;rgb:0000/0000/0000\x1b\\')
   (E/'cli.pty').write_bytes(transcript)
 collect(5);os.write(master,b'SYNTHETIC_SMOKE_PROMPT_1357\r');collect(25)
 os.write(master,b'/exit\r');collect(5)
 log({'cli_exit':cli.poll(),'provider_observed':(E/'provider.jsonl').exists(),'response_in_pty':b'SYNTHETIC_SMOKE_RESPONSE_1357' in transcript})
 assert cli.poll()==0,'CLI graceful exit not confirmed'
 assert b'SYNTHETIC_SMOKE_RESPONSE_1357' in transcript,'synthetic response absent'
finally:
 if cli and cli.poll() is None:
  cli.send_signal(signal.SIGTERM)
  try: cli.wait(timeout=8)
  except subprocess.TimeoutExpired: cli.kill();cli.wait()
 if agent.poll() is None:
  agent.send_signal(signal.SIGTERM)
  try: agent.wait(timeout=10)
  except subprocess.TimeoutExpired: agent.kill();agent.wait()
 log({'agent_exit':agent.returncode})
 assert agent.returncode==0, 'agent graceful exit failed'
 server.shutdown();server.server_close();afile.close()
 log((E/'agent.log').read_text()[-14000:])
 for p in (R/'home').rglob('*'):
  if p.is_file(): log({'persisted':str(p.relative_to(R)),'size':p.stat().st_size,'sha256':hashlib.sha256(p.read_bytes()).hexdigest()})
