#!/usr/bin/env python3
"""Disposable-prefix native cases; transport is restricted to owned macOS VM.
No source/guard patches. No existing successful fixture mutation.
"""
import base64,pathlib,sys
from rehearse import guest
HERE=pathlib.Path(__file__).resolve().parent
BASE='/opt/cercano-failure-20261115'
OLD='/opt/cercano-migration-20261115'
GOOD='/opt/cercano-migration-unpriv-20261115'
def encoded(s): return base64.b64encode(s.encode()).decode()
def stage(case,name,text):
 r=f'{BASE}-{case}'
 guest(f"set -eu; echo '{encoded(text)}' | base64 -D > {r}/brew/Library/Taps/migration/homebrew-cercano/Formula/{name}.rb; chown 5501:5501 {r}/brew/Library/Taps/migration/homebrew-cercano/Formula/{name}.rb")
def formula(case,name,version='0.27.0'):
 s=(HERE/'formulas'/f'{name}.rb').read_text()
 s=s.replace('https://github.com/cercano-ai/Cercano/releases/download/v0.27.0/',f'file://{BASE}-{case}/artifacts/')
 if version=='0.25.2':
  s=s.replace('0.27.0','0.25.2').replace('18c68ccc69662da6d18b8017a3eba7eaebc67e057065b90bb66deba2838d2aa0','e57b4ff87d23241588df64e6f106b087775bb6c8939611fc9c540e907efef664')
 return s
if sys.argv[1]=='setup':
 for case in ('conflict','recovery','lag'):
  r=f'{BASE}-{case}'
  wrapper=(HERE/'unpriv-brew-fixture.sh').read_text().replace(GOOD,r)
  guest(f'''set -eu
R={r}; test ! -e "$R"; test -z "$(ls {OLD}/brew/Cellar)"
mkdir -m 755 "$R"
cp -R {OLD}/brew "$R/brew"
mkdir -p "$R"/{{home/.config/cercano,home/.cercano,tmp,cache,logs,artifacts}}
cp {GOOD}/artifacts/*.tar.gz "$R/artifacts/"
rm -rf "$R/brew/Library/Taps/native/homebrew-migration" "$R/brew/Library/Taps/migration/homebrew-cercano"
mkdir -p "$R/brew/Library/Taps/migration/homebrew-cercano/Formula"
sed 's|{GOOD}|{r}|g' {GOOD}/tap/Formula/cercano.rb > "$R/brew/Library/Taps/migration/homebrew-cercano/Formula/cercano.rb"
printf 'port: "49383"\\n' > "$R/home/.config/cercano/config.yaml"
printf 'synthetic-conversation\\n' > "$R/home/.cercano/conversation-marker"
printf 'INERT-NOT-A-CREDENTIAL\\n' > "$R/home/.cercano/credential-marker"
cp {GOOD}/brew/etc/homebrew/brew.env "$R/brew/etc/homebrew/brew.env"
cp {GOOD}/safety.sb "$R/safety.sb"
printf '\\n(deny file-read-data file-write* (subpath "{GOOD}"))\\n' >> "$R/safety.sb"
echo '{encoded(wrapper)}' | base64 -D > "$R/brew-fixture.sh"
chown -R 5501:5501 "$R"; chmod -R go-w "$R"
chown root:wheel "$R" "$R/safety.sb" "$R/brew-fixture.sh"; chmod 755 "$R"; chmod 444 "$R/safety.sb"; chmod 555 "$R/brew-fixture.sh"
cmp {GOOD}/brew/Library/Homebrew/sandbox.rb "$R/brew/Library/Homebrew/sandbox.rb"
cmp {GOOD}/brew/Library/Homebrew/extend/os/mac/sandbox.rb "$R/brew/Library/Homebrew/extend/os/mac/sandbox.rb"
id _cercanorehearsal
shasum -a 256 "$R"/artifacts/*.tar.gz "$R/safety.sb" "$R/home/.config/cercano/config.yaml" "$R/home/.cercano/"*
"$R/brew-fixture.sh" config
''')
  if case!='lag': guest(f'set -eu; {r}/brew-fixture.sh install --formula migration/cercano/cercano; shasum -a 256 {r}/brew/Cellar/cercano/0.25.2/bin/*; ls -l {r}/brew/bin/cercano*')
elif sys.argv[1]=='stage':
 case=sys.argv[2]
 for name in ('cercano-agent','cercano-cli','cercano'): stage(case,name,formula(case,name))
elif sys.argv[1]=='lag-agent': stage('lag','cercano-agent',formula('lag','cercano-agent','0.25.2'))
elif sys.argv[1]=='guest': guest(sys.stdin.read())
else: raise SystemExit('unknown action')
