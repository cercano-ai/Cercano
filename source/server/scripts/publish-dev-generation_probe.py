#!/usr/bin/env python3
"""Opt-in Darwin production build/sign probe. Never executes either application.
Uses trusted pre-populated offline caches; no native credential APIs are called.
Usage: python3 ..._probe.py --gopath PATH --gocache PATH
Artifacts and isolated HOME are retained under the printed /tmp directory.
"""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--gopath', required=True)
parser.add_argument('--gocache', required=True)
args = parser.parse_args()
assert sys.platform == 'darwin'
publisher = Path(__file__).with_name('publish-dev-generation.py').resolve()
repo = publisher.parents[3]
go = shutil.which('go')
base = Path(tempfile.mkdtemp(prefix='pub-', dir='/tmp'))
(base / 'home').mkdir()
(base / 'tmp').mkdir()
env = {'PATH': '/opt/homebrew/bin:/usr/local/go/bin:/usr/bin:/bin:/usr/sbin:/sbin',
       'HOME': str(base / 'home'), 'TMPDIR': str(base / 'tmp'),
       'XDG_CONFIG_HOME': str(base / 'home/config'),
       'GOPATH': str(Path(args.gopath).resolve()), 'GOCACHE': str(Path(args.gocache).resolve()),
       'GOPROXY': 'off', 'GOSUMDB': 'off', 'GIT_TERMINAL_PROMPT': '0',
       'GIT_CONFIG_NOSYSTEM': '1', 'GIT_CONFIG_GLOBAL': '/dev/null',
       'SSH_AUTH_SOCK': '', 'VERSION': 'publisher-production-probe'}
print(base, flush=True)
# The publisher policy is also used for package selection; go list only compiles
# package metadata, and neither application is ever run.
spec = importlib.util.spec_from_file_location('publisher', publisher)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
os.environ.clear()
os.environ.update(env)
build_env, _ = module.build_environment()
effective = json.loads(subprocess.check_output([go, 'env', '-json'], env=build_env))
assert effective['GOOS'] == 'darwin' and effective['CGO_ENABLED'] == '1', effective
selection = {}
for package in ('github.com/99designs/keyring', './internal/brewrestart'):
    info = json.loads(subprocess.check_output(
        [go, 'list', '-mod=readonly', '-json', package], cwd=repo / 'source/server', env=build_env))
    selection[package] = info.get('GoFiles', []) + info.get('CgoFiles', [])
assert 'keychain.go' in selection['github.com/99designs/keyring'], selection
assert 'inspect_darwin.go' in selection['./internal/brewrestart'], selection
assert not any('unsupported' in name for name in selection['./internal/brewrestart']), selection
(base / 'selection.json').write_text(json.dumps(selection, indent=2) + '\n')
print(json.dumps(selection, indent=2), flush=True)
subprocess.run([sys.executable, str(publisher), 'publish', '--repo', str(repo),
                '--go', go, '--root', str(base / 'generations'), '--generation', 'native', '--sign'],
               env=env, check=True)
