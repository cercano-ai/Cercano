#!/bin/bash
set -euo pipefail
R=/opt/cercano-snapshot-recovery-20261115
export R
/usr/bin/python3 - <<'PY'
import os,hashlib,pathlib
r=pathlib.Path(os.environ['R']); b=r/'brew'
checks={'brew/Cellar/cercano/0.25.2/bin/cercano':'effc0cb3c0ec4785b0f2244b872d75f0621249247ca067363c3d49b980c5b24e','brew/Cellar/cercano/0.25.2/bin/cercano-cli':'c94534e942967207dacfa715ad534a3abddf75eb453d3ca75c501d71feee42d2','home/.config/cercano/config.yaml':'ee4cd98070adf3eb294279793c7fa9415e3f1e7743939fd894de9b2326033157','home/.cercano/conversation-marker':'9a9ec9ed1854cc95148fbeb12db10a011503255f7b7408aac9e77826dbb703b3','home/.cercano/credential-marker':'e583c11459158be2080b528872ea2a5595afd886c1e1509d6dd61d2d5bf3e902'}
for p,want in checks.items():
 actual=hashlib.sha256((r/p).read_bytes()).hexdigest(); assert actual==want,(p,actual); print(actual,p,'OK')
for name in ('cercano','cercano-cli'):
 p=b/'bin'/name; assert p.resolve()==b/'Cellar/cercano/0.25.2/bin'/name
 assert p.lstat().st_uid==5501; assert p.resolve().stat().st_uid==5501
 print(name,os.readlink(p),'owner UID5501 OK')
assert sorted(p.name for p in (b/'Cellar').iterdir())==['cercano']
assert sorted(p.name for p in (b/'Cellar/cercano').iterdir())==['0.25.2']
assert '0.25.2' in (b/'Library/Taps/migration/homebrew-cercano/Formula/cercano.rb').read_text()
assert not (b/'Library/Taps/migration/homebrew-cercano/Formula/cercano-cli.rb').exists()
print('Restored historical tap and exclusive monolith Cellar OK')
PY
"$R/brew-fixture.sh" list --formula --versions
for cmd in cercano cercano-cli; do
 /usr/bin/sudo -u _cercanorehearsal /usr/bin/env -i HOME="$R/home" USER=_cercanorehearsal PATH=/usr/bin:/bin TMPDIR="$R/tmp" /usr/bin/sandbox-exec -f "$R/safety.sb" "$R/brew/bin/$cmd" --help
 done
ls -ld "$R" "$R/safety.sb" "$R/brew-fixture.sh"
ps -axo comm= | grep -E '(^|/)(cercano|cercano-cli|cercano-model-host)$' && exit 70 || true
