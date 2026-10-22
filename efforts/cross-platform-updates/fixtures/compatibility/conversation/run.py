#!/usr/bin/env python3
"""TEST ONLY: pinned source experiment, not release qualification. Offline."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import signal
import sqlite3
import subprocess
import tarfile
import tempfile

BASELINE = '58befd2801dd6f3d53438e8c8bfc74d937a9e811'
# Original failing candidate/results remain recorded in VERIFICATION.md/results.txt.
CANDIDATE = 'e9496fb6c35df349ab7850016fce1c67c37e0589'
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


def snapshot(path):
    # All harnesses have closed; immutable avoids observer WAL/SHM writes.
    wal = Path(str(path) + '-wal')
    if wal.exists() and wal.stat().st_size:
        raise RuntimeError('uncheckpointed WAL: refusing incomplete snapshot')
    # URI mode=ro is only for this diagnostic observer, NOT production Open.
    with sqlite3.connect('file:' + str(path) + '?mode=ro&immutable=1', uri=True) as db:
        return list(db.iterdump())


def main():
    global failed
    print('baseline:', BASELINE, 'candidate:', CANDIDATE, flush=True)
    with tempfile.TemporaryDirectory(prefix='cercano-conversation-compat-') as tmp:
        tmp = Path(tmp)
        home = tmp / 'home'
        home.mkdir()
        # An explicitly supplied existing module cache is permitted only as build
        # input. No default HOME/config discovery, credentials, or module network.
        cache = os.environ['COMPAT_GOMODCACHE']
        env = {'PATH': os.environ['PATH'], 'HOME': str(home),
               'XDG_CONFIG_HOME': str(home / 'config'), 'XDG_CACHE_HOME': str(home / 'cache'),
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
            if run(['go', 'build', '-mod=readonly', '-o', str(tmp / label / 'harness'),
                    './cmd/compat-experiment'], module, env, timeout=240):
                return 1
            digest = hashlib.sha256((tmp / label / 'harness').read_bytes()).hexdigest()
            print(label, 'harness sha256:', digest, flush=True)
        for kind in ['empty', 'populated']:
            db = tmp / (kind + '.db')
            def invoke(label, mode):
                return run([str(tmp / label / 'harness'), mode, str(db), kind], tmp, env, timeout=20)
            if invoke('prior', 'seed'):
                continue
            if invoke('prior', 'seed-check'):
                continue
            # Failure is evidence, not a reason to skip the remaining writes/readback.
            invoke('candidate', 'seed-check')
            if invoke('candidate', 'write'):
                continue
            invoke('candidate', 'verify')
            before = snapshot(db)
            before_hash = hashlib.sha256(db.read_bytes()).hexdigest()
            invoke('prior', 'open')
            after = snapshot(db)
            print(kind, 'prior Open logical mutation:', before != after,
                  'file hash changed:', before_hash != hashlib.sha256(db.read_bytes()).hexdigest())
            print('removed dump lines:', json.dumps([x for x in before if x not in after]))
            print('added dump lines:', json.dumps([x for x in after if x not in before]))
            invoke('prior', 'verify')
        for label, module in modules.items():
            run(['go', 'test', '-mod=readonly', './internal/conversation', '-count=1',
                 '-timeout=60s'], module, env, timeout=180)
        print('EXPERIMENT:', 'FAIL (see assertions above)' if failed else 'PASS (bounded scenarios only)')
    return int(failed)


if __name__ == '__main__':
    raise SystemExit(main())
