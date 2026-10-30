"""Reuse the existing compiled generation launcher, against the upgraded fixture DB."""
import importlib.util, json, pathlib, shutil, sqlite3, subprocess, sys, time

def accept(repo, root, env, history, result):
    spec = importlib.util.spec_from_file_location('compiled', repo/'efforts/dev-host-generations/everyday-launcher-compiled.py')
    compiled = importlib.util.module_from_spec(spec); spec.loader.exec_module(compiled)
    state=root/'state'; barrier=root/'barrier'; barrier.mkdir(mode=0o700)
    mcp=barrier/'mcp-fixture'
    mcp.write_text('#!'+sys.executable+'\n'+(repo/'efforts/dev-host-generations/compiled-mcp-fixture.py').read_text().split('\n',1)[1]); mcp.chmod(0o700)
    copy=root/'repo'; copy.mkdir()
    shutil.copytree(repo/'source',copy/'source',ignore=shutil.ignore_patterns('bin','__pycache__'))
    tools=root/'tools'; tools.mkdir(); (tools/'python3').symlink_to(sys.executable)
    go='/opt/homebrew/bin/go'
    wrapper=tools/'go'
    wrapper.write_text('#!'+sys.executable+'\nimport os,sys\nos.environ["GOTOOLCHAIN"]="local"\na=sys.argv[1:]\nif a and a[0]=="build": a.insert(1,"-tags=developmentfixture,cercano_development_acceptance")\nos.execv('+repr(go)+',['+repr(go)+',*a])\n'); wrapper.chmod(0o700)
    env=dict(env,PATH=str(tools)+':'+env['PATH'],GOPATH='/Users/bryancostanich/go',GOCACHE='/Users/bryancostanich/Library/Caches/go-build',GOPROXY='off',GOSUMDB='off',CGO_ENABLED='0',TERM='xterm-256color',CERCANO_OUTER_ENABLED='1',CERCANO_REPO=str(copy),CERCANO_DEVELOPMENT_STATE=str(state),CERCANO_RUNTIME_ROOT=str(root/'run'),CERCANO_FIXTURE_BARRIER=str(barrier))
    for key in ('CERCANO_OPEN_RUNTIME','CERCANO_OPEN_MODEL','OLLAMA_URL','CERCANO_PORT'):
        env.pop(key, None)
    # Same tagged synthetic backend/config contract as the existing compiled rehearsal.
    (state/'config.yaml').write_text('open_runtime: llama_server\nlocus_mode: open_only\nllama_server:\n  enabled: true\n  default_model: offline-fixture\nmodels:\n  open:\n    overrides:\n      llama_server:\n        everyday: offline-fixture\n        most_capable: offline-fixture\n        fast_light: offline-fixture\n        fast_light_text: offline-fixture\n        embedding: offline-fixture\n')
    (state/'mcp.json').write_text(json.dumps({'mcpServers':{'fixture':{'command':str(mcp),'env':{'CERCANO_DISABLE_CREDENTIALS':'1'}}}}))
    def call(args):
        p=subprocess.run(args,cwd=copy,env=env,capture_output=True,text=True,timeout=300)
        assert p.returncode==0,p.stdout+p.stderr
        return p.stdout
    call(['git','init','-q']); call(['git','add','source'])
    call(['git','-c','user.name=Fixture','-c','user.email=fixture@invalid','commit','-qm','Isolated source snapshot'])
    result['generation_source']=call(['git','rev-parse','HEAD:source']).strip()
    owned=compiled.OwnedProcesses(root); terminal=None
    def rows(sql,params=()):
        c=sqlite3.connect('file:'+str(state/'conversations.db')+'?mode=rw',uri=True)
        try:return c.execute(sql,params).fetchall()
        finally:c.close()
    def wait(pred,label,timeout=300):
        deadline=time.monotonic()+timeout
        while time.monotonic()<deadline:
            terminal.pump(); owned.collect(barrier,state)
            if pred():return
            assert terminal.p.poll() is None,terminal.data.decode(errors='replace')
            time.sleep(.1)
        raise AssertionError('timeout '+label)
    try:
        terminal=compiled.Terminal(['bash',str(copy/'source/server/scripts/cercano-launcher.sh')],env,root/'generation.pty')
        wait(lambda:b'\x1b[?1049h' in terminal.data,'CLI startup')
        time.sleep(2); terminal.pump()
        selections=list((state/'selections').glob('*.json')); assert len(selections)==1
        result['generation_selection']=json.loads(selections[0].read_text())
        assert result['generation_selection']['Identity']['store']==str(state/'conversations.db')
        assert rows('select * from turns order by id')==history
        conversation=rows("select conversation_id from turns where role='user' order by rowid limit 1")[0][0]
        terminal.send('/resume '+conversation+'\r')
        wait(lambda:b'BEFORE_STOP_REPLY' in terminal.data,'preserved history visible',40)
        terminal.send('FIXTURE_COMBINED_FRESH_REPLY\r')
        wait(lambda:rows("select conversation_id from turns where role='user' and content='FIXTURE_COMBINED_FRESH_REPLY'")==[(conversation,)],'fresh user persisted',40)
        sql="select content from turns where conversation_id=? and role='assistant' and rowid > (select rowid from turns where content='FIXTURE_COMBINED_FRESH_REPLY' and role='user')"
        wait(lambda:any('FIXTURE_OK' in r[0] for r in rows(sql,(conversation,))),'fresh assistant persisted',40)
        current={r[0]:r for r in rows('select * from turns')}
        assert all(current[r[0]]==r for r in history)
        assert rows("select status from delegated_tasks where id='fixture-task'")==[('interrupted',)]
        worker_events=[json.loads(line) for line in (barrier/'worker.jsonl').read_text().splitlines()]
        assert any(e['event']=='worker-turn' and e['conversation']==conversation for e in worker_events)
        result['generation_worker_events']=[{k:v for k,v in e.items() if k!='build_info'} for e in worker_events]
        result['fresh_reply']=rows(sql,(conversation,)); result['history_rows_preserved']=len(history)
        result['replacement_started']=True
    finally:
        if terminal:terminal.stop()
        result['generation_cleanup']=owned.cleanup(barrier,state)
        assert not result['generation_cleanup']['remaining']
