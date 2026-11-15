#!/usr/bin/env python3
"""Stage and deliberately break NEW clone fixture after convenience replacement."""
import base64,pathlib,subprocess
HERE=pathlib.Path(__file__).resolve().parent
R='/opt/cercano-snapshot-recovery-20261115'
s='set -eu\n'
for name in ('cercano-agent','cercano-cli','cercano'):
 text=(HERE/'formulas'/f'{name}.rb').read_text().replace('https://github.com/cercano-ai/Cercano/releases/download/v0.27.0/',f'file://{R}/artifacts/')
 s+=f"echo '{base64.b64encode(text.encode()).decode()}' | base64 -D > {R}/brew/Library/Taps/migration/homebrew-cercano/Formula/{name}.rb\nchown 5501:5501 {R}/brew/Library/Taps/migration/homebrew-cercano/Formula/{name}.rb\n"
s+=f'''R={R}
"$R/brew-fixture.sh" unlink migration/cercano/cercano
"$R/brew-fixture.sh" install --formula migration/cercano/cercano-agent migration/cercano/cercano-cli
"$R/brew-fixture.sh" upgrade --formula migration/cercano/cercano
"$R/brew-fixture.sh" list --formula --versions
ls -l "$R/brew/bin/"cercano*
test ! -d "$R/brew/Cellar/cercano/0.27.0/bin"
# Deliberately remove only the synthetic component executable after convenience
# replacement. No brew relink or cleanup: restore the entire cold snapshot.
mv "$R/brew/Cellar/cercano-cli/0.27.0/bin/cercano-cli" "$R/brew/Cellar/cercano-cli/0.27.0/bin/cercano-cli.INJECTED-FAILURE"
test ! -e "$R/brew/bin/cercano-cli"
ls -l "$R/brew/bin/"cercano* "$R/brew/Cellar/cercano-cli/0.27.0/bin/"
shasum -a 256 "$R/home/.config/cercano/config.yaml" "$R/home/.cercano/"*
echo 'INJECTED POST-CONVENIENCE CLI UNAVAILABLE; no repair performed'
sync
'''
subprocess.run(['python3',str(HERE/'snapshot-recovery.py'),'guest'],input=s,text=True,check=True)
