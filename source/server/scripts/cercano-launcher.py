#!/usr/bin/env python3
"""Publish canonical signed inputs, then delegate authenticated acquisition to that binary."""
import json
import fcntl
import os
from pathlib import Path
import sqlite3
import stat
import subprocess
import sys
import tempfile


def private(path):
    p = Path(path)
    if not p.is_absolute() or str(p.resolve()) != str(p):
        raise ValueError('paths must be canonical and absolute')
    s = p.stat()
    if not stat.S_ISDIR(s.st_mode) or s.st_uid != os.getuid() or s.st_mode & 0o077:
        raise ValueError('directory must be private and owned: ' + str(p))
    return p


def selection_file(path, state):
    p = Path(path)
    if p.parent != state / 'selections' or p.resolve() != p:
        raise ValueError('immutable selection in this state required')
    s = p.lstat()
    if not stat.S_ISREG(s.st_mode) or s.st_uid != os.getuid() or s.st_mode & 0o077:
        raise ValueError('private owned selection required')
    return p


def main():
    runtime_env = dict(os.environ)
    repo = Path(sys.argv[2]).resolve()
    args = sys.argv[3:]
    state = private(os.environ['CERCANO_DEVELOPMENT_STATE'])
    # Read-only prerequisite check before any build; authoritative Go preflight follows.
    db = state / 'conversations.db'
    if db.is_symlink() or not db.is_file() or not (state / 'config.yaml').is_file():
        raise ValueError('prepared database and explicit config.yaml required')
    for p in (db, state / 'config.yaml'):
        s = p.lstat()
        if not stat.S_ISREG(s.st_mode) or s.st_uid != os.getuid() or s.st_mode & 0o077:
            raise ValueError('database and config.yaml must be owned private regular files (0600)')
    with sqlite3.connect(db.as_uri() + '?mode=ro', uri=True) as conn:
        if conn.execute('select contract, schema_version from coexistence_metadata').fetchall() != [('cercano-development-owned-v2', 2)] or conn.execute('pragma user_version').fetchone() != (2,):
            raise ValueError('unprepared or incompatible store; never implicitly migrated')
    # An explicit selection reconnect never builds or creates an attachment here.
    if '--dev-host' in args:
        i = args.index('--dev-host')
        selection_path = selection_file(args[i + 1], state)
        selection = json.loads(selection_path.read_text())
        generation = Path(selection['Generation'])
        if generation.parent != state / 'generations' or generation.resolve() != generation:
            raise ValueError('selection must name an exact generation in this state')
        os.execve(str(generation / 'cercano-cli'), [str(generation / 'cercano-cli'), *args], runtime_env)
    runtime = os.environ.get('CERCANO_RUNTIME_ROOT')
    if runtime is None:
        fd = os.open(state / 'runtime-root.lock', os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
        with os.fdopen(fd, 'w') as lock:
            fcntl.flock(lock, fcntl.LOCK_EX)
            record = state / 'runtime-root.json'
            if record.exists():
                if record.is_symlink():
                    raise ValueError('runtime record must not be a symlink')
                runtime = json.loads(record.read_text())
            else:
                runtime = str(Path(tempfile.mkdtemp(prefix='cd-', dir='/tmp')).resolve())
                fd = os.open(record, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
                with os.fdopen(fd, 'w') as f:
                    json.dump(runtime, f)
                    f.flush()
                    os.fsync(f.fileno())
    runtime = private(runtime)
    if len(os.fsencode(runtime)) > 38:
        raise ValueError('runtime-root must be at most 38 resolved bytes')
    root = state / 'generations'
    root.mkdir(mode=0o700, exist_ok=True)
    private(root)
    publisher = [sys.executable, str(repo / 'source/server/scripts/publish-dev-generation.py')]
    common = ['--repo', str(repo), '--sign']
    # Build tooling may not access credentials; runtime policy belongs to the caller.
    build_env = dict(runtime_env, CERCANO_DISABLE_CREDENTIALS='1')
    fingerprint = subprocess.check_output([*publisher, 'fingerprint', *common], env=build_env, text=True).strip()
    generation = subprocess.check_output([*publisher, 'publish', *common, '--root', str(root), '--generation', fingerprint, '--fingerprint', fingerprint, '--reuse'], env=build_env, text=True).strip()
    if generation != str(root / fingerprint):
        raise ValueError('publisher returned noncanonical generation')
    binary = str(Path(generation) / 'cercano')
    selection = subprocess.check_output([binary, 'development-launch', '--state', str(state), '--generation', generation, '--runtime-root', str(runtime)], env=runtime_env, text=True).strip()
    selection_file(selection, state)
    cli = str(Path(generation) / 'cercano-cli')
    os.execve(cli, [cli, '--dev-host', selection, *args], runtime_env)


if __name__ == '__main__':
    try:
        main()
    except (OSError, ValueError, KeyError, IndexError, sqlite3.Error, subprocess.CalledProcessError) as error:
        print('[cercano] launch refused: ' + str(error), file=sys.stderr)
        print('Provision once with prepare-development --new-state /absolute/NEW/private/state, then provide explicit config.yaml; no legacy import or native credentials.', file=sys.stderr)
        sys.exit(78)
