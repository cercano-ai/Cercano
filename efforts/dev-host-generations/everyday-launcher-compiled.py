#!/usr/bin/env python3
"""Offline compiled bounded A/B rehearsal; not ordinary-startup parity acceptance."""
import argparse, errno, fcntl, hashlib, json, os, pathlib, pty, select, shlex, shutil, signal, sqlite3, struct, subprocess, sys, tempfile, termios, time, traceback
P=pathlib.Path
# Startup-worker coordination: pass an explicit private config-authority file
# from launcher selection into ordinary host startup (including restart). It
# must stay under this rehearsal's HOME, and UpdateConfig must persist there;
# do not fall back to the operator's default config. This harness intentionally
# does not edit launcher/startup composition or claim its convergence complete.

class Terminal:
    def __init__(self, argv, env, log):
        self.fd, slave=pty.openpty()
        fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',40,140,0,0))
        self.p=subprocess.Popen(argv,stdin=slave,stdout=slave,stderr=slave,env=env,start_new_session=True)
        os.close(slave); self.data=b''; self.log=log
    def pump(self):
        while select.select([self.fd],[],[],.05)[0]:
            try: b=os.read(self.fd,65536)
            except OSError as e:
                if e.errno==errno.EIO: break
                raise
            if not b: break
            self.data+=b; self.log.write_bytes(self.data)
            # Answer terminal queries (not application input).
            if b'\x1b[6n' in b: os.write(self.fd,b'\x1b[1;1R')
            if b'\x1b[c' in b: os.write(self.fd,b'\x1b[?1;2c')
            if b'\x1b[>c' in b: os.write(self.fd,b'\x1b[>0;0;0c')
            if b'\x1b]11;?' in b: os.write(self.fd,b'\x1b]11;rgb:0000/0000/0000\x1b\\')
            if b'\x1b]10;?' in b: os.write(self.fd,b'\x1b]10;rgb:ffff/ffff/ffff\x1b\\')
    def send(self,s): os.write(self.fd,s.encode())
    def stop(self):
        if self.p.poll() is None:
            self.send('\x03'); time.sleep(.3); self.pump()
        if self.p.poll() is None: self.p.terminate()
        try: self.p.wait(timeout=5)
        except subprocess.TimeoutExpired: self.p.kill(); self.p.wait()
        os.close(self.fd)

class OwnedProcesses:
    """Observe only fixture-recorded PIDs and children of verified own processes."""
    def __init__(self, root):
        self.root=root; self.identities={}; self.actions=[]
    def identity(self, pid):
        if not isinstance(pid,int) or pid<=1: return None
        try:
            p=subprocess.run(['ps','-ww','-p',str(pid),'-o','lstart=,command='],
                             capture_output=True,text=True,timeout=2)
            fields=p.stdout.strip().split(None,5)
            if p.returncode or len(fields)!=6: return None
            args=shlex.split(fields[5])
            # Exact path components, never a substring match against a command line.
            generation=self.root/'state/generations'
            binary=P(args[0]) if args else P('.')
            own_binary=(binary.name=='cercano' and binary.parent.parent==generation)
            own_mcp=str(self.root/'barrier/mcp-fixture') in args[:2]
            if not (own_binary or own_mcp): return None
            return {'pid':pid,'start':' '.join(fields[:5]),'command':fields[5]}
        except (OSError,ValueError,subprocess.TimeoutExpired): return None
    def observe(self, pid):
        if pid in self.identities: return
        identity=self.identity(pid)
        if identity and pid not in self.identities: self.identities[pid]=identity
    def collect(self, barrier, state):
        seen=set()
        for path in [*barrier.glob('*.jsonl'),*(state/'hosts').glob('*.json')]:
            try:
                for line in path.read_text().splitlines() if path.suffix=='.jsonl' else [path.read_text()]:
                    try: event=json.loads(line)
                    except ValueError: continue  # writer may be midway through a line
                    for key in ('pid','PID','parent_pid'):
                        pid=event.get(key)
                        if isinstance(pid,int) and pid not in seen:
                            seen.add(pid); self.observe(pid)
            except OSError: pass
        # No system-wide process listing: only direct children of identities
        # already proven to execute inside this invocation's exact private root.
        pending=list(self.identities); visited=set()
        while pending:
            pid=pending.pop()
            if pid in visited or self.identity(pid)!=self.identities[pid]: continue
            visited.add(pid)
            try:
                p=subprocess.run(['pgrep','-P',str(pid)],capture_output=True,text=True,timeout=2)
                for value in p.stdout.split():
                    child=int(value); self.observe(child)
                    if child in self.identities and child not in visited: pending.append(child)
            except (OSError,ValueError,subprocess.TimeoutExpired): pass
    def cleanup(self, barrier, state):
        self.collect(barrier,state)
        def send(sig):
            for pid,identity in list(self.identities.items()):
                # Recheck start time AND exact root-qualified command immediately
                # before every signal, including escalation (PID reuse guard).
                if self.identity(pid)!=identity: continue
                try:
                    os.kill(pid,sig); self.actions.append({'pid':pid,'signal':sig.name})
                except ProcessLookupError: pass
                except PermissionError: self.actions.append({'pid':pid,'skipped':'permission denied'})
        def wait(seconds):
            end=time.monotonic()+seconds
            while time.monotonic()<end:
                if not any(self.identity(pid)==identity for pid,identity in self.identities.items()): return
                time.sleep(.1)
        send(signal.SIGTERM); wait(5)
        # Catch any own child created while the hosts were shutting down.
        self.collect(barrier,state)
        send(signal.SIGTERM); wait(2)
        send(signal.SIGKILL); wait(2)
        return {'identities':list(self.identities.values()),'actions':self.actions,
                'remaining':[pid for pid,identity in self.identities.items() if self.identity(pid)==identity]}

def assert_new_worker(events, pre_pids, boundary, conversation):
    turns=[e for e in events if e['event']=='worker-turn' and e['conversation']==conversation]
    assert turns, 'missing first turn'
    pid=turns[0]['pid']
    assert pid not in pre_pids, 'reused pre-publication worker'
    births=[e for e in events if e['event']=='worker-startup' and e['pid']==pid]
    assert len(births)==1, 'missing unique child-entry startup (StartTurn is not birth)'
    birth=births[0]
    assert boundary < birth['unix_nano'] < turns[0]['unix_nano'], 'startup must follow B publication and precede first turn'
    return birth, turns

def run(repo,evidence):
    assert all(os.environ.get(k)==v for k,v in {"CERCANO_DISABLE_CREDENTIALS":"1","GOPROXY":"off","GOSUMDB":"off"}.items()), "required outer offline/credential guards missing"
    evidence.mkdir(parents=True,exist_ok=True)
    report={'scope':'actual shell -> real publisher -> compiled production host -> compiled CLI PTY','steps':[],'passed':False}
    terminals=[]
    def record(step,**kw):
        report['steps'].append(dict(step=step,**kw)); (evidence/'compiled.json').write_text(json.dumps(report,indent=2)+'\n'); print(step,flush=True)
    with __import__('contextlib').nullcontext(tempfile.mkdtemp(prefix='ca-',dir='/tmp')) as tmp:
        root=P(tmp).resolve(); owned=OwnedProcesses(root); os.chmod(root,0o700); report['artifact_directory']=str(root); state=root/'state'; runtime=root/'r'; runtime.mkdir(mode=0o700)
        copy=root/'repo'; copy.mkdir(); shutil.copytree(repo/'source',copy/'source')
        home=root/'home';home.mkdir(); barrier=root/'barrier';barrier.mkdir(mode=0o700)
        mcp_fixture=barrier/'mcp-fixture'
        mcp_source=(repo/'efforts/dev-host-generations/compiled-mcp-fixture.py').read_text()
        mcp_fixture.write_text('#!'+sys.executable+'\n'+mcp_source.split('\n',1)[1]);mcp_fixture.chmod(0o700)
        go=shutil.which('go'); assert go
        def goenv(key): return subprocess.check_output([go,'env',key],text=True).strip()
        tools=root/'tools';tools.mkdir(); trace=root/'build.jsonl'
        # Bypass /usr/bin/python3's xcrun shim, which injects CPATH after
        # sanitization and is correctly refused by the real publisher.
        (tools/'python3').symlink_to(sys.executable)
        # Build wrapper delegates EVERY command to Go; only adds explicit build tags.
        wrapper=tools/'go'
        wrapper.write_text('#!/usr/bin/env python3\nimport os,sys,json\na=sys.argv[1:]\nif a and a[0]=="build":\n a.insert(1,"-tags=developmentfixture,cercano_development_acceptance")\n with open('+repr(str(trace))+',"a") as f:f.write(json.dumps({"cwd":os.getcwd(),"args":a})+"\\n")\nos.execv('+repr(go)+',['+repr(go)+',*a])\n');wrapper.chmod(0o700)
        env=dict(os.environ)
        for k in list(env):
            if k.startswith(('GO','CGO_')) or k in ('CC','CXX','PKG_CONFIG','SDKROOT','LIBRARY_PATH','CPATH','C_INCLUDE_PATH','CPLUS_INCLUDE_PATH'):env.pop(k)
        env.update(HOME=str(home),XDG_CONFIG_HOME=str(home/'config'),XDG_DATA_HOME=str(home/'data'),XDG_CACHE_HOME=str(home/'cache'),PATH=str(tools)+':'+env['PATH'],GOPATH=goenv('GOPATH'),GOCACHE=goenv('GOCACHE'),GOPROXY='off',GOSUMDB='off',CGO_ENABLED='0',VERSION='compiled-acceptance',TERM='xterm-256color',CERCANO_DISABLE_CREDENTIALS='1',CERCANO_OUTER_ENABLED='0',CERCANO_REPO=str(copy),CERCANO_DEVELOPMENT_STATE=str(state),CERCANO_RUNTIME_ROOT=str(runtime),CERCANO_FIXTURE_BARRIER=str(barrier))
        script=copy/'source/server/scripts/cercano-launcher.sh'
        def call(args,cwd=None,timeout=240):
            p=subprocess.run(list(map(str,args)),cwd=cwd,env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=timeout)
            assert p.returncode==0,p.stdout.decode(errors='replace')
            return p.stdout.decode()
        def selections():return set((state/'selections').glob('*.json'))
        def launch(label):
            before=selections(); t=Terminal(['bash',str(script)],env,evidence/(label+'.pty'));terminals.append(t)
            wait(lambda:len(selections()-before)==1,'selection '+label,240)
            selection=(selections()-before).pop(); s=json.loads(selection.read_text())
            wait(lambda:b'\x1b[?1049h' in t.data or b'FIXTURE' in t.data,'CLI TUI '+label,30)
            time.sleep(2);t.pump(); assert t.p.poll() is None,t.data.decode(errors='replace')
            return t,selection,s
        def wait(pred,label,timeout=30):
            end=time.monotonic()+timeout
            while time.monotonic()<end:
                for t in terminals:t.pump()
                if pred():return
                owned.collect(barrier,state)
                time.sleep(.1)
                for t in terminals:
                    if t.p.poll() is not None: raise AssertionError('launcher exited: '+t.data.decode(errors='replace'))
            raise AssertionError('timeout '+label)
        def control(sel,action,dest=None,success=True):
            args=['bash',str(script),'--dev-host',str(sel),'--dev-host-action',action]
            if dest: args+=['--dev-host-destination',str(dest),'--dev-host-handover','acceptance-'+str(time.time_ns())]
            p=subprocess.run(args,env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=20)
            record('control '+action,exit=p.returncode,output=p.stdout.decode(errors='replace'))
            assert (p.returncode==0)==success,p.stdout.decode(errors='replace')
            return p.stdout.decode()
        def rows(sql):
            # Python's macOS SQLite cannot open a fresh WAL database read-only
            # before -shm exists. rw forbids creation; SQL remains observation only.
            with sqlite3.connect('file:'+str(state/'conversations.db')+'?mode=rw',uri=True) as db:return db.execute(sql).fetchall()
        def worker_events():
            f=barrier/'worker.jsonl'
            return [json.loads(x) for x in f.read_text().splitlines()] if f.exists() else []
        def events():
            f=barrier/'provider.jsonl'
            return [json.loads(x) for x in f.read_text().splitlines()] if f.exists() else []
        try:
            call(['git','init','-q',copy])
            call(['git','add','source'],copy)
            call(['git','-c','user.name=Offline Fixture','-c','user.email=fixture@invalid','commit','-qm','Snapshot actual acceptance inputs'],copy)
            record('fresh source provenance', checkout=subprocess.check_output(['git','rev-parse','HEAD'],cwd=repo,text=True).strip(), snapshot=call(['git','rev-parse','HEAD'],copy).strip(), source_tree=call(['git','rev-parse','HEAD:source'],copy).strip(), fixture_sha256=hashlib.sha256((copy/'source/server/cmd/cercano/development_provider_fixture.go').read_bytes()).hexdigest())
            bootstrap=root/'bootstrap'
            call([wrapper,'build','-o',bootstrap,'./cmd/cercano/'],copy/'source/server')
            settings_client=root/'settings-client'
            call([wrapper,'build','-o',settings_client,'./cmd/compiled-settings-fixture/'],copy/'source/server')
            record('fresh bootstrap artifact', sha256=hashlib.sha256(bootstrap.read_bytes()).hexdigest(), build_info=call([go,'version','-m',bootstrap]))
            # Preparation copies normal config discovery; seed only synthetic HOME.
            synthetic=home/'.config/cercano/config.yaml'
            synthetic.parent.mkdir(parents=True,mode=0o700)
            synthetic.write_text('outer:\n  enabled: false\n');synthetic.chmod(0o600)
            prepared=call([bootstrap,'prepare-development','--new-state',state])
            record('fresh bootstrap preparation',output=prepared,files=[str(x) for x in state.iterdir()])
            cfg=state/'config.yaml';cfg.write_text('open_runtime: llama_server\nlocus_mode: open_only\ntask_assignments:\n  dispatch: {destination: local, quality: standard}\nllama_server:\n  enabled: true\n  default_model: offline-fixture\nmodels:\n  open:\n    overrides:\n      llama_server:\n        everyday: offline-fixture\n        most_capable: offline-fixture\n        fast_light: offline-fixture\n        fast_light_text: offline-fixture\n        embedding: offline-fixture\nouter:\n  enabled: false\n');cfg.chmod(0o600)
            record('explicit tagged real bootstrap and prepared private state', files=[str(x) for x in state.iterdir()], sqlite=rows('pragma user_version'))
            assert rows('pragma user_version')==[(2,)]
            mcp_definition=state/'mcp.json'
            mcp_definition.write_text(json.dumps({'mcpServers':{'fixture':{'command':str(mcp_fixture),'env':{'CERCANO_DISABLE_CREDENTIALS':'1'}}}}));mcp_definition.chmod(0o600)
            mcp_digest=hashlib.sha256(mcp_definition.read_bytes()).hexdigest()
            env['CERCANO_DEVELOPMENT_CONFIG_AUTHORITY']=str(cfg)
            record('one explicit shared A/B configuration authority', config=str(cfg), mcp=str(mcp_definition), mcp_sha256=mcp_digest,
                   exclusions=['not ordinary startup parity', 'no native credentials or loaded runtime', 'no enterprise integration', 'MCP definition mutation unsupported'])
            a,sa,A=launch('A')
            a.send('FIXTURE_ORIGINAL_HISTORY\r')
            wait(lambda:any('FIXTURE_ORIGINAL_HISTORY' in json.dumps(e['request'].get('Messages')) for e in events() if e['event']=='call'),'A initial foreground')
            wait(lambda:b'FIXTURE_OK' in a.data,'A initial response')
            pidA=[e['pid'] for e in events() if e['event']=='call'][0]
            # Retain a second ordinary A conversation without ever sending a turn.
            spare,ss,S=launch('A-unstarted')
            assert S['Identity']['host']==A['Identity']['host']
            assert not any(e.get('conversation')==S['Conversation'] for e in worker_events())
            a.send('FIXTURE_DISPATCH\r')
            wait(lambda:b'wants to run a delegated agent' in a.data,'ordinary dispatch permission prompt')
            a.send('y\r')
            wait(lambda:any(x['event']=='child-entered' for x in events()),'original independent task active before B',40)
            assert rows('select status from delegated_tasks')==[('running',)]
            pre_b_workers=worker_events()
            pre_b_pids=sorted({e['pid'] for e in pre_b_workers})
            record('pre-B worker set', workers=pre_b_workers, pids=pre_b_pids, unstarted_conversation=S['Conversation'])
            change=copy/'source/server/cmd/cercano/development_provider_fixture.go'
            change.write_text(change.read_text()+'\n// generation B source byte change\n')
            b,sb,B=launch('B')
            assert A['Identity']['host']!=B['Identity']['host'] and A['Generation']!=B['Generation']
            assert a.p.poll() is None
            os.kill(pidA,0)
            record('B published and launched without stopping A',A=A,B=B,pidA=pidA)
            # First B turn exercises the ordinary model -> tool registry ->
            # host-owned stdio manager -> synthetic peer -> model result path.
            b.send('FIXTURE_MCP\r')
            wait(lambda:(barrier/'mcp.jsonl').exists() or b'mcp/fixture/echo' in b.data, 'MCP call or permission prompt',40)
            if not (barrier/'mcp.jsonl').exists(): b.send('y')
            wait(lambda:any(e['event']=='mcp-result-request' and 'FIXTURE_MCP_RESULT' in json.dumps(e['request']) for e in events()),'MCP result returned to provider',40)
            assert (barrier/'mcp.jsonl').exists(), 'provider text alone is not MCP execution evidence'
            assert hashlib.sha256(mcp_definition.read_bytes()).hexdigest()==mcp_digest
            record('B real MCP tool path', calls=[json.loads(x) for x in (barrier/'mcp.jsonl').read_text().splitlines()])
            # Same ordinary B session; mutate through the public authenticated
            # client, then observe the real loop's cap instruction in model input.
            mutation=json.loads(call([settings_client,sb,'1'],timeout=20))
            assert mutation['after_loop_limit']==1, mutation
            offset=len(events()); mcp_before=len((barrier/'mcp.jsonl').read_text().splitlines())
            screen_offset=len(b.data)
            b.send('FIXTURE_MCP_SETTINGS_LIMIT\r')
            wait(lambda:b'mcp/fixture/echo' in b.data[screen_offset:] or len((barrier/'mcp.jsonl').read_text().splitlines())>mcp_before,'post-settings MCP prompt or execution',40)
            if len((barrier/'mcp.jsonl').read_text().splitlines())==mcp_before:b.send('y')
            wait(lambda:len((barrier/'mcp.jsonl').read_text().splitlines())>mcp_before,'post-settings MCP execution',40)
            wait(lambda:any("You've reached the 1-step tool limit" in json.dumps(e['request']) for e in events()[offset:] if e['event']=='call'),'changed loop limit reaches actual model request',40)
            affected=[e for e in events()[offset:] if e['event']=='call']
            assert any('FIXTURE_MCP_SETTINGS_LIMIT' in json.dumps(e['request']) for e in affected)
            record('authenticated ordinary UpdateConfig affects subsequent model request',mutation=mutation,calls=affected,
                   exact_behavior='one tool iteration then production loop adds 1-step tool limit final-answer instruction; no DB config injection')
            restored=json.loads(call([settings_client,sb,str(mutation['before_loop_limit'])],timeout=20))
            assert restored['after_loop_limit']==mutation['before_loop_limit']
            record('settings restored through ordinary RPC',mutation=restored)
            published_b_ns=time.time_ns()
            record('B publication observed boundary', unix_nano=published_b_ns, B=B)
            spare.send('FIXTURE_NEW_WORKER_AFTER_B\r')
            wait(lambda:any(e.get('conversation')==S['Conversation'] for e in worker_events()),'new A child first turn after B',40)
            birth,later=assert_new_worker(worker_events(),pre_b_pids,published_b_ns,S['Conversation'])
            record('actual new A child entry after B', startup=birth, first_turn=later[0])
            offset=len(events())
            a.send('FIXTURE_AFTER_B\r')
            wait(lambda:any('FIXTURE_AFTER_B' in json.dumps(e['request'].get('Messages')) for e in events()[offset:] if e['event']=='call'),'A fresh foreground after B')
            assert {e['pid'] for e in events()[offset:] if e['event']=='call'}=={pidA}
            time.sleep(2)
            task=rows('select id,parent_id,status,child_id from delegated_tasks')
            assert len(task)==1 and task[0][2]=='running',task
            manifest_path=P(A['Generation'])/'manifest.json'
            manifest=json.loads(manifest_path.read_text())
            manifest_hash=hashlib.sha256(manifest_path.read_bytes()).hexdigest()
            for e in [birth,*later]:
                assert e['pid']!=pidA and e['parent_pid']==pidA, e
                assert e['unix_nano']>published_b_ns, e
                assert P(e['executable'])==P(A['Generation'])/'cercano', e
                assert e['sha256']==manifest['binaries']['cercano']['sha256'], e
                assert e['manifest_sha256']==manifest_hash, e
                assert manifest['build_id']==A['Identity']['build']
                assert manifest['generation']==P(A['Generation']).name
                # The isolated source copy has no VCS metadata. Exact binary
                # and manifest digests above bind this worker to the source build.
                assert 'path\tcercano/source/server/cmd/cercano' in e['build_info'], e
                assert '-tags=developmentfixture,cercano_development_acceptance' in e['build_info'], e
                assert e['sha256']!=json.loads((P(B['Generation'])/'manifest.json').read_text())['binaries']['cercano']['sha256']
            shutil.copyfile(manifest_path,evidence/'A-manifest.json')
            shutil.copyfile(P(B['Generation'])/'manifest.json',evidence/'B-manifest.json')
            record('fresh A foreground and newly born pinned worker after B',pidA=pidA,tasks=task,workers=later,A_manifest_sha256=manifest_hash,A_build_id=manifest['build_id'],A_generation=manifest['generation'])
            # A client really exits; its host and independent task must survive.
            a.stop();terminals.remove(a);time.sleep(1)
            b.send('/resume '+A['Conversation']+'\r')
            wait(lambda:b'no transfer is scheduled' in b.data,'ordinary busy reopen refusal')
            assert rows('select status from delegated_tasks')==[('running',)]
            retained=json.loads(control(sa,'list'))['Response']['retained']
            assert A['Conversation'] in {r['conversation'] for r in retained}
            assert json.loads(control(sb,'list'))['Response'].get('pending_incoming',0)==0
            record('ordinary busy reopen preserves A task and schedules no transfer')
            (barrier/'child.release').touch()
            wait(lambda:rows('select status from delegated_tasks')==[('completed',)],'independent task completion',40)
            wait(lambda:rows('select notification_pending,wake_claimed from delegated_tasks')==[(0,1)],'exactly one handled completion',40)
            assert len([e for e in events() if e['event']=='completion-notice'])==1
            retained=json.loads(control(sa,'list'))['Response']['retained']
            assert A['Conversation'] in {r['conversation'] for r in retained}
            record('A independent task completed; no deferred transfer',tasks=rows('select id,status,notification_pending,wake_claimed from delegated_tasks'), completion_notices=len([e for e in events() if e['event']=='completion-notice']))
            # Physically restart through the ordinary launcher, never a selection file.
            b.stop();terminals.remove(b)
            reopened,sr,R=launch('B-restarted')
            assert R['Identity']['host']==B['Identity']['host']
            reopened.send('/resume '+A['Conversation']+'\r')
            wait(lambda:A['Conversation'] in {r['conversation'] for r in json.loads(control(sb,'list'))['Response'].get('retained',[])},'original conversation owned by B',40)
            wait(lambda:b'FIXTURE_ORIGINAL_HISTORY' in reopened.data,'original history visible on restarted B',40)
            offset=len(events())
            reopened.send('FIXTURE_REOPENED_ON_B\r')
            wait(lambda:any('FIXTURE_REOPENED_ON_B' in json.dumps(e['request'].get('Messages')) for e in events()[offset:] if e['event']=='call'),'reopened B foreground',40)
            calls=[e for e in events()[offset:] if e['event']=='call']
            assert calls and all(e['pid']!=pidA for e in calls)
            # Foreground inference does not populate Call.ConversationID. Verify
            # the actual durable turn association rather than inventing metadata.
            wait(lambda:rows("select conversation_id from turns where role='user' and content='FIXTURE_REOPENED_ON_B'")==[(A['Conversation'],)],'reopened turn persisted in original conversation')
            os.kill(pidA,0)
            control(sa,'list')
            record('physical client restart and ordinary resume on B with original history; A still alive',calls=calls, original_conversation=A['Conversation'], durable_row_columns=['conversation_id','role','content'], durable_rows=rows("select conversation_id,role,content from turns where role='user' and content in ('FIXTURE_REOPENED_ON_B','FIXTURE_ORIGINAL_HISTORY') order by content"))
            export={'columns':['conversation_id','role','content'], 'rows':rows("select conversation_id,role,content from turns where conversation_id='"+A['Conversation']+"' order by rowid")}
            (evidence/'synthetic-reopened-rows.json').write_text(json.dumps(export,indent=2)+'\n')
            spare.stop();terminals.remove(spare)
            control(ss,'release')
            remaining=json.loads(control(sa,'list'))['Response'].get('retained',[])
            assert not remaining, remaining
            control(sa,'retire')
            def a_exited():
                try: os.kill(pidA,0); return False
                except ProcessLookupError: return True
            wait(a_exited,'explicit A retirement process exit',20)
            offset=len(events())
            reopened.send('FIXTURE_B_AFTER_A_RETIREMENT\r')
            wait(lambda:any('FIXTURE_B_AFTER_A_RETIREMENT' in json.dumps(e['request'].get('Messages')) for e in events()[offset:] if e['event']=='call'),'B fresh model work after A retirement',40)
            wait(lambda:rows("select conversation_id from turns where role='user' and content='FIXTURE_B_AFTER_A_RETIREMENT'")==[(A['Conversation'],)],'post-retirement B durable turn')
            post_reply_sql="select role,content from turns where conversation_id='"+A['Conversation']+"' and role='assistant' and rowid > (select rowid from turns where role='user' and content='FIXTURE_B_AFTER_A_RETIREMENT')"
            wait(lambda:any('FIXTURE_OK' in r[1] for r in rows(post_reply_sql)),'B durable assistant reply after A retirement',40)
            record('explicit A retirement after transfer and release; B continues',calls=events()[offset:], durable_assistant_reply=rows(post_reply_sql))
            report['passed']=True
            record('PASS bounded compiled A/B rehearsal through validated launcher startup', limitations=['synthetic providers and memory credentials', 'not full ordinary-app production acceptance', 'canonical production activation remains denied'])
        except Exception as e:
            report['failure']=str(e);report['traceback']=traceback.format_exc()
            if (state/'conversations.db').exists():
                record('failure task observation',tasks=rows('select id,parent_id,status,child_id,error,result_json from delegated_tasks'))
            raise
        finally:
            owned.collect(barrier,state)
            for t in terminals:
                try:t.stop()
                except Exception:pass
            report['cleanup']=owned.cleanup(barrier,state)
            # Export only test-private logs, after bounded cleanup.
            for name,source in [('mcp.jsonl',barrier/'mcp.jsonl'),('worker.jsonl',barrier/'worker.jsonl'),('provider.jsonl',barrier/'provider.jsonl'),('routing-config.json',barrier/'routing-config.json'),('build.jsonl',trace)]:
                if source.exists():shutil.copyfile(source,evidence/name)
            if state.exists():
                # Never export host token files, even to local evidence.
                record('private token hashes only', tokens=[{'name': f.name, 'sha256': hashlib.sha256(f.read_bytes()).hexdigest()} for f in (state/'hosts').glob('*.token')])
                for h in (state/'hosts').glob('*'):
                    if h.is_file() and h.suffix in ('.log','.json'):shutil.copyfile(h,evidence/h.name)
            (evidence/'compiled.json').write_text(json.dumps(report,indent=2)+'\n')
    assert report['passed']

if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--repo',type=P,required=True);p.add_argument('--evidence',type=P,required=True);a=p.parse_args();run(a.repo.resolve(),a.evidence.resolve())
