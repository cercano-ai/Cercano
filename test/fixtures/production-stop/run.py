import sys,os,pathlib,tempfile,subprocess as sp,time,json,threading,http.server,socket,signal,sqlite3
os.umask(0o077)
base=pathlib.Path(__file__).resolve().parent
mode=sys.argv[1] if len(sys.argv)>1 else 'host-only'
assert mode in ['host-only','native','combined']
root=pathlib.Path(tempfile.mkdtemp(prefix='cstop-',dir='/tmp')).resolve()
print('ROOT',root,flush=True)
for d in ['h/.config/cercano','t','data','cache','state','run','work']: (root/d).mkdir(parents=True,exist_ok=True)
conf=root/'h/.config/cercano'; hold=threading.Event(); release=threading.Event(); events=[]; owned={}; children=[]; extra_hosts=[]
expect={'native':'Before correction native shutdown must refuse on a host-owned RunCommand child, or leave writes running; after correction it must succeed and writes must stop. Tool lifetime is bounded to 12 seconds even on probe failure.', 'before':'Completed reply persisted; an existing idle worker plus second in-flight worker expected.','stop':'TERM then at most 20 0.1s polls, KILL only if host alive, then 0.2s. In-flight reply may be cancelled; no guaranteed post-stop host-bridge persistence.','writes':'Before-stop Write should create before.txt. Held response requests Write after host stop: cancellation may prevent execution; surviving process alone does not prove continuation. DB persistence and file writes checked separately.','isolation':'Untagged production binary, empty environment allowlist, loopback synthetic Ollama, disposable stores.'}
(root/'work/writer.sh').write_text('trap \"\" TERM\ni=0\nwhile [ $i -lt 120 ]; do echo tick >> \"'+str(root/'work/ticks.txt')+'\"; i=$((i+1)); /bin/sleep 0.1; done\n')
(root/'expectations.json').write_text(json.dumps(expect,indent=2))
class HTTP(http.server.BaseHTTPRequestHandler):
 def log_message(self,*a): pass
 def do_GET(self): self.respond({})
 def respond(self,obj):
  self.send_response(200);self.send_header('Content-Type','application/json');self.end_headers();self.wfile.write(json.dumps(obj).encode())
 def do_POST(self):
  body=json.loads(self.rfile.read(int(self.headers.get('Content-Length',0))) or '{}');events.append({'path':self.path,'body':body,'time':time.time()})
  model={'name':'fake-model','model':'fake-model','size':1,'digest':'fixture','details':{'family':'fixture','parameter_size':'1B','quantization_level':'Q4_0'}}
  try:
   if self.path in ['/api/tags','/api/ps']: return self.respond({'models':[model]})
   if self.path=='/api/show': return self.respond({'model_info':{'general.context_length':8192},'capabilities':['completion','tools'],'details':model['details']})
   if self.path in ['/api/embed','/api/embeddings']: return self.respond({'embeddings':[[0.1]*8],'embedding':[0.1]*8})
   if self.path in ['/api/chat','/api/generate']:
    held='HOLD_MARKER' in json.dumps(body)
    if held: hold.set();release.wait(20)
    text='AFTER_STOP_REPLY' if held else 'BEFORE_STOP_REPLY'
    self.send_response(200);self.send_header('Content-Type','application/x-ndjson');self.end_headers()
    message={'role':'assistant','content':text}
    if not any(m.get('role')=='tool' for m in body.get('messages',[])):
     message={'role':'assistant','content':'','tool_calls':[{'function':{'name':'RunCommand','arguments':{'cmd':['/bin/sh',str(root/'work/writer.sh')],'timeout_seconds':20}}}]}
    self.wfile.write((json.dumps({'model':'fake-model','message':message,'response':text,'done':True,'done_reason':'stop','prompt_eval_count':10,'eval_count':4})+'\n').encode());return
   self.respond({})
  except (BrokenPipeError,ConnectionResetError): pass
srv=http.server.ThreadingHTTPServer(('127.0.0.1',0),HTTP);threading.Thread(target=srv.serve_forever,daemon=True).start()
# GET tags needed for provider model validation.
def get(self):
 if self.path in ['/api/tags','/api/ps']: self.respond({'models':[{'name':'fake-model','model':'fake-model','size':1,'details':{'family':'fixture'}}]})
 elif self.path=='/api/version': self.respond({'version':'0.0.0'})
 else: self.respond({})
HTTP.do_GET=get
s=socket.socket();s.bind(('127.0.0.1',0));port=s.getsockname()[1];s.close()
url='http://127.0.0.1:'+str(srv.server_port)
(conf/'config.yaml').write_text('locus_mode: open_only\nopen_runtime: ollama\nollama_url: '+url+'\nport: "'+str(port)+'"\nmodels:\n  open:\n    overrides:\n      ollama:\n'+''.join('        '+t+': fake-model\n' for t in ['most_capable','everyday','fast_light','fast_light_text','embedding','vision']))
env={'PATH':'/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin','HOME':str(root/'h'),'XDG_CONFIG_HOME':str(root/'h/.config'),'XDG_DATA_HOME':str(root/'data'),'XDG_CACHE_HOME':str(root/'cache'),'XDG_STATE_HOME':str(root/'state'),'XDG_RUNTIME_DIR':str(root/'run'),'TMPDIR':str(root/'t'),'CERCANO_DISABLE_CREDENTIALS':'1','CERCANO_PORT':str(port),'CERCANO_CONVERSATIONS_DB':str(root/'state/conversations.db'),'CERCANO_OPEN_RUNTIME':'ollama','CERCANO_OPEN_MODEL':'fake-model','OLLAMA_URL':url,'AWS_EC2_METADATA_DISABLED':'true'}
(root/'env.json').write_text(json.dumps(env,indent=2))
def identity(pid):
 try:
  args=sp.check_output(['ps','-p',str(pid),'-o','args='],text=True).strip()
  if not args.startswith(str(base/'cercano')+' '): return None
  environ=sp.check_output(['ps','eww','-p',str(pid),'-o','command='],text=True).split()
  if 'HOME='+env['HOME'] not in environ or 'TMPDIR='+env['TMPDIR'] not in environ:return None
  return args+'\n'+sp.check_output(['ps','-p',str(pid),'-o','lstart='],text=True).strip()
 except sp.CalledProcessError:return None
def track(pid):
 i=identity(pid)
 if i: owned[pid]=i

def capture():
 # Scoped to children of the known, directly spawned host; never global discovery.
 r=sp.run(['pgrep','-P',str(host.pid)],capture_output=True,text=True)
 for p in r.stdout.split(): track(int(p))
 for extra in extra_hosts:
  for p in sp.run(['pgrep','-P',str(extra.pid)],capture_output=True,text=True).stdout.split(): track(int(p))
 for f in (root/'h').rglob('modelhost.json'):
  try: track(json.loads(f.read_text())['pid'])
  except (ValueError,KeyError): pass

def stop(pid):
 if identity(pid)!=owned.get(pid) or pid not in owned:return
 os.kill(pid,signal.SIGTERM)
 for _ in range(20):
  if pid==host.pid and host.poll() is not None:break
  if identity(pid)!=owned[pid]:break
  time.sleep(.1)
 killed=False
 if identity(pid)==owned[pid] and not (pid==host.pid and host.poll() is not None):os.kill(pid,signal.SIGKILL);killed=True;time.sleep(.2)
 return killed

def snap(name):
 p=root/'state/conversations.db'
 if p.exists():
  c=sqlite3.connect('file:'+str(p)+'?mode=ro',uri=True)
  data={}
  for table in ['conversations','turns','delegated_tasks']:
   try:data[table]=[dict(zip([d[0] for d in q.description],r)) for q in [c.execute('select * from '+table)] for r in q.fetchall()]
   except sqlite3.Error as e:data[table]=str(e)
  c.close();(root/(name+'.json')).write_text(json.dumps(data,indent=2,default=str))
log=open(root/'host.log','w');host=sp.Popen([str(base/'cercano'),'agent'],env=env,cwd=root/'work',stdout=log,stderr=sp.STDOUT);track(host.pid)
sibling=None
unidentified=None
result={'source_commit':sp.check_output(['git','rev-parse','HEAD'],cwd=base,text=True).strip()}
try:
 for _ in range(100):
  capture()
  if host.poll() is not None:raise RuntimeError('host exited '+str(host.returncode))
  try:
   with socket.create_connection(('127.0.0.1',port),timeout=.1):break
  except OSError:time.sleep(.1)
 def client(prompt,name):
  out=open(root/(name+'.log'),'w');p=sp.Popen([str(base/'client'),'127.0.0.1:'+str(port),prompt,str(root/'work'),'stop-'+name],env=env,stdout=out,stderr=sp.STDOUT);children.append(p);return p
 p=client('Run the requested fixture command. Return BEFORE_STOP_REPLY.','before')
 for _ in range(100):
  capture()
  if (root/'work/ticks.txt').exists():break
  time.sleep(.1)
 result['writer_started']=(root/'work/ticks.txt').exists();snap('before');result['existing_pids']=list(owned)
 p=client('HOLD_MARKER return AFTER_STOP_REPLY only.','inflight')
 for _ in range(150):
  capture()
  if hold.is_set():break
  if p.poll() is not None:break
  time.sleep(.1)
 result['provider_hold_reached']=hold.is_set();result['owned_before_stop']=owned.copy();snap('at-stop')
 result['late_workers']=[pid for pid,i in owned.items() if pid not in result['existing_pids'] and ' worker --socket ' in i]
 assert result['late_workers'], 'second production worker was not observed after initial inventory'
 result['ticks_at_stop']=(root/'work/ticks.txt').read_text().count('tick')
 if mode != 'combined':
  if mode in ['native']:
   r=sp.run([str(base/'upgrade-fixture'),'127.0.0.1:'+str(port),str(host.pid)],env=env,capture_output=True,text=True,timeout=15);result['native_exit']=r.returncode;result['native_output']=r.stdout+r.stderr
  else: result['host_killed']=stop(host.pid)
  result['host_exit']=host.poll();result['stop_time']=time.time();snap('immediate-after-stop');release.set();result['ticks_after_shutdown']=(root/'work/ticks.txt').read_text().count('tick');time.sleep(2);capture();snap('after-release');result['ticks_after_wait']=(root/'work/ticks.txt').read_text().count('tick');result['survivors']={str(pid):identity(pid) for pid in owned if identity(pid)}
  if mode in ['native']:
   assert result['writer_started'] and result['provider_hold_reached'], 'production work not established'
   assert result['native_exit']==0 and result['host_exit'] is not None, 'native stop failed'
   assert result['ticks_after_shutdown']==result['ticks_after_wait'], 'tool continues writing'
   assert result['survivors'] and all(' model-host' in i for i in result['survivors'].values()), 'unexpected survivor or lost shared model host'
 if mode=='combined':
  # Unrelated unavailable image: no role/socket corroboration and no signal authority.
  image=root/'cercano'
  sleeper=root/'unrelated.c'
  sleeper.write_text('#include <unistd.h>\nint main(void) { sleep(120); return 0; }\n')
  sp.run(['/usr/bin/cc',str(sleeper),'-o',str(image)],env=env,check=True)
  unidentified=sp.Popen([str(image),'120'],env=env)
  time.sleep(.2)
  assert unidentified.poll() is None
  image.unlink()
  result['unidentified_pid']=unidentified.pid
  # A second untagged production writer shares only this disposable store.
  # It has its own listener and active worker; no client-idle prerequisite.
  s=socket.socket();s.bind(('127.0.0.1',0));extra_port=s.getsockname()[1];s.close()
  extra=sp.Popen([str(base/'cercano'),'agent'],env=dict(env,CERCANO_PORT=str(extra_port)),cwd=root/'work',stdout=open(root/'extra-host.log','w'),stderr=sp.STDOUT)
  extra_hosts.append(extra);track(extra.pid)
  for _ in range(100):
   if extra.poll() is not None: raise RuntimeError('additional host exited')
   try:
    with socket.create_connection(('127.0.0.1',extra_port),timeout=.1):break
   except OSError:time.sleep(.1)
  extra_client=sp.Popen([str(base/'client'),'127.0.0.1:'+str(extra_port),'HOLD_MARKER additional active writer',str(root/'work'),'extra'],env=env,stdout=open(root/'extra-client.log','w'),stderr=sp.STDOUT)
  children.append(extra_client)
  extra_workers=[]
  for _ in range(100):
   capture()
   extra_workers=[int(p) for p in sp.run(['pgrep','-P',str(extra.pid)],capture_output=True,text=True).stdout.split() if ' worker ' in owned.get(int(p),'')]
   if extra_workers:break
   time.sleep(.1)
  assert extra_workers, 'additional production worker not established'
  result['additional_host']=extra.pid;result['additional_workers']=extra_workers
  sibling=sp.Popen([str(base/'cercano'),'worker','--socket',str(root/'sibling.sock')],env=env,cwd=root/'work',stdout=open(root/'sibling.log','w'),stderr=sp.STDOUT)
  track(sibling.pid)
  for _ in range(100):
   if (root/'sibling.sock').exists():break
   assert sibling.poll() is None
   time.sleep(.1)
  assert (root/'sibling.sock').exists()
  result['sibling_worker']={'pid':sibling.pid,'parent':int(sp.check_output(['/bin/ps','-p',str(sibling.pid),'-o','ppid='],text=True)),'identity':identity(sibling.pid)}
  assert result['sibling_worker']['parent']==os.getpid()
  descendants=[]
  def descend(pid):
   for raw in sp.run(['pgrep','-P',str(pid)],capture_output=True,text=True).stdout.split():
    child=int(raw)
    if ' model-host' in owned.get(child,''): continue  # Shared model host is not a DB writer.
    descendants.append(child); descend(child)
  descend(host.pid);descend(extra.pid);descendants.append(extra.pid)
  assert sibling.pid not in descendants
  result['owned_descendants_before_confirmation']=descendants
  assert len(descendants)>=3, 'expected workers plus live work descendant'
  db=root/'state/conversations.db'
  with sqlite3.connect(db) as c:
   c.execute("insert into delegated_tasks(id,parent_id,status,task,work_dir,created_at,updated_at) values('fixture-task','fixture-parent','running','preserve without replay','',1,2)")
  # Keep production endpoint authority, but use the compiled synthetic generation backend.
  import shutil
  shutil.copy2(conf/'config.yaml',root/'state/config.yaml')
  upgrade_env=dict(env,CERCANO_FIXTURE_UNIDENTIFIED=str(unidentified.pid),CERCANO_FIXTURE_OWNED_DESCENDANTS=','.join(map(str,[host.pid,*descendants])),CERCANO_FIXTURE_PORT=str(port),CERCANO_ACCEPTANCE_UPGRADE_PROCESSES='owned-worker',CERCANO_FIXTURE_SIBLING_WORKER=str(sibling.pid),CERCANO_FIXTURE_OWNED_HOST=str(host.pid),CERCANO_FIXTURE_ADDITIONAL_HOSTS=str(extra.pid),CERCANO_FIXTURE_STOP_RECORD=str(root/'native-stop.log'),CERCANO_FIXTURE_UPGRADE_HELPER=str(base/'cercano-native-upgrade'))
  from generation import accept
  accept(base.parents[2],root,upgrade_env,None,result,upgrading=True)
  assert unidentified.poll() is None, 'unrelated process terminated'
  result['unidentified_survived_upgrade']=True
  assert sibling.poll() is None and identity(sibling.pid)==result['sibling_worker']['identity']
  with socket.socket(socket.AF_UNIX) as probe: probe.connect(str(root/'sibling.sock'))
  result['sibling_worker']['survived_and_serving_after_generation']=True
  result['host_exit']=host.poll()
  assert result['host_exit'] is not None
  result['additional_host_exit']=extra.poll()
  assert result['additional_host_exit'] is not None
  ticks=(root/'work/ticks.txt').read_text().count('tick');time.sleep(2)
  assert (root/'work/ticks.txt').read_text().count('tick')==ticks
  result['fixture_combined_acceptance']=True
except Exception as e:result['error']=repr(e)
finally:
 if unidentified is not None:
  unidentified.terminate(); unidentified.wait(timeout=3)
  result['unidentified_cleanup_exit']=unidentified.returncode
 release.set();capture()
 # Writers self-expire; never signal an unverified shell or its process group.
 time.sleep(13)
 for pid in list(owned):stop(pid)
 for p in children:
  if p.poll() is None:p.terminate()
  try:p.wait(timeout=3)
  except sp.TimeoutExpired:p.kill();p.wait()
 if host.poll() is None:stop(host.pid)
 if sibling is not None: sibling.wait(timeout=3); result['sibling_cleanup_exit']=sibling.returncode
 result['cleanup_remaining']={str(pid):identity(pid) for pid in owned if identity(pid)}
 result['filesystem']={f.name:f.read_text() for f in (root/'work').glob('*.txt')}
 (root/'requests.json').write_text(json.dumps(events,indent=2));(root/'result.json').write_text(json.dumps(result,indent=2));srv.shutdown();print(json.dumps(result,indent=2),flush=True)

if 'error' in result: sys.exit(1)
