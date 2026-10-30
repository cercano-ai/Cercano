"""Offline, isolated build/test driver. Never inherits provider credentials."""
import json, os, pathlib, subprocess, sys, tempfile
base = pathlib.Path(__file__).resolve().parent
repo = base.parents[2]
root = pathlib.Path(tempfile.mkdtemp(prefix='upgrade-build-'))
env = {'PATH':'/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin',
       'HOME':str(root), 'TMPDIR':str(root), 'CERCANO_DISABLE_CREDENTIALS':'1',
       'GOPATH':'/Users/bryancostanich/go', 'GOMODCACHE':'/Users/bryancostanich/go/pkg/mod',
       'GOCACHE':'/Users/bryancostanich/Library/Caches/go-build',
       'GOROOT':'/opt/homebrew/Cellar/go/1.26.3/libexec',
       'GOPROXY':'off','GOSUMDB':'off','GOTOOLCHAIN':'local'}
for k in ['CONFIG','RUNTIME','STATE','CACHE','DATA']:
    p=root/k.lower(); p.mkdir(); env['XDG_'+k+'_HOME' if k!='RUNTIME' else 'XDG_RUNTIME_DIR']=str(p)
(root/'env.json').write_text(json.dumps(env,indent=2))
commands = [sys.argv[1:]] if len(sys.argv)>1 else [
 ['build','-o',str(base/'cercano'),'./cmd/cercano'],
 ['build','-tags=ordinary_upgrade_acceptance','-o',str(base/'cercano-upgrade'),'./cmd/cercano'],
 ['build','-o',str(base/'client'),str(base/'client.go')],
 ['build','-tags=upgradefixture','-o',str(base/'upgrade-fixture'),'./cmd/upgrade-fixture']]
print('BUILD_ROOT',root,flush=True)
for args in commands:
    r=subprocess.run(['/opt/homebrew/bin/go']+args,cwd=repo/'source/server',env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
    print(r.stdout,flush=True)
    with (root/'output.log').open('a') as f: f.write(repr(args)+'\n'+r.stdout)
    if r.returncode: sys.exit(r.returncode)
