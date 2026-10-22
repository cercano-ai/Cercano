#!/usr/bin/env python3
"""TEST ONLY: pinned source experiment, not release qualification. Offline."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import signal

import subprocess
import tarfile
import tempfile

BASELINE = '58befd2801dd6f3d53438e8c8bfc74d937a9e811'
CANDIDATE = '0b3f03709e7d0f58e775a8fd2291bb67bfdd8755'
HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[4]
failed = False


def run(args, cwd, env, timeout=120, required=True):
    global failed
    print('$', ' '.join(map(str, args)), flush=True)
    p = subprocess.Popen(args, cwd=cwd, env=env, stdout=subprocess.PIPE,
                         stderr=subprocess.STDOUT, start_new_session=True)
    try:
        out, _ = p.communicate(timeout=timeout)
    except subprocess.TimeoutExpired:
        os.killpg(p.pid, signal.SIGKILL)
        out, _ = p.communicate()
        print('TIMEOUT', timeout)
    print(out.decode(errors='replace'), end='', flush=True)
    print('exit:', p.returncode, flush=True)
    if p.returncode and required:
        failed = True
    return p.returncode


def main():
    global failed
    print('baseline:', BASELINE, 'candidate:', CANDIDATE, flush=True)
    for name in ['run.py', 'harness.go.txt']:
        print(name, 'sha256:', hashlib.sha256((HERE / name).read_bytes()).hexdigest(), flush=True)
    with tempfile.TemporaryDirectory(prefix='cercano-credential-compat-') as tmp:
        tmp = Path(tmp)
        home = tmp / 'home'
        home.mkdir()
        # An explicitly supplied existing module cache is permitted only as build
        # input. No default HOME/config discovery, credentials, or module network.
        cache = os.environ['COMPAT_GOMODCACHE']
        env = {'PATH': os.environ['PATH'], 'HOME': str(home),
               'XDG_CONFIG_HOME': str(home / 'config'), 'XDG_CACHE_HOME': str(home / 'cache'),
               'XDG_DATA_HOME': str(home / 'data'), 'XDG_STATE_HOME': str(home / 'state'),
               'USERPROFILE': str(home), 'APPDATA': str(home / 'appdata'),
               'LOCALAPPDATA': str(home / 'localappdata'),
               'TMPDIR': str(tmp), 'GOENV': 'off', 'GOTOOLCHAIN': 'local',
               'GOPROXY': 'off', 'GOSUMDB': 'off', 'GOWORK': 'off',
               'GOMODCACHE': cache, 'GOCACHE': str(tmp / 'go-build'),
               'GIT_CONFIG_NOSYSTEM': '1', 'GIT_CONFIG_GLOBAL': os.devnull}
        run(['go', 'version'], ROOT, env)
        modules = {}
        for label, commit in [('prior', BASELINE), ('candidate', CANDIDATE)]:
            archive = tmp / (label + '.tar')
            if run(['git', 'archive', '--format=tar', '-o', str(archive), commit,
                    'source/server'], ROOT, env):
                return 1
            print(label, 'source archive sha256:', hashlib.sha256(archive.read_bytes()).hexdigest(), flush=True)
            dest = tmp / label
            dest.mkdir()
            with tarfile.open(archive) as tar:
                # Support the system Python 3.9 without unsafe extractall.
                for member in tar:
                    target = dest / member.name
                    if member.name.startswith('/') or '..' in Path(member.name).parts:
                        raise ValueError('unsafe archive path')
                    if member.isdir():
                        target.mkdir(parents=True, exist_ok=True)
                    elif member.isfile():
                        target.parent.mkdir(parents=True, exist_ok=True)
                        with tar.extractfile(member) as src, target.open('wb') as dst:
                            shutil.copyfileobj(src, dst)
                    else:
                        raise ValueError('non-regular archive member: ' + member.name)
            module = dest / 'source/server'
            modules[label] = module
            harness = module / 'cmd/compat-experiment'
            harness.mkdir()
            shutil.copyfile(HERE / 'harness.go.txt', harness / 'main.go')
            assert (harness / 'main.go').read_bytes() == (HERE / 'harness.go.txt').read_bytes()
            if run(['go', 'build', '-mod=readonly', '-o', str(tmp / label / 'harness'),
                    './cmd/compat-experiment'], module, env, timeout=240):
                return 1
            digest = hashlib.sha256((tmp / label / 'harness').read_bytes()).hexdigest()
            print(label, 'harness sha256:', digest, flush=True)
        for kind in ['empty', 'populated']:
            path = tmp / (kind + '.synthetic-map.json')
            def invoke(label, mode):
                return run([str(tmp / label / 'harness'), mode, str(path), kind], tmp, env, timeout=20)
            if invoke('prior', 'seed'):
                continue
            invoke('prior', 'seed-check')
            invoke('candidate', 'seed-check')
            invoke('candidate', 'write')
            invoke('candidate', 'verify')
            invoke('prior', 'verify')
        # Explicit socket-free allowlist, inspected at both pins. Do not run
        # entire auth/credential suites: other tests open httptest listeners.
        for label, module in modules.items():
            run(['go', 'test', '-mod=readonly', '-v',
                 './internal/secrets', './internal/hostsvc/credentials',
                 './internal/anthropicauth', './internal/chatgptauth',
                 '-run', '^(TestMemoryStoreRoundTrip|TestMissingCredentialHasPortableSentinel|TestLoginNewAttemptOwnsCommit|TestLoginCancellationAndReplacement|TestClosingOldLoginDoesNotCancelNewOne|TestDifferentProfilesHaveIndependentLogins|TestLoginRejectsCanceledContext|TestTokenSet_EncodeDecodeRoundTrip|TestTokenSetRoundTrip)$',
                 '-count=1', '-timeout=60s'], module, env, timeout=180)
        print('EXPERIMENT:', 'FAIL (see assertions above)' if failed else 'PASS (bounded scenarios only)')
    return int(failed)


if __name__ == '__main__':
    raise SystemExit(main())
