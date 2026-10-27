#!/usr/bin/env python3
"""Offline compiled production A/B acceptance. Never substitutes a server/CLI."""
import argparse, errno, fcntl, hashlib, json, os, pathlib, pty, select, shutil, signal, sqlite3, struct, subprocess, sys, tempfile, termios, time, traceback
P=pathlib.Path

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

def run(repo,evidence):
    evidence.mkdir(parents=True,exist_ok=True)
    report={'scope':'actual shell -> real publisher -> compiled production host -> compiled CLI PTY','steps':[],'passed':False}
    terminals=[]; hosts=[]
    def record(step,**kw):
        report['steps'].append(dict(step=step,**kw)); (evidence/'compiled.json').write_text(json.dumps(report,indent=2)+'\n'); print(step,flush=True)
    with __import__('contextlib').nullcontext(tempfile.mkdtemp(prefix='ca-',dir='/tmp')) as tmp:
        root=P(tmp).resolve(); os.chmod(root,0o700); report['artifact_directory']=str(root); state=root/'state'; runtime=root/'r'; runtime.mkdir(mode=0o700)
        copy=root/'repo'; copy.mkdir(); shutil.copytree(repo/'source',copy/'source')
        home=root/'home';home.mkdir(); barrier=root/'barrier';barrier.mkdir()
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
                for h in (state/'hosts').glob('*.json'):
                    try:
                        d=json.loads(h.read_text()); pid=d.get('PID') or d.get('pid')
                        if pid and pid not in hosts:hosts.append(pid)
                    except (ValueError,OSError):pass
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
            record('fresh bootstrap artifact', sha256=hashlib.sha256(bootstrap.read_bytes()).hexdigest(), build_info=call([go,'version','-m',bootstrap]))
            prepared=call([bootstrap,'prepare-development','--new-state',state])
            record('fresh bootstrap preparation',output=prepared,files=[str(x) for x in state.iterdir()])
            cfg=state/'config.yaml';cfg.write_text('open_runtime: llama_server\nlocus_mode: open_only\ntask_assignments:\n  dispatch: {destination: local, quality: standard}\nllama_server:\n  enabled: true\n  default_model: offline-fixture\nmodels:\n  open:\n    overrides:\n      llama_server:\n        everyday: offline-fixture\n        most_capable: offline-fixture\n        fast_light: offline-fixture\n        fast_light_text: offline-fixture\n        embedding: offline-fixture\nouter:\n  enabled: false\n');cfg.chmod(0o600)
            record('explicit tagged real bootstrap and prepared private state', files=[str(x) for x in state.iterdir()], sqlite=rows('pragma user_version'))
            assert rows('pragma user_version')==[(2,)]
            a,sa,A=launch('A');record('A acquired via ordinary launcher',selection=A,artifacts=[dict(path=str(f.relative_to(state)),sha256=hashlib.sha256(f.read_bytes()).hexdigest()) for f in (state/'generations').glob('*/cercano*') if f.is_file()])
            a.send('FIXTURE_DISPATCH\r')
            wait(lambda:b'wants to run a delegated agent' in a.data,'ordinary dispatch permission prompt')
            a.send('y\r')
            wait(lambda:any(x['event']=='child-entered' for x in events()),'independent dispatch reached provider barrier',40)
            task=rows('select id,parent_id,status,child_id from delegated_tasks');assert len(task)==1 and task[0][2]=='running',task
            record('real independent dispatch running',tasks=task)
            # A physically disconnects while its independent task remains running.
            a.p.kill();a.p.wait();a.pump();os.close(a.fd);terminals.remove(a)
            builds=trace.read_text();sels=selections(); pidA=[e['pid'] for e in events() if e['event']=='child-entered'][0]
            a=Terminal(['bash',str(script),'--dev-host',str(sa)],env,evidence/'A-reconnect.pty');terminals.append(a)
            wait(lambda:b'\x1b[?1049h' in a.data,'A physical reconnect TUI')
            a.send('FIXTURE_RECONNECTED\r')
            wait(lambda:any('FIXTURE_RECONNECTED' in json.dumps(e['request'].get('Messages')) for e in events() if e['event']=='call'),'A reconnect inference')
            assert trace.read_text()==builds and selections()==sels
            assert {e['pid'] for e in events() if e['event']=='call'}=={pidA}
            assert rows('select status from delegated_tasks')==[('running',)]
            record('A physically killed and reconnected through saved selection; same exact host PID; no publication or acquisition',pid=pidA)
            y,sy,Y=launch('Y');y.stop();terminals.remove(y)
            # Change actual snapshotted source bytes; no factory/runtime substitution.
            change=copy/'source/server/cmd/cercano/development_provider_fixture.go';change.write_text(change.read_text()+'\n// generation B source byte change\n')
            b,sb,B=launch('B');assert A['Identity']['host']!=B['Identity']['host'] and A['Generation']!=B['Generation'];assert a.p.poll() is None
            record('B distinct after source byte change; A CLI and task alive',selection=B,tasks=rows('select id,status from delegated_tasks'))
            c,sc,C=launch('B-reuse');assert C['Identity']['host']==B['Identity']['host'];record('same generation reuses host')
            # Existing selections must not publish or spawn. Management has no attachment.
            builds=trace.read_text(); sels=selections();control(sa,'list');assert trace.read_text()==builds and selections()==sels
            record('saved selection no-build/no-acquisition management reconnect')
            # Two ordinary wrappers race unchanged-source acquisition.
            before=selections();builds=trace.read_text()
            racers=[Terminal(['bash',str(script)],env,evidence/('B-concurrent-'+str(i)+'.pty')) for i in range(2)];terminals.extend(racers)
            wait(lambda:len(selections()-before)==2,'concurrent unchanged-source selections',120)
            concurrent=selections()-before
            assert all(json.loads(s.read_text())['Identity']['host']==B['Identity']['host'] for s in concurrent)
            assert trace.read_text()==builds
            record('concurrent unchanged-source invocations reuse B without rebuild or conflict',selections=[json.loads(s.read_text()) for s in concurrent])
            moved=json.loads(control(sy,'transfer',sb))['Selection'];assert moved and moved['Identity']['host']==B['Identity']['host']
            moved['Generation']=B['Generation']
            sym=state/'selections'/'Y-moved.json';sym.write_text(json.dumps(moved));sym.chmod(0o600)
            assert rows('select status from delegated_tasks')==[('running',)]
            record('unrelated Y transferred A to B while independent A task running',selection=moved)
            # Detach A physically so refusal tests task ownership, not attachment.
            a.p.kill();a.p.wait();a.pump();os.close(a.fd);terminals.remove(a);time.sleep(1)
            pending_before=json.loads(control(sb,'list'))['Response'].get('pending_incoming',0)
            control(sa,'transfer',sb,False)
            pending_after=json.loads(control(sb,'list'))['Response'].get('pending_incoming',0)
            record('busy transfer refused with A task held',pending_before=pending_before,pending_after=pending_after)
            if pending_before != 0 or pending_after != 0:
                record('NEW PRODUCTION FAILURE: refused busy transfer leaks destination reservation',root='agentclient Transfer prepares destination before source relinquish; source failure returns without canceling destination acceptance')
                raise AssertionError('busy transfer refusal leaked destination pending_incoming: '+str(pending_before)+' -> '+str(pending_after))
            # The real Go compiler rejects invalid candidate source while both hosts remain live.
            stable_hosts={f.name:f.read_bytes() for f in (state/'hosts').glob('*') if f.suffix in ('.json','.token')}
            stable_generations={str(f.relative_to(state)):hashlib.sha256(f.read_bytes()).hexdigest() for f in (state/'generations').rglob('*') if f.is_file()}
            stable_selections=selections(); stable_tasks=rows('select * from delegated_tasks')
            original=change.read_bytes();change.write_bytes(original+b'\nvar acceptanceBuildMustFail =\n')
            failed=Terminal(['bash',str(script)],env,evidence/'failed-build.pty')
            end=time.monotonic()+120
            while failed.p.poll() is None and time.monotonic()<end: failed.pump()
            failed.pump();assert failed.p.wait(timeout=5)!=0
            assert b'syntax error' in failed.data and b'development_provider_fixture.go' in failed.data,failed.data
            os.close(failed.fd);change.write_bytes(original)
            assert selections()==stable_selections and rows('select * from delegated_tasks')==stable_tasks
            assert {f.name:f.read_bytes() for f in (state/'hosts').glob('*') if f.suffix in ('.json','.token')}==stable_hosts
            assert {str(f.relative_to(state)):hashlib.sha256(f.read_bytes()).hexdigest() for f in (state/'generations').rglob('*') if f.is_file()}==stable_generations
            control(sa,'list');control(sb,'list')
            record('actual compiler failure before publication preserves live A/B records, generations, selections and running task')
            (barrier/'child.release').touch()
            wait(lambda:rows('select status from delegated_tasks')==[('completed',)],'independent task completion',40)
            wait(lambda:any(x['event']=='child-released' for x in events()),'child release trace')
            record('independent task completed',tasks=rows('select id,status,notification_pending,wake_claimed,result_json,error from delegated_tasks'))
            wait(lambda:rows('select notification_pending,wake_claimed from delegated_tasks')==[(0,1)],'exactly one handled completion',40)
            assert len([e for e in events() if e['event']=='completion-notice'])==1
            time.sleep(2)
            assert len([e for e in events() if e['event']=='completion-notice'])==1
            record('exactly one completion notification delivered and durably acknowledged')
            control(sa,'retire',success=False)
            retained=json.loads(control(sa,'list'))['Response']['retained']
            assert task[0][1] in {r['conversation'] for r in retained}
            assert {r['conversation'] for r in retained}<={task[0][1],rows('select child_id from delegated_tasks')[0][0]}
            for i,r in enumerate(retained):
                release_selection=dict(A,Conversation=r['conversation'],Epoch=r['epoch'])
                release_path=state/'selections'/('A-release-'+str(i)+'.json');release_path.write_text(json.dumps(release_selection));release_path.chmod(0o600)
                control(release_path,'release')
            control(sa,'retire')
            b.pump(); response_offset=len(b.data)
            b.send('FIXTURE_B_STILL_USABLE\r')
            wait(lambda:any('FIXTURE_B_STILL_USABLE' in json.dumps(e['request'].get('Messages')) for e in events() if e['event']=='call'),'B inference after A retirement')
            wait(lambda:b'FIXTURE_OK' in b.data[response_offset:],'B response')
            record('A retained sessions explicitly released and retired; B inference still usable')
            for t in terminals:t.stop()
            terminals.clear();time.sleep(1)
            for sel in [sb,sc,sym,*concurrent]:control(sel,'release')
            control(sb,'retire')
            report['passed']=True
            record('PASS all compiled ordinary-launcher acceptance assertions')
        except Exception as e:
            report['failure']=str(e);report['traceback']=traceback.format_exc()
            if (state/'conversations.db').exists():
                record('failure task observation',tasks=rows('select id,parent_id,status,child_id,error,result_json from delegated_tasks'))
            raise
        finally:
            for t in terminals:
                try:t.stop()
                except Exception:pass
            # Record logs before deleting test-private roots. Only recorded children owned by this invocation.
            for name,source in [('provider.jsonl',barrier/'provider.jsonl'),('routing-config.json',barrier/'routing-config.json'),('build.jsonl',trace)]:
                if source.exists():shutil.copyfile(source,evidence/name)
            if state.exists():
                # Never export host token files, even to local evidence.
                record('private token hashes only', tokens=[{'name': f.name, 'sha256': hashlib.sha256(f.read_bytes()).hexdigest()} for f in (state/'hosts').glob('*.token')])
                for h in (state/'hosts').glob('*'):
                    if h.is_file() and h.suffix in ('.log','.json'):shutil.copyfile(h,evidence/h.name)
            for e in events():
                pid=e.get('pid')
                if pid and pid not in hosts:hosts.append(pid)
            for pid in hosts:
                try:os.kill(pid,signal.SIGTERM)
                except ProcessLookupError:pass
            (evidence/'compiled.json').write_text(json.dumps(report,indent=2)+'\n')
    assert report['passed']

if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--repo',type=P,required=True);p.add_argument('--evidence',type=P,required=True);a=p.parse_args();run(a.repo.resolve(),a.evidence.resolve())
