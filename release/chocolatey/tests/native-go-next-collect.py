#!/usr/bin/env python3
import importlib,json,pathlib,sys
sys.path.insert(0,str(pathlib.Path(__file__).parent));h=importlib.import_module('native-go-fix-host')
run=json.loads((h.OUT/'native-go-next-run.json').read_text())['run']
text="$ErrorActionPreference='Stop';$r='"+run+"';$files=@{}; Get-ChildItem $r -Recurse -File | Where-Object { $_.Extension -in '.json','.jsonl','.txt','.log','.ps1','.nuspec' } | ForEach-Object {$files[$_.FullName.Substring($r.Length+1)]=[IO.File]::ReadAllText($_.FullName)};"
text+="[pscustomobject]@{files=$files;processes=@(Get-CimInstance Win32_Process|Where-Object {$_.Name -eq 'choco.exe' -or $_.ExecutablePath -like '*cercano-native-go-*' -or $_.CommandLine -like ('*'+$r+'*')}|Select-Object ProcessId,ExecutablePath,CommandLine);metadata=@(& 'C:\\ProgramData\\chocolatey\\bin\\choco.exe' list --limit-output)}|ConvertTo-Json -Depth 5"
r=h.ps(text,30)
(h.OUT/'native-go-next-collection-transport.json').write_text(json.dumps(r,indent=2))
assert r['exit']==0,r
value=json.loads(r['stdout']);(h.OUT/'native-go-next-evidence.json').write_text(json.dumps(value,indent=2))
for name in ('native-go-status.txt','native-go-failure.txt','native-go-results.json','running/native-go-process.txt','running/native-go-control.ready'):
 print(name, value['files'].get(name))
print('processes',value['processes']);print('metadata',value['metadata'])
