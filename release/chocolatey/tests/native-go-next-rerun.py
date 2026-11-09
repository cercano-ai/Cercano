#!/usr/bin/env python3
"""Single native fixture rerun, reusing hash-verified clone artifacts. No build/download."""
import datetime, hashlib, importlib, json, pathlib, sys
sys.path.insert(0,str(pathlib.Path(__file__).parent))
h=importlib.import_module('native-go-fix-host')
build=json.loads((h.OUT/'native-go-build.json').read_text())
source=(h.ROOT/'release/chocolatey/tests/native-go-fixture.go').read_bytes()
assert hashlib.sha256(source).hexdigest()==build['source_sha256']
run='C:\\CercanoRunner\\phase1\\native-go-'+datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%d%H%M%S')
old=r'C:\CercanoRunner\phase1\native-go-20261106201208'
meta={'run':run,'source_sha256':hashlib.sha256(source).hexdigest(),'known_artifacts':build['artifacts']}
(h.OUT/'native-go-next-run.json').write_text(json.dumps(meta,indent=2))
setup="$ErrorActionPreference='Stop'; $r='"+run+"'; if(Test-Path $r){throw 'Run already exists'}; New-Item -ItemType Directory $r|Out-Null;"
for version,artifact in build['artifacts'].items():
 setup+=f"$s='{old}\\native-go-{version}.exe';if((Get-FileHash $s).Hash -cne '{artifact['sha256']}'){{throw 'Artifact mismatch'}};Copy-Item $s $r;"
r=h.ps(setup,20)
(h.OUT/'native-go-next-setup.json').write_text(json.dumps(r,indent=2))
assert r['exit']==0,r
for name in ('native-go-probe.ps1','native-go-plain-json.ps1'):
 h.upload(run+'\\'+name,h.ROOT/'release/chocolatey/tests'/name)
r=h.ps("& '"+run+"\\native-go-probe.ps1' -Run '"+run+"'",600)
(h.OUT/'native-go-next-execution.json').write_text(json.dumps(r,indent=2))
print(json.dumps(r,indent=2))
sys.exit(0 if r['exit']==0 else 1)
