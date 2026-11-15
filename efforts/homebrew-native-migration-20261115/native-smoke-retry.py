#!/usr/bin/env python3
"""One-shot clone-only retry; preserve first failure and keep policy unchanged."""
import base64,pathlib,subprocess
H=pathlib.Path(__file__).resolve().parent
R='/opt/cercano-native-smoke-20261115-1357'
b64=base64.b64encode((H/'native-smoke-runtime.py').read_bytes()).decode()
s=f'''set -euo pipefail
R={R}
test ! -e "$R/evidence-tier-retry"
cp "$R/home/.config/cercano/config.yaml" "$R/evidence/config-before-tier-retry.yaml"
cp "$R/home/.config/cercano/conversations.db" "$R/evidence/conversations-before-tier-retry.db"
cat >> "$R/home/.config/cercano/config.yaml" <<'CONFIG'
models:
  open:
    overrides:
      ollama:
        everyday: fixture-smoke
        most_capable: fixture-smoke
        fast_light: fixture-smoke
        fast_light_text: fixture-smoke
CONFIG
echo '{b64}' | base64 -D > "$R/runtime-tier-retry.py"
chmod 444 "$R/runtime-tier-retry.py"
mkdir "$R/evidence-tier-retry"
chown 5501:5501 "$R/evidence-tier-retry"
sudo -u _cercanorehearsal env -i HOME="$R/home" USER=_cercanorehearsal LOGNAME=_cercanorehearsal PATH="$R/brew/bin:/usr/bin:/bin:/usr/sbin:/sbin" TMPDIR="$R/tmp" TERM=xterm-256color CERCANO_DISABLE_CREDENTIALS=1 sandbox-exec -f "$R/safety.sb" /usr/bin/python3 "$R/runtime-tier-retry.py"
'''
subprocess.run(['python3',str(H/'native-smoke.py'),'guest'],input=s,text=True,check=True)
