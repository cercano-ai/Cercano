"""Bounded Phase 4 guest entrypoint; read-only transport probe."""
import os
assert os.getuid() == 501, "guest operations require UID 501"
import hashlib
import json
import pathlib
import subprocess

ROOT = pathlib.Path('/tmp/cnp.h8sjTc')
assert os.environ['HOME'] == str(ROOT / 'home')
assert os.environ['XDG_CONFIG_HOME'] == str(ROOT / 'config')
assert os.environ['GOCACHE'] == str(ROOT / 'gocache')
assert os.environ['GOMODCACHE'] == str(ROOT / 'gomodcache')
assert os.environ['GOPROXY'] == 'off'
assert os.environ['GOSUMDB'] == 'off'
assert os.environ['GOTOOLCHAIN'] == 'local'
print(json.dumps({'uid': os.getuid(), 'script_sha256': hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest(), 'home': os.environ['HOME'], 'xdg': os.environ['XDG_CONFIG_HOME']}, sort_keys=True), flush=True)
for argv in [['/usr/bin/id'], ['/opt/homebrew/bin/go', 'version']]:
    subprocess.run(argv, check=True, timeout=30)
assert (ROOT / 'gomodcache').is_dir()
policy = ROOT / 'config/homebrew/brew.env'
assert 'HOMEBREW_NO_CLEANUP_FORMULAE=cercano-agent' in policy.read_text().splitlines()
print('PASS read-only script transport, UID, isolated environment, cache presence, policy file (not effective-loading proof)', flush=True)
