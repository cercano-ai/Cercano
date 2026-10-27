#!/usr/bin/env python3
"""Publish canonical signed inputs, then delegate authenticated acquisition to that binary."""
import json
import hashlib
from contextlib import closing
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


def parse_args(args):
    """Normalize Go's one/two-dash and equals spellings before any work.

    Parse the entire argv, including after positionals: Go stops there, which
    must not hide an overriding selector from wrapper preflight.
    """
    values = {}
    positionals = []
    options = {'dev-host', 'dev-host-action', 'dev-host-destination',
               'dev-host-handover', 'dev-host-receipt'}
    info = None
    i = 0
    while i < len(args):
        arg = args[i]
        i += 1
        if not arg.startswith('-'):
            positionals.append(arg)
            continue
        name, equal, value = arg.removeprefix('-').removeprefix('-').partition('=')
        if name in {'h', 'help', 'version'} and not equal:
            if info is not None:
                raise ValueError('duplicate informational flag')
            info = name
            continue
        if name not in options:
            raise ValueError('unsupported launcher flag: ' + arg)
        if not equal:
            if i == len(args) or args[i].startswith('-'):
                raise ValueError('missing value for ' + name)
            value = args[i]
            i += 1
        if not value or name in values:
            raise ValueError('empty or duplicate selector: ' + name)
        values[name] = value
    if info:
        if values or positionals:
            raise ValueError('informational flags must be used alone')
        return None, [], info
    if values and 'dev-host' not in values:
        raise ValueError('management options require explicit --dev-host')
    if 'dev-host-action' in values and values['dev-host-action'] not in {'list', 'acquire', 'release', 'retire', 'transfer', 'claim'}:
        raise ValueError('unsupported host action')
    normalized = [item for name, value in values.items() for item in ('--' + name, value)]
    return values.get('dev-host'), normalized + positionals, None


def object_json(path):
    value = json.loads(path.read_text())
    if not isinstance(value, dict):
        raise ValueError('JSON object required: ' + str(path))
    return value


def validate_generation(generation, state):
    if not isinstance(generation, str) or not generation:
        raise ValueError('selection Generation must be a nonempty string')
    generation = Path(generation)
    if generation.parent != state / 'generations' or generation.resolve() != generation or not generation.is_dir():
        raise ValueError('selection must name an exact canonical generation in this state')
    manifest_path = generation / 'manifest.json'
    if not stat.S_ISREG(manifest_path.lstat().st_mode):
        raise ValueError('regular nonsymlink manifest required')
    manifest = object_json(manifest_path)
    if manifest.get('schema') != 1 or manifest.get('generation') != generation.name or not isinstance(manifest.get('build_id'), str) or not manifest['build_id']:
        raise ValueError('invalid canonical generation manifest')
    binaries = manifest.get('binaries')
    if not isinstance(binaries, dict):
        raise ValueError('manifest binaries object required')
    for name in ('cercano', 'cercano-cli'):
        path = generation / name
        metadata = binaries.get(name)
        mode = path.lstat().st_mode
        if not stat.S_ISREG(mode) or not mode & 0o111 or not os.access(path, os.X_OK):
            raise ValueError('regular nonsymlink executable required: ' + name)
        if not isinstance(metadata, dict) or metadata.get('size') != path.stat().st_size or metadata.get('sha256') != hashlib.sha256(path.read_bytes()).hexdigest():
            raise ValueError('generation hash/size mismatch: ' + name)
    return generation


def main():
    selected, args, info = parse_args(sys.argv[3:])
    if info:
        if info == 'version':
            print('cercano development source launcher (binary version requires an explicit generation)')
        else:
            print('Usage: cercano [--dev-host SELECTION] [--dev-host-action ACTION]\nDevelopment only; no legacy discovery. --help / --version never acquire a host.')
        return
    runtime_env = dict(os.environ, CERCANO_LAUNCHED_DEVELOPMENT='1')
    repo = Path(sys.argv[2]).resolve()
    state = private(os.environ['CERCANO_DEVELOPMENT_STATE'])
    # Read-only prerequisite check before any build; authoritative Go preflight follows.
    db = state / 'conversations.db'
    if db.is_symlink() or not db.is_file() or not (state / 'config.yaml').is_file():
        raise ValueError('prepared database and explicit config.yaml required')
    for p in (db, state / 'config.yaml'):
        s = p.lstat()
        if not stat.S_ISREG(s.st_mode) or s.st_uid != os.getuid() or s.st_mode & 0o077:
            raise ValueError('database and config.yaml must be owned private regular files (0600)')
    with closing(sqlite3.connect(db.as_uri() + '?mode=ro', uri=True)) as conn:
        if conn.execute('select contract, schema_version from coexistence_metadata').fetchall() != [('cercano-development-owned-v2', 2)] or conn.execute('pragma user_version').fetchone() != (2,):
            raise ValueError('unprepared or incompatible store; never implicitly migrated')
    # An explicit selection reconnect never builds or creates an attachment here.
    if selected is not None:
        selection_path = selection_file(selected, state)
        selection = object_json(selection_path)
        generation = validate_generation(selection.get('Generation'), state)
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
    validate_generation(generation, state)
    binary = str(Path(generation) / 'cercano')
    selection = subprocess.check_output([binary, 'development-launch', '--state', str(state), '--generation', generation, '--runtime-root', str(runtime)], env=runtime_env, text=True).strip()
    selection_file(selection, state)
    validate_generation(generation, state)
    cli = str(Path(generation) / 'cercano-cli')
    os.execve(cli, [cli, '--dev-host', selection, *args], runtime_env)


if __name__ == '__main__':
    try:
        main()
    except (OSError, ValueError, KeyError, IndexError, sqlite3.Error, subprocess.CalledProcessError) as error:
        print('[cercano] launch refused: ' + str(error), file=sys.stderr)
        print('Provision once with prepare-development --new-state /absolute/NEW/private/state, which copies existing config.yaml privately; no legacy database or native credential import.', file=sys.stderr)
        sys.exit(78)
