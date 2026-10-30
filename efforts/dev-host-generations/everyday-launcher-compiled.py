#!/usr/bin/env python3
"""Offline compiled bounded A/B rehearsal; not ordinary-startup parity acceptance."""
import argparse, errno, fcntl, hashlib, json, os, pathlib, pty, select, shlex, shutil, signal, sqlite3, struct, subprocess, sys, tempfile, termios, time, traceback
P=pathlib.Path
# Startup-worker coordination: pass an explicit private config-authority file
# from launcher selection into ordinary host startup (including restart). It
# must stay under this rehearsal's HOME, and UpdateConfig must persist there;
# do not fall back to the operator's default config. This harness intentionally
# does not edit launcher/startup composition or claim its convergence complete.

def run_prepared(command, *, artifact_sha256, timeout, cwd=None):
    """Bounded non-PTY alternative for one explicitly selected prepared role.

    Does not run the compiled rehearsal, provision state, or grant permission.
    Descendant hosts still require the existing OwnedProcesses identity cleanup.
    """
    if not 0 < timeout <= 1800:
        raise ValueError('finite timeout within the existing run window required')
    command.check_execution(artifact_sha256)
    child = subprocess.Popen(command.argv, env=command.env, cwd=cwd,
                             stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                             start_new_session=True)
    try:
        output, _ = child.communicate(timeout=timeout)
        return subprocess.CompletedProcess(command.argv, child.returncode, output)
    except BaseException:
        if child.poll() is None: child.terminate()
        try: child.wait(timeout=2)
        except subprocess.TimeoutExpired:
            child.kill(); child.wait(timeout=2)
        raise
    finally:
        child.stdout.close()


class Terminal:
    def __init__(self, argv, env=None, log=None, *, artifact_sha256=None):
        # Existing offline callers retain their explicit env contract. Prepared
        # commands cannot accept an overlay, even an apparently benign one.
        if hasattr(argv, 'check_execution'):
            if env is not None:
                raise ValueError('prepared env must replace, not overlay, ambient env')
            argv.check_execution(artifact_sha256)
            env=argv.env; argv=argv.argv
        elif artifact_sha256 is not None:
            raise ValueError('artifact checks require a PreparedCommand')
        self.fd, slave=pty.openpty()
        self.closed=False
        try:
            fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',40,140,0,0))
            self.p=subprocess.Popen(argv,stdin=slave,stdout=slave,stderr=slave,env=env,start_new_session=True)
        except BaseException:
            os.close(self.fd)
            raise
        finally:
            os.close(slave)
        self.data=b''; self.log=log
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
        if self.closed: return
        try:
            # Signal/wait only this invocation's unreaped Popen child. Detached
            # hosts are handled separately by OwnedProcesses, never killpg.
            if self.p.poll() is None: self.p.terminate()
            try: self.p.wait(timeout=2)
            except subprocess.TimeoutExpired:
                self.p.kill(); self.p.wait(timeout=2)
        finally:
            os.close(self.fd); self.closed=True

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

def run(repo,evidence,worker_delegation=False,ordinary_closure_probe=False,ordinary_session_close=False,same_conversation_close=False,offline_upgrade=False):
    assert all(os.environ.get(k)==v for k,v in {"CERCANO_DISABLE_CREDENTIALS":"1","GOPROXY":"off","GOSUMDB":"off"}.items()), "required outer offline/credential guards missing"
    evidence.mkdir(parents=True,exist_ok=True)
    report={'scope':'actual shell -> real publisher -> compiled production host -> compiled CLI PTY','steps':[],'passed':False,'worker_delegation_required':worker_delegation}
    terminals=[]
    def record(step,**kw):
        report['steps'].append(dict(step=step,**kw)); (evidence/'compiled.json').write_text(json.dumps(report,indent=2)+'\n'); print(step,flush=True)
    with __import__('contextlib').nullcontext(tempfile.mkdtemp(prefix='ca-',dir='/tmp')) as tmp:
        root=P(tmp).resolve(); owned=OwnedProcesses(root); os.chmod(root,0o700); report['artifact_directory']=str(root); state=root/'state'; runtime=root/'r'; runtime.mkdir(mode=0o700)
        copy=root/'repo'; copy.mkdir(); shutil.copytree(repo/'source',copy/'source',ignore=shutil.ignore_patterns('bin', '__pycache__', '.routing-check*'))
        home=root/'home';home.mkdir(); barrier=root/'barrier';barrier.mkdir(mode=0o700)
        if worker_delegation: (barrier/'worker-delegation.enabled').touch(mode=0o600)
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
        env.update(HOME=str(home),XDG_CONFIG_HOME=str(home/'config'),XDG_DATA_HOME=str(home/'data'),XDG_CACHE_HOME=str(home/'cache'),PATH=str(tools)+':'+env['PATH'],GOPATH=goenv('GOPATH'),GOCACHE=goenv('GOCACHE'),GOPROXY='off',GOSUMDB='off',CGO_ENABLED='0',VERSION='compiled-acceptance',TERM='xterm-256color',CERCANO_DISABLE_CREDENTIALS='1',CERCANO_OUTER_ENABLED='1',CERCANO_REPO=str(copy),CERCANO_DEVELOPMENT_STATE=str(state),CERCANO_RUNTIME_ROOT=str(runtime),CERCANO_FIXTURE_BARRIER=str(barrier))
        # Exercise the exact installed command shape, not the source shell path.
        install_dir=root/'bin'; install_dir.mkdir(mode=0o700)
        script=install_dir/'cercano'
        subprocess.run(['bash',str(copy/'source/server/scripts/install-launcher.sh'),'--destination',str(script)],env=env,check=True)
        report['installed_command']=str(script)
        report['installed_sha256']=hashlib.sha256(script.read_bytes()).hexdigest()
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
        def intentional_quit(t):
            t.send('/quit\r')
            deadline=time.monotonic()+30
            while t.p.poll() is None and time.monotonic()<deadline:
                for terminal in terminals: terminal.pump()
                time.sleep(.05)
            assert t.p.poll()==0, t.data.decode(errors='replace')
            t.pump()
            t.stop()  # Close the PTY even when caller removes it from cleanup.
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
            db=sqlite3.connect('file:'+str(state/'conversations.db')+'?mode=rw',uri=True)
            try: return db.execute(sql).fetchall()
            finally: db.close()
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
            synthetic.write_text('watchdog:\n  enabled: true\n  audit:\n    enabled: true\n');synthetic.chmod(0o600)
            prepared=call([bootstrap,'prepare-development','--new-state',state])
            record('fresh bootstrap preparation',output=prepared,files=[str(x) for x in state.iterdir()])
            if offline_upgrade:
                # Disposable historical schema, not a prepared-store marker rewrite.
                for suffix in ('', '-wal', '-shm'):
                    (state/('conversations.db'+suffix)).unlink(missing_ok=True)
                (state/'conversations.db').touch(mode=0o600,exist_ok=False)
                legacy=sqlite3.connect(state/'conversations.db')
                try:
                    legacy.executescript((copy/'source/server/internal/conversation/schema.sql').read_text())
                    legacy.execute("insert into conversations(id,title,started_at,last_turn_at) values('offline-original','Before upgrade',1,1)")
                    legacy.execute("insert into turns(id,conversation_id,role,content,created_at) values('offline-turn','offline-original','user','FIXTURE_PRE_UPGRADE_HISTORY',1)")
                    legacy.executescript("""CREATE TABLE delegated_tasks (
                        id TEXT PRIMARY KEY, parent_id TEXT NOT NULL, status TEXT NOT NULL,
                        task TEXT NOT NULL, work_dir TEXT NOT NULL, child_id TEXT NOT NULL DEFAULT '',
                        result_json TEXT NOT NULL DEFAULT 'null', error TEXT NOT NULL DEFAULT '',
                        created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
                        notification_pending INTEGER NOT NULL DEFAULT 0, wake_claimed INTEGER NOT NULL DEFAULT 0);""")
                    for status in ('accepted','running','completed'):
                        legacy.execute("insert into delegated_tasks(id,parent_id,status,task,work_dir,created_at,updated_at) values(?,?,?,?,?,1,1)",('offline-'+status,'offline-original',status,'SYNTHETIC_NOT_FOR_REPLAY',''))
                    legacy.commit()
                finally:
                    # A connection context manager commits; it does NOT close.
                    legacy.close()
                historical_tasks=rows("select * from delegated_tasks order by id")
                before_upgrade=(state/'conversations.db').read_bytes()
                refused=subprocess.run([str(script)],env=env,capture_output=True,text=True,timeout=30)
                assert refused.returncode==78 and 'one-time OFFLINE' in refused.stderr and 'upgrade-store --database' in refused.stderr, refused.stderr
                assert (state/'conversations.db').read_bytes()==before_upgrade
                assert not (state/'generations').exists()
                record('installed command refuses legacy schema before build; explicit offline instructions; database byte-identical',output=refused.stderr)
                assert rows('pragma user_version')==[(0,)]
                operation=root/'offline-upgrade-fixture'
                call([wrapper,'build','-o',operation,'./cmd/compiled-offline-upgrade-fixture/'],copy/'source/server')
                # Controlled no-pre-existing-writers condition: private root, closed
                # writer, no hosts launched yet. Never invoke live discovery here.
                upgrade=call([operation,state/'conversations.db'])
                repeated=call([operation,state/'conversations.db'])
                assert rows("select content from turns where id='offline-turn'")==[('FIXTURE_PRE_UPGRADE_HISTORY',)]
                assert rows("select * from delegated_tasks order by id")==historical_tasks
                record('real UpgradeOffline preserved history and task rows; repeat safe',output=upgrade,repeat=repeated,
                       historical_tasks=historical_tasks, production_discovery_end_to_end=False,
                       writer_closed_before_upgrade=True, operation_sha256=hashlib.sha256(operation.read_bytes()).hexdigest())
            cfg=state/'config.yaml';cfg.write_text('open_runtime: llama_server\nlocus_mode: open_only\ntask_assignments:\n  dispatch: {destination: local, quality: standard}\n  review: {destination: local, quality: standard}\nllama_server:\n  enabled: true\n  default_model: offline-fixture\nmodels:\n  open:\n    overrides:\n      llama_server:\n        everyday: offline-fixture\n        most_capable: offline-fixture\n        fast_light: offline-fixture\n        fast_light_text: offline-fixture\n        embedding: offline-fixture\nwatchdog:\n  enabled: true\n  audit:\n    enabled: true\n');cfg.chmod(0o600)
            record('explicit tagged real bootstrap and prepared private state', files=[str(x) for x in state.iterdir()], sqlite=rows('pragma user_version'))
            assert rows('pragma user_version')==[(2,)]
            mcp_definition=state/'mcp.json'
            mcp_definition.write_text(json.dumps({'mcpServers':{'fixture':{'command':str(mcp_fixture),'env':{'CERCANO_DISABLE_CREDENTIALS':'1'}}}}));mcp_definition.chmod(0o600)
            mcp_digest=hashlib.sha256(mcp_definition.read_bytes()).hexdigest()
            env['CERCANO_DEVELOPMENT_CONFIG_AUTHORITY']=str(cfg)
            record('one explicit shared A/B configuration authority', config=str(cfg), mcp=str(mcp_definition), mcp_sha256=mcp_digest,
                   exclusions=['not ordinary startup parity', 'no native credentials or loaded runtime', 'no enterprise integration', 'MCP definition mutation unsupported'])
            # Ordinary startup retains its permissive default; this probe explicitly
            # requests strict permissions; read-only dispatch must remain prompt-free.
            permissions=state/'permissions.yaml';permissions.write_text('mode: strict\n');permissions.chmod(0o600)
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
            wait(lambda:any(x['event']=='child-entered' for x in events()),'original independent task active before B',40)
            assert b'wants to run a delegated agent' not in a.data
            assert rows('select status from delegated_tasks where parent_id != "offline-original"')==[('running',)]
            if worker_delegation:
                assert any(e['event']=='worker-delegation-request' for e in events()), 'host dispatch alone is not worker delegation'
                independent=rows('select child_id from delegated_tasks where parent_id != "offline-original"')[0][0]
                nested=rows("select id,parent_id,kind from conversations where parent_id='"+independent+"'")
                assert len(nested)==1 and nested[0][2]=='subagent', nested
                descendant=nested[0][0]
                assert rows("select parent_id from conversations where id='"+independent+"'")==[(A['Conversation'],)]
                assert rows("select kind from dispatch_events where conversation_id='"+descendant+"' and kind='dispatch_start'")==[('dispatch_start',)]
                record('real worker delegation descendant active', independent=independent, descendant=nested,
                       history=rows("select kind,payload from dispatch_events where conversation_id='"+descendant+"' order by seq"))
            pre_b_workers=worker_events()
            pre_b_pids=sorted({e['pid'] for e in pre_b_workers})
            record('pre-B worker set', workers=pre_b_workers, pids=pre_b_pids, unstarted_conversation=S['Conversation'])
            change=copy/'source/server/cmd/cercano/development_provider_fixture.go'
            change.write_text(change.read_text()+'\n// generation B source byte change\n')
            # Failed B build and startup must not replace the current A target.
            target_a=(state/'current-target.json').read_bytes()
            generation_a={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in P(A['Generation']).iterdir() if p.is_file()}
            original=change.read_text()
            change.write_text(original+'\nthis is deliberately invalid Go\n')
            failed=subprocess.run([str(script)],env=env,capture_output=True,text=True,timeout=240)
            change.write_text(original)
            assert failed.returncode!=0 and (state/'current-target.json').read_bytes()==target_a
            control(sa,'list'); assert a.p.poll() is None
            bad_config=state/'startup-failure.yaml'; bad_config.write_text('[invalid yaml'); bad_config.chmod(0o600)
            failed=subprocess.run([str(script)],env=dict(env,CERCANO_DEVELOPMENT_CONFIG_AUTHORITY=str(bad_config)),capture_output=True,text=True,timeout=240)
            assert failed.returncode!=0 and (state/'current-target.json').read_bytes()==target_a, failed.stdout+failed.stderr
            assert generation_a=={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in P(A['Generation']).iterdir() if p.is_file()}
            control(sa,'list'); assert a.p.poll() is None
            record('installed B build/startup failures preserve reachable A and exact retained binary bytes', startup_failure=failed.stderr)
            # Failed startup deliberately leaves its start intent fail-closed.
            # A corrected candidate has new source identity; never remove the intent.
            change.write_text(original+'\n// corrected B after failed startup candidate\n')
            b,sb,B=launch('B')
            assert A['Identity']['host']!=B['Identity']['host'] and A['Generation']!=B['Generation']
            assert a.p.poll() is None
            os.kill(pidA,0)
            current_path=state/'current-target.json'
            target_b=current_path.read_bytes()
            assert json.loads(target_b)['Identity']==B['Identity']
            control(sa,'list') # exact-A reconnect/control is not target selection
            assert current_path.read_bytes()==target_b
            record('B published as new-session target without stopping A',A=A,B=B,pidA=pidA)
            fresh,sfresh,F=launch('B-normal-new-session')
            assert F['Identity']==B['Identity'] and F['Conversation']!=B['Conversation']
            if ordinary_closure_probe or ordinary_session_close or same_conversation_close:
                intentional_quit(fresh)
            fresh.stop();terminals.remove(fresh)
            if not (ordinary_closure_probe or ordinary_session_close or same_conversation_close): control(sfresh,'release')
            assert json.loads(current_path.read_bytes())['Identity']==B['Identity']
            target_b=current_path.read_bytes()
            record('normal new session chooses B',selection=F)
            if offline_upgrade:
                # Acquire unowned historical history while both hosts are reachable.
                # After A retires, its stale record correctly makes unknown-owner
                # acquisition fail closed; that is not an upgrade failure.
                history,sh,H=launch('B-upgraded-history')
                assert H['Identity']==B['Identity']
                history.send('/resume offline-original\r')
                wait(lambda:b'FIXTURE_PRE_UPGRADE_HISTORY' in history.data,'pre-upgrade history resumed on B',40)
                offset=len(events())
                history.send('FIXTURE_UPGRADED_HISTORY_REPLY\r')
                wait(lambda:rows("select conversation_id from turns where role='user' and content='FIXTURE_UPGRADED_HISTORY_REPLY'")==[('offline-original',)],'fresh B turn in pre-upgrade history',40)
                wait(lambda:any('FIXTURE_OK' in r[0] for r in rows("select content from turns where role='assistant' and conversation_id='offline-original'")),'fresh B reply in pre-upgrade history',40)
                assert all(e['pid']!=pidA for e in events()[offset:] if e['event']=='call')
                intentional_quit(history)
                history.stop();terminals.remove(history)
                assert rows("select * from delegated_tasks where parent_id='offline-original' order by id")==historical_tasks
                record('pre-upgrade original history resumed and fresh B reply persisted')
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
            task=rows('select id,parent_id,status,child_id from delegated_tasks where parent_id != "offline-original"')
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
            assert current_path.read_bytes()==target_b, 'later A worker changed new-session target'
            record('fresh A foreground and newly born pinned worker after B',pidA=pidA,tasks=task,workers=later,A_manifest_sha256=manifest_hash,A_build_id=manifest['build_id'],A_generation=manifest['generation'])
            if ordinary_session_close:
                intentional_quit(spare)
                terminals.remove(spare)
                retained=json.loads(control(sa,'list'))['Response'].get('retained',[])
                assert S['Conversation'] not in {r['conversation'] for r in retained}, retained
                assert rows('select status from delegated_tasks where parent_id != "offline-original"')==[('running',)]
                os.kill(pidA,0)
                record('intentional /quit releases sibling A session while original A independent work survives; not same-conversation drain acceptance')
            # A client really exits; its host and independent task must survive.
            a.stop();terminals.remove(a);time.sleep(1)
            b.send('/resume '+A['Conversation']+'\r')
            wait(lambda:b'no transfer is scheduled' in b.data,'ordinary busy reopen refusal')
            assert rows('select status from delegated_tasks where parent_id != "offline-original"')==[('running',)]
            retained=json.loads(control(sa,'list'))['Response']['retained']
            assert A['Conversation'] in {r['conversation'] for r in retained}
            assert json.loads(control(sb,'list'))['Response'].get('pending_incoming',0)==0
            record('ordinary busy reopen preserves A task and schedules no transfer')
            if worker_delegation:
                # Both explicit management paths must refuse, not merely the UI.
                control(sa,'transfer',sb,success=False)
                control(sa,'retire',success=False)
                assert json.loads(control(sb,'list'))['Response'].get('pending_incoming',0)==0
                record('descendant execution barrier refuses transfer and retirement')
            (barrier/'child.release').touch()
            wait(lambda:rows('select status from delegated_tasks where parent_id != "offline-original"')==[('completed',)],'independent task completion',40)
            if worker_delegation:
                wait(lambda:any(e['event']=='final-entered' for e in events()),'completion callback final-write barrier',40)
                assert any(e['event']=='worker-delegation-result' for e in events()), 'worker did not observe nested tool result'
                history=rows("select kind,payload from dispatch_events where conversation_id='"+descendant+"' order by seq")
                assert sum(k=='dispatch_done' for k,_ in history)==1, history
                assert any('FIXTURE_OK' in payload for _,payload in history), history
                assert rows('select notification_pending,wake_claimed from delegated_tasks where parent_id != "offline-original"')==[(1,1)]
                control(sa,'transfer',sb,success=False)
                control(sa,'retire',success=False)
                assert json.loads(control(sb,'list'))['Response'].get('pending_incoming',0)==0
                record('descendant durable completion before blocked final callback', history=history,
                       ancestry=rows("select id,parent_id,kind from conversations where id in ('"+independent+"','"+descendant+"')"),
                       outcomes=rows('select id,parent_id,status,child_id,error,result_json from delegated_tasks where parent_id != "offline-original"'))
                (barrier/'final.release').touch()
            wait(lambda:rows('select notification_pending,wake_claimed from delegated_tasks where parent_id != "offline-original"')==[(0,1)],'exactly one handled completion',40)
            assert len([e for e in events() if e['event']=='completion-notice'])==1
            retained=json.loads(control(sa,'list'))['Response']['retained']
            assert A['Conversation'] in {r['conversation'] for r in retained}
            record('A independent task completed; no deferred transfer',tasks=rows('select id,status,notification_pending,wake_claimed from delegated_tasks where parent_id != "offline-original"'), completion_notices=len([e for e in events() if e['event']=='completion-notice']))
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
            if not ordinary_session_close:
                os.kill(pidA,0)
                control(sa,'list')
            record('physical client restart and ordinary resume on B with original history',calls=calls, original_conversation=A['Conversation'], durable_row_columns=['conversation_id','role','content'], durable_rows=rows("select conversation_id,role,content from turns where role='user' and content in ('FIXTURE_REOPENED_ON_B','FIXTURE_ORIGINAL_HISTORY') order by content"))
            export={'columns':['conversation_id','role','content'], 'rows':rows("select conversation_id,role,content from turns where conversation_id='"+A['Conversation']+"' order by rowid")}
            (evidence/'synthetic-reopened-rows.json').write_text(json.dumps(export,indent=2)+'\n')
            if same_conversation_close:
                # Preserve the earlier unexpected-disconnect + busy/history reopen
                # scenario. Now use the remaining A conversation for a separate
                # independent task, and quit that EXACT task-owning conversation.
                (barrier/'child.release').unlink()
                offset=len(events()); screen_offset=len(spare.data)
                spare.send('FIXTURE_DISPATCH\r')
                wait(lambda:any(e['event']=='child-entered' for e in events()[offset:]), 'same-conversation independent task barrier',40)
                assert b'wants to run a delegated agent' not in spare.data[screen_offset:]
                task_sql="select id,parent_id,status,child_id from delegated_tasks where parent_id='"+S['Conversation']+"'"
                same_task=rows(task_sql)
                assert len(same_task)==1 and same_task[0][2]=='running', same_task
                intentional_quit(spare); terminals.remove(spare)
                retained=json.loads(control(sa,'list'))['Response'].get('retained',[])
                assert S['Conversation'] in {r['conversation'] for r in retained}, retained
                assert rows(task_sql)==same_task
                ownership_sql="select state from conversation_ownership where conversation_id='"+S['Conversation']+"'"
                assert rows(ownership_sql)==[('owned',)]
                os.kill(pidA,0)
                assert not any(e['event']=='child-released' for e in events()[offset:])
                record('same task-owning A conversation intentionally quit; ownership and running task retained', conversation=S['Conversation'], tasks=same_task, retained=retained)
                (barrier/'child.release').touch()
                wait(lambda:rows(task_sql)[0][2]=='completed', 'same-conversation durable completion',40)
                wait(lambda:rows("select notification_pending,wake_claimed from delegated_tasks where parent_id='"+S['Conversation']+"'")==[(0,1)], 'same-conversation completion callback drained',40)
                assert len([e for e in events()[offset:] if e['event']=='completion-notice'])==1
                record('same-conversation completion drained without ownership management',tasks=rows(task_sql), completion_notices=1)
            elif ordinary_closure_probe:
                intentional_quit(spare)
                terminals.remove(spare)
                report['ordinary_closure_acceptance']='ordinary quit; automatic retirement checked below'
            elif not ordinary_session_close:
                spare.stop();terminals.remove(spare)
                control(ss,'release')
            def a_exited():
                try: os.kill(pidA,0); return False
                except ProcessLookupError: return True
            wait(a_exited,'automatic drained A retirement process exit',20)
            if same_conversation_close:
                assert rows(ownership_sql)==[('released',)]
                assert json.loads(current_path.read_bytes())['Identity']==B['Identity']
                record('same-conversation durable ownership released and superseded A exited', ownership=rows(ownership_sql), pidA=pidA, current_target=B['Identity'])
            offset=len(events())
            reopened.send('FIXTURE_B_AFTER_A_RETIREMENT\r')
            wait(lambda:any('FIXTURE_B_AFTER_A_RETIREMENT' in json.dumps(e['request'].get('Messages')) for e in events()[offset:] if e['event']=='call'),'B fresh model work after A retirement',40)
            wait(lambda:rows("select conversation_id from turns where role='user' and content='FIXTURE_B_AFTER_A_RETIREMENT'")==[(A['Conversation'],)],'post-retirement B durable turn')
            post_reply_sql="select role,content from turns where conversation_id='"+A['Conversation']+"' and role='assistant' and rowid > (select rowid from turns where role='user' and content='FIXTURE_B_AFTER_A_RETIREMENT')"
            wait(lambda:any('FIXTURE_OK' in r[1] for r in rows(post_reply_sql)),'B durable assistant reply after A retirement',40)
            record('automatic A retirement after same-conversation closure drain; B continues' if same_conversation_close else 'automatic A retirement after transfer and release; B continues',calls=events()[offset:], durable_assistant_reply=rows(post_reply_sql))
            if offline_upgrade:
                assert rows("select * from delegated_tasks where parent_id='offline-original' order by id")==historical_tasks
                record('historical task rows remain untouched after A/B drain')

            watchdog_opens=[e for e in events() if e['event']=='watchdog-open']
            assert {pidA, calls[0]['pid']}.issubset({e['pid'] for e in watchdog_opens}), watchdog_opens
            record('ordinary watchdog enabled on A and B with offline audit',opens=watchdog_opens)
            report['passed']=True
            record('PASS bounded compiled A/B rehearsal through validated launcher startup', limitations=['synthetic providers and memory credentials', 'not full ordinary-app production acceptance', 'canonical production activation remains denied'])
        except Exception as e:
            report['failure']=str(e);report['traceback']=traceback.format_exc()
            if (state/'conversations.db').exists():
                try:
                    record('failure task observation',tasks=rows('select id,parent_id,status,child_id,error,result_json from delegated_tasks where parent_id != "offline-original"'))
                except sqlite3.OperationalError as observation_error:
                    record('failure task observation unavailable before initialization',error=str(observation_error))
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
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--repo',type=P,required=True);p.add_argument('--evidence',type=P,required=True);p.add_argument('--worker-delegation',action='store_true',help='require real worker-local review delegation and final-write barrier');p.add_argument('--ordinary-closure-probe',action='store_true',help='check ordinary PTY quit and automatic retirement without management release');p.add_argument('--ordinary-session-close',action='store_true',help='close A session via /quit while independent work runs; contrast unexpected disconnect');p.add_argument('--same-conversation-close',action='store_true',help='preserve disconnect/reopen then quit the exact A conversation owning a second independent task');p.add_argument('--offline-upgrade',action='store_true',help='run real UpgradeOffline on private synthetic history/tasks; excludes production discovery');a=p.parse_args();assert not (a.same_conversation_close and (a.worker_delegation or a.ordinary_session_close or a.ordinary_closure_probe));assert not (a.ordinary_closure_probe and a.ordinary_session_close);run(a.repo.resolve(),a.evidence.resolve(),a.worker_delegation,a.ordinary_closure_probe,a.ordinary_session_close,a.same_conversation_close,a.offline_upgrade)
