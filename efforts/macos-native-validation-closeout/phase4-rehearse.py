#!/usr/bin/env python3
"""Guest-only offline build and actual agent-formula rehearsal. No signal fallback."""
import hashlib, io, json, os, pathlib, shutil, subprocess, sys, tarfile
assert os.getuid() == 501
BASE=pathlib.Path('/tmp/cnp.h8sjTc')
assert os.environ['HOME']==str(BASE/'home') and os.environ['XDG_CONFIG_HOME']==str(BASE/'config')
ROOT=pathlib.Path(__file__).resolve().parents[2]
E=ROOT/'native-evidence'
E.mkdir(exist_ok=True)
def sha(p): return hashlib.sha256(pathlib.Path(p).read_bytes()).hexdigest()
env=dict(PATH='/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin',HOME=str(BASE/'home'),XDG_CONFIG_HOME=str(BASE/'config'),XDG_DATA_HOME=str(BASE/'data'),XDG_CACHE_HOME=str(BASE/'cache'),GOCACHE=str(BASE/'gocache'),GOMODCACHE=str(BASE/'gomodcache'),GOPROXY='off',GOSUMDB='off',GOTOOLCHAIN='local',GOENV='off',GOFLAGS='-p=2',GOMAXPROCS='2',HOMEBREW_NO_AUTO_UPDATE='1',HOMEBREW_NO_INSTALL_FROM_API='1',HOMEBREW_NO_ANALYTICS='1',CERCANO_DISABLE_CREDENTIALS='1')
def run(name,args,cwd=None,extra=None,timeout=300,ok=(0,)):
    print(name,repr(args),flush=True)
    log=E/(name+'.log')
    with log.open('x') as f:
        f.write(repr(args)+'\n');f.flush()
        p=subprocess.run(args,cwd=cwd,env=env| (extra or {}),stdout=f,stderr=subprocess.STDOUT,timeout=timeout)
    print(log.read_text(),flush=True)
    assert p.returncode in ok,(name,p.returncode)
    return log.read_text()
def brew(name,*args): return run(name,['/opt/homebrew/bin/brew',*args])
def policy(name):
    assert 'HOMEBREW_NO_CLEANUP_FORMULAE' not in env
    text=brew(name,'config')
    assert 'HOMEBREW_NO_CLEANUP_FORMULAE: cercano-agent' in text
    assert 'HOMEBREW_NO_INSTALL_CLEANUP' not in text
T=pathlib.Path('/tmp/cnp-homebrew-evidence/tap')
INST=pathlib.Path('/opt/homebrew/Library/Taps/fixture/homebrew-cercano')
def stage(v):
    for path in (T,INST):
        assert not subprocess.check_output(['/usr/bin/git','-C',str(path),'status','--porcelain'],env=env).strip()
    shutil.copyfile(E/('agent-'+v+'.rb'),T/'Formula/cercano-agent.rb')
    run('tap-add-'+v,['/usr/bin/git','-C',str(T),'add','Formula/cercano-agent.rb'])
    run('tap-commit-'+v,['/usr/bin/git','-C',str(T),'-c','user.name=Fixture','-c','user.email=fixture@invalid','commit','-m','phase4-agent-'+v])
    run('tap-pull-'+v,['/usr/bin/git','-C',str(INST),'pull','--ff-only'])
if len(sys.argv)>1 and sys.argv[1]=='upgrade':
    policy('policy-before-B')
    A=pathlib.Path('/opt/homebrew/Cellar/cercano-agent/0.0.905/bin/cercano')
    before=(A.stat().st_dev,A.stat().st_ino,sha(A))
    stage('0.0.906')
    brew('upgrade-B','upgrade','--formula','fixture/cercano/cercano-agent')
    assert (A.stat().st_dev,A.stat().st_ino,sha(A))==before
    cleanup=run('explicit-agent-cleanup',['/opt/homebrew/bin/brew','cleanup','fixture/cercano/cercano-agent'])
    assert 'Refusing to clean' in cleanup
    assert (A.stat().st_dev,A.stat().st_ino,sha(A))==before
    assert pathlib.Path('/opt/homebrew/bin/cercano').resolve()==pathlib.Path('/opt/homebrew/Cellar/cercano-agent/0.0.906/bin/cercano')
    print('A retained device/inode/SHA',before,flush=True)
    sys.exit(0)
manifest=json.loads((ROOT/'phase4-manifest.json').read_text())
for path,digest in manifest['files'].items(): assert sha(ROOT/path)==digest,path
print('ALL SOURCE FILE HASHES VERIFIED',manifest['revision'],flush=True)
policy('policy-before-build')
run('go-version',['/opt/homebrew/bin/go','version'])
brew('initial-packages','list','--versions','cercano','cercano-agent','cercano-cli')
# Both monotonic fixture versions must be absent: never accept a no-op upgrade.
for v in ('0.0.905','0.0.906'):
    assert not pathlib.Path('/opt/homebrew/Cellar/cercano-agent',v).exists(), (
        'fixture already installed: preserve state; no rollback or no-op rehearsal')
source=ROOT/'source/server'
for v in ('0.0.905','0.0.906'):
    binary=E/('cercano-'+v)
    run('build-'+v,['/opt/homebrew/bin/go','build','-tags=','-ldflags=-X main.version='+v,'-o',str(binary),'./cmd/cercano'],cwd=source,timeout=600)
    run('metadata-'+v,['/opt/homebrew/bin/go','version','-m',str(binary)])
    run('version-'+v,[str(binary),'--version'])
    archive=E/('cercano-'+v+'.tar.gz')
    with tarfile.open(archive,'w:gz') as tar:
        tar.add(binary,arcname='fixture/bin/cercano')
        body=b'Disposable current-source agent fixture only.\n'
        entry=tarfile.TarInfo('fixture/README.txt');entry.size=len(body)
        tar.addfile(entry,io.BytesIO(body))
    formula=(T/'Formula/cercano-agent.rb').read_text()
    import re
    formula=re.sub(r'  url ".*"','  url "file://'+str(archive)+'"',formula)
    formula=re.sub(r'  version ".*"','  version "'+v+'"',formula)
    formula=re.sub(r'  sha256 ".*"','  sha256 "'+sha(archive)+'"',formula)
    (E/('agent-'+v+'.rb')).write_text(formula)
    print('ARTIFACT',v,'binary_sha256',sha(binary),'archive_sha256',sha(archive),flush=True)
# Compile controller before any package mutation. Missing deps are a hard stop.
run('compile-controller',['/opt/homebrew/bin/go','test','-c','-o',str(E/'controller.test'),'./cmd/cercano'],cwd=source,timeout=600)
stage('0.0.905')
brew('upgrade-A','upgrade','--formula','fixture/cercano/cercano-agent')
try:
    run('controller',[str(E/'controller.test'),'-test.run=^TestNative(ProductionController|ControllerAssertions)$','-test.v','-test.timeout=360s'],cwd=source/'cmd/cercano',extra=dict(CERCANO_NATIVE_CONTROLLER='1',CERCANO_NATIVE_BREW='1',CERCANO_NATIVE_AGENT='/opt/homebrew/Cellar/cercano-agent/0.0.905/bin/cercano',CERCANO_NATIVE_BREW_HOOK=str(pathlib.Path(__file__).resolve()),CERCANO_NATIVE_STATE_PARENT='/tmp'),timeout=390)
finally:
    brew('residual-packages','list','--versions','cercano','cercano-agent','cercano-cli')
    run('residual-processes',['/bin/ps','-axo','pid=,command='])
