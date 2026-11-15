#!/usr/bin/env python3
"""Generate clone-only setup from reviewed profile and split-formula fixtures."""
import base64,pathlib,subprocess
H=pathlib.Path(__file__).resolve().parent
R='/opt/cercano-native-smoke-20261115-1357'
s=f'''set -euo pipefail
R={R}
test -d "$R/inspect"
test ! -e "$R/brew"
cp -R /opt/cercano-migration-20261115/brew "$R/brew"
test -z "$(ls "$R/brew/Cellar")"
mkdir -p "$R"/{{cache,logs,home/.config/cercano,home/.cercano}}
rm -rf "$R/brew/Library/Taps/native/homebrew-migration" "$R/brew/Library/Taps/migration/homebrew-cercano"
mkdir -p "$R/brew/Library/Taps/migration/homebrew-cercano/Formula"
'''
def put(path,text):
 global s
 s+=f"echo '{base64.b64encode(text.encode()).decode()}' | base64 -D > '{path}'\n"
put(R+'/safety.sb',(H/'native-smoke.sb').read_text())
for name in ('cercano-agent','cercano-cli','cercano'):
 put(R+'/brew/Library/Taps/migration/homebrew-cercano/Formula/'+name+'.rb',(H/'formulas'/f'{name}.rb').read_text().replace('https://github.com/cercano-ai/Cercano/releases/download/v0.27.0/',f'file://{R}/artifacts/'))
put(R+'/brew-fixture.sh',(H/'unpriv-brew-fixture.sh').read_text().replace('/opt/cercano-migration-unpriv-20261115',R))
put(R+'/home/.config/cercano/config.yaml','''port: "49391"
open_runtime: ollama
ollama_url: http://127.0.0.1:49392
open_model: fixture-smoke
locus_mode: open_only
execution_mode: worker
agent:
  shutdown_on_last_client: false
''')
s+='''chown -R 5501:5501 "$R"
chown root:wheel "$R" "$R/safety.sb" "$R/brew-fixture.sh"
chmod 755 "$R"
chmod 444 "$R/safety.sb"
chmod 555 "$R/brew-fixture.sh"
# Parse/execute only /usr/bin/true before any released program or brew action.
sudo -u _cercanorehearsal sandbox-exec -f "$R/safety.sb" /usr/bin/true
"$R/brew-fixture.sh" trust migration/cercano
"$R/brew-fixture.sh" install --formula migration/cercano/cercano-agent migration/cercano/cercano-cli migration/cercano/cercano
"$R/brew-fixture.sh" list --formula --versions
shasum -a 256 "$R/brew/bin/cercano" "$R/brew/bin/cercano-cli" "$R/safety.sb"
'''
subprocess.run(['python3',str(H/'native-smoke.py'),'guest'],input=s,text=True,check=True)
