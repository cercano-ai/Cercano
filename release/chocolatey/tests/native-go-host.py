#!/usr/bin/env python3
"""Bounded clone-only native fixture build/transport; no product build/download."""
import base64, hashlib, json, os, pathlib, subprocess, tempfile, datetime
ROOT=pathlib.Path(__file__).resolve().parents[3]
OUT=ROOT/'efforts/cross-platform-updates'
RUN='native-go-'+datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%d%H%M%S')
GUEST='C:\\CercanoRunner\\phase1\\'+RUN
PS=r'C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe'
ARGV=['prlctl','exec','{SANITIZED_VM_2}',PS,'-NoProfile','-NonInteractive','-ExecutionPolicy','Bypass']
records=[]
def call(label,args,**kw):
 r=subprocess.run(args,stdout=subprocess.PIPE,stderr=subprocess.PIPE,**kw)
 records.append(dict(label=label,argv=args,exit=r.returncode,stdout=r.stdout.decode('utf-8',errors='replace'),stderr=r.stderr.decode('utf-8',errors='replace')))
 (OUT/'native-go-host-transcript.json').write_text(json.dumps(dict(run=RUN,guest=GUEST,records=records),indent=2))
 if r.returncode: raise RuntimeError(f'{label} exit {r.returncode}: {r.stdout!r} {r.stderr!r}')
 return r.stdout

def ps(label,text,data=None,timeout=60):
 return call(label,ARGV+['-EncodedCommand',base64.b64encode(text.encode('utf-16le')).decode()],input=data,timeout=timeout)
def upload(name,data):
 sha=hashlib.sha256(data).hexdigest().upper()
 ps('upload-'+name, "$ErrorActionPreference='Stop'; $b=[Convert]::FromBase64String([Console]::In.ReadToEnd()); [IO.File]::WriteAllBytes('"+GUEST+'\\'+name+"',$b); $h=(Get-FileHash -LiteralPath '"+GUEST+'\\'+name+"').Hash; if($h -ne '"+sha+"'){throw 'Transport hash mismatch'}; Write-Output $h",base64.b64encode(data))
 return sha

def main():
 source=ROOT/'release/chocolatey/tests/native-go-fixture.go'
 manifest={'run':RUN,'source_sha256':hashlib.sha256(source.read_bytes()).hexdigest(),'source':source.read_text(),'environment':{'CGO_ENABLED':'0','GOOS':'windows','GOARCH':'arm64','GOTOOLCHAIN':'local','GOPROXY':'off','GOSUMDB':'off'},'artifacts':{}}
 manifest['compiler']=call('compiler',['go','version']).decode().strip()
 manifest['compiler_environment']=call('compiler-environment',['go','env','GOVERSION','GOROOT','GOTOOLCHAIN']).decode()
 ps('fresh-directory',"$ErrorActionPreference='Stop'; if(Test-Path '"+GUEST+"'){throw 'Fresh run required'}; New-Item -ItemType Directory '"+GUEST+"' | Out-Null")
 with tempfile.TemporaryDirectory(prefix='native-go-') as temp:
  env=os.environ.copy();env.update(manifest['environment']);env['GOCACHE']=temp+'/cache';env['GO111MODULE']='off'
  for version in ['1.0.0','2.0.0']:
   target=pathlib.Path(temp)/('native-go-'+version+'.exe')
   call('build-'+version,['go','build','-trimpath','-ldflags=-s -w -X main.version='+version,'-o',str(target),str(source)],env=env,timeout=180)
   manifest['artifacts'][version]={'sha256':upload(target.name,target.read_bytes()),'length':target.stat().st_size,'go_build_info':call('build-info-'+version,['go','version','-m',str(target)]).decode()}
  upload('native-go-fixture.go',source.read_bytes())
  script=(ROOT/'release/chocolatey/tests/native-go-probe.ps1').read_bytes()
  upload('native-go-probe.ps1',script)
  (OUT/'native-go-build.json').write_text(json.dumps(manifest,indent=2))
  try:
   call('probe',ARGV+['-File',GUEST+'\\native-go-probe.ps1','-Run',GUEST],timeout=900)
  finally:
   # Preserve full transcript and all fixture-owned textual evidence, including failure paths.
   raw=ps('collect',"$r='"+GUEST+"'; $x=@(Get-ChildItem -LiteralPath $r -File -Recurse | Where-Object {$_.Extension -in '.json','.txt','.log','.ps1','.go','.csv','.nuspec'} | ForEach-Object { [pscustomobject]@{path=$_.FullName;content=[IO.File]::ReadAllText($_.FullName)} }); ConvertTo-Json -InputObject $x -Depth 12",timeout=90)
   (OUT/'native-go-evidence.json').write_bytes(raw)
 print(RUN)
if __name__=='__main__': main()
