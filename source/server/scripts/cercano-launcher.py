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
import time
import tempfile
import shutil


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


def remove_owned_bootstrap(bootstrap):
    """Remove only this invocation's scratch tree, never published state.

    Publisher directories are 0555. Change directory permissions through open
    descriptors, not paths, and never follow symlinks (including the root).
    Files need no chmod; unlinking them requires a writable parent only.
    """
    if not shutil.rmtree.avoids_symlink_attacks:
        raise OSError('safe bootstrap cleanup unavailable')
    for _, _, _, fd in os.fwalk(bootstrap, follow_symlinks=False):
        mode = os.fstat(fd)
        if mode.st_uid != os.getuid():
            raise OSError('bootstrap directory is not owned')
        os.fchmod(fd, stat.S_IMODE(mode.st_mode) | 0o700)
    shutil.rmtree(bootstrap)


def prepare_if_absent(state, repo, env):
    """Serialize first setup outside the exclusively-created destination.

    Never adopt even an empty existing directory. The early Go command owns
    source discovery, config copying and exclusive database provisioning.
    """
    if not state.is_absolute() or state.resolve() != state:
        raise ValueError('state must be canonical and absolute')
    parent = state.parent
    mode = parent.stat()
    if mode.st_uid != os.getuid() or mode.st_mode & 0o022:
        raise ValueError('state parent must be owned and not writable by others')
    fd = os.open(parent / ('.' + state.name + '.setup.lock'),
                 os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, 'r+') as lock:
        mode = os.fstat(lock.fileno())
        if not stat.S_ISREG(mode.st_mode) or mode.st_uid != os.getuid() or mode.st_mode & 0o077 or mode.st_nlink != 1:
            raise ValueError('private owned setup lock required')
        fcntl.flock(lock, fcntl.LOCK_EX)
        if os.path.lexists(state):
            return
        bootstrap = Path(tempfile.mkdtemp(prefix='.' + state.name + '-bootstrap-', dir=parent))
        try:
            root = bootstrap / 'generations'
            publisher = [sys.executable, str(repo / 'source/server/scripts/publish-dev-generation.py')]
            common = ['--repo', str(repo), '--sign']
            build_env = dict(env, CERCANO_DISABLE_CREDENTIALS='1')
            # Do not relay build/preparation output: config bytes and credentials
            # must never escape via a failing child or build diagnostic.
            def run(argv):
                result = subprocess.run(argv, env=build_env, stdout=subprocess.PIPE,
                                        stderr=subprocess.DEVNULL, text=True)
                if result.returncode:
                    raise ValueError('automatic development preparation failed; existing files preserved (check source config and build prerequisites)')
                return result.stdout.strip()
            fingerprint = run([*publisher, 'fingerprint', *common])
            if not fingerprint or Path(fingerprint).name != fingerprint or fingerprint in {'.', '..'}:
                raise ValueError('invalid bootstrap fingerprint')
            generation = run([*publisher, 'publish', *common, '--root', str(root),
                              '--generation', fingerprint, '--fingerprint', fingerprint, '--reuse'])
            if generation != str(root / fingerprint):
                raise ValueError('publisher returned noncanonical bootstrap generation')
            validate_generation(generation, bootstrap)
            run([str(Path(generation) / 'cercano'), 'prepare-development', '--new-state', str(state)])
        finally:
            # Cleanup must not mask preparation errors or turn successful
            # provisioning into a refused launch. Leave failed cleanup intact.
            try:
                remove_owned_bootstrap(bootstrap)
            except OSError:
                print('[cercano] warning: temporary bootstrap cleanup failed; scratch directory retained', file=sys.stderr)


def owned(path, directory=False):
    path = Path(path)
    if not path.is_absolute() or path.resolve() != path:
        raise ValueError('canonical nonsymlink path required: ' + str(path))
    mode = path.lstat()
    if mode.st_uid != os.getuid() or ((not stat.S_ISDIR(mode.st_mode) or mode.st_mode & 0o022) if directory else
                                    not stat.S_ISREG(mode.st_mode) or mode.st_nlink != 1):
        raise ValueError('owned unlinked regular file/directory required: ' + str(path))
    return path


def compatible_store(db):
    def inspect(path):
        try:
            with closing(sqlite3.connect(path.as_uri() + '?mode=ro&immutable=1', uri=True)) as conn:
                return (conn.execute('select contract, schema_version from coexistence_metadata').fetchall()
                        == [('cercano-development-owned-v2', 2)]
                        and conn.execute('pragma user_version').fetchone() == (2,))
        except sqlite3.DatabaseError:
            return False
    # Immutable reads cannot create SHM or recover a journal before consent.
    wal = Path(str(db) + '-wal')
    if not os.path.lexists(wal):
        return inspect(db)
    owned(wal)
    # A disposable inspection copy can read a committed WAL without touching the
    # original store. It is never passed to acquisition or migration.
    with tempfile.TemporaryDirectory(prefix='cercano-inspect-') as scratch:
        copy = Path(scratch) / 'inspect.db'
        for source, target in ((db, copy), (wal, Path(str(copy)+'-wal'))):
            with source.open('rb') as inp, target.open('xb') as out:
                os.fchmod(out.fileno(), 0o600)
                shutil.copyfileobj(inp, out)
        try:
            with closing(sqlite3.connect(copy)) as conn:
                conn.execute('pragma wal_checkpoint(TRUNCATE)')
            return inspect(copy)
        except sqlite3.DatabaseError:
            return False


def confirmed_upgrade(state, db, authority, repo, env):
    print(f'One-time in-place store upgrade for {db}: history and task statuses are preserved; '
          'no reconciliation or replay. A verified private SQLite backup will be retained. '
          'State, database, WAL/SHM and config permissions will be made private.', file=sys.stderr)
    print('Historical agents cannot prove accepted-work drain or stopped writers. '
          'No shutdown will be requested. Let tasks finish, then quit incompatible clients '
          'and agents using their existing controls and retry this SAME command. '
          'Detection covers only same-user known executable names, not custom/renamed '
          'writers or other users. All legacy writers must remain stopped; quit reconnecting '
          'legacy clients too. The shared SDK lock covers only this TMPDIR.', file=sys.stderr)
    if not sys.stdin.isatty():
        raise ValueError('interactive confirmation required; no upgrade performed')
    print('Type STOPPED to confirm accepted work has finished, ALL legacy writers/clients are stopped and will remain '
          'stopped, and authorize backup, private permissions and in-place upgrade: ',
          end='', file=sys.stderr, flush=True)
    if sys.stdin.readline().strip() != 'STOPPED':
        raise ValueError('upgrade declined; database and permissions unchanged')
    # Build away from live binaries, without credential access. The helper retains
    # the production detector; never invoke shutdown or claim it is a writer fence.
    backup = None
    with tempfile.TemporaryDirectory(prefix='cercano-upgrade-') as scratch:
        helper = str(Path(scratch) / 'cercano')
        result = subprocess.run(['go', 'build', '-o', helper, './cmd/cercano/'],
                                cwd=repo / 'source/server',
                                env=dict(env, CERCANO_DISABLE_CREDENTIALS='1'),
                                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        if result.returncode:
            raise ValueError('upgrade helper build failed; store unchanged')
        command = [helper, 'upgrade-store', '--database', str(db), '--all-legacy-agents-stopped']
        def run(args):
            result = subprocess.run(args, env=env, text=True, capture_output=True)
            if result.returncode:
                raise ValueError(result.stderr.strip() or 'offline upgrade helper failed')
        run([*command, '--check-only'])
        # Revalidate before chmod; descriptor-based changes do not follow links.
        paths = [(state, True), (db, False), (authority, False)]
        paths += [(p, False) for p in (Path(str(db)+'-wal'), Path(str(db)+'-shm')) if os.path.lexists(p)]
        descriptors = []
        try:
            for path, directory in paths:
                owned(path, directory)
                fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
                descriptors.append((fd, directory))
                mode = os.fstat(fd)
                if mode.st_uid != os.getuid() or (not stat.S_ISDIR(mode.st_mode) if directory else
                        not stat.S_ISREG(mode.st_mode) or mode.st_nlink != 1):
                    raise ValueError('unsafe store descriptor during upgrade')
                current = path.lstat()
                if (mode.st_dev, mode.st_ino) != (current.st_dev, current.st_ino):
                    raise ValueError('store path changed during upgrade')
            for fd, directory in descriptors:
                os.fchmod(fd, 0o700 if directory else 0o600)
        finally:
            for fd, _ in descriptors:
                os.close(fd)
        fd, name = tempfile.mkstemp(prefix='conversations.before-upgrade-', suffix='.db', dir=state)
        os.close(fd)
        backup = Path(name)
        print('[cercano] retained backup: ' + str(backup), file=sys.stderr)
        try:
            # SQLite online backup includes committed WAL, unlike copying the main file.
            with closing(sqlite3.connect(db.as_uri() + '?mode=ro', uri=True)) as source:
                with closing(sqlite3.connect(backup)) as destination:
                    deadline = time.monotonic() + 30
                    def progress(status, remaining, total):
                        if time.monotonic() > deadline:
                            raise ValueError('backup timed out; writers must remain stopped')
                    source.backup(destination, pages=256, progress=progress)
                    if destination.execute('pragma integrity_check').fetchall() != [('ok',)]:
                        raise ValueError('backup integrity verification failed')
            with backup.open('rb') as saved:
                os.fsync(saved.fileno())
            directory_fd = os.open(state, os.O_RDONLY | os.O_NOFOLLOW)
            try:
                os.fsync(directory_fd)
            finally:
                os.close(directory_fd)
            run(command)
        except Exception as error:
            raise ValueError('upgrade failed; retained backup (may be incomplete if backup failed): '
                             + str(backup) + '; ' + str(error)) from error
    return backup


def main():
    # Same identity as agentclient.acquireAutoLaunchLock (os.TempDir()), taken
    # before setup/runtime/development acquisition locks. CLOEXEC releases at CLI
    # entry, after migration and host acquisition; reconnect cannot spawn mid-upgrade.
    if parse_args(sys.argv[3:])[2]:
        return launch_main()
    path = Path(os.environ.get('TMPDIR') or '/tmp') / 'cercano-agent-launch.lock'
    if not path.is_absolute():
        raise ValueError('SDK launch lock directory must be absolute')
    fd = os.open(path, os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, 'r+') as lock:
        mode = os.fstat(fd)
        if not stat.S_ISREG(mode.st_mode) or mode.st_uid != os.getuid() or mode.st_nlink != 1:
            raise ValueError('unsafe SDK launch lock')
        fcntl.flock(lock, fcntl.LOCK_EX)
        launch_main()


def launch_main():
    selected, args, info = parse_args(sys.argv[3:])
    if info:
        if info == 'version':
            print('cercano development source launcher (binary version requires an explicit generation)')
        else:
            print('Usage: cercano [--dev-host SELECTION] [--dev-host-action ACTION]\nDevelopment only; no legacy discovery. --help / --version never acquire a host.')
        return
    runtime_env = dict(os.environ, CERCANO_LAUNCHED_DEVELOPMENT='1')
    repo = Path(sys.argv[2]).resolve()
    state = Path(os.environ.get('CERCANO_DEVELOPMENT_STATE') or str(Path.home() / '.config/cercano'))
    runtime_env['CERCANO_DEVELOPMENT_STATE'] = str(state)
    # Development host acquisition binds the database to this exact state path.
    # Refuse unsupported overrides before provisioning or opening either store.
    db = state / 'conversations.db'
    override = os.environ.get('CERCANO_CONVERSATIONS_DB')
    if override and override != str(db):
        raise ValueError('CERCANO_CONVERSATIONS_DB conflicts with launcher state; '
                         'only the exact state/conversations.db path is supported. '
                         'Select its state with CERCANO_DEVELOPMENT_STATE or unset the DB override.')
    explicit_authority = os.environ.get('CERCANO_DEVELOPMENT_CONFIG_AUTHORITY')
    if selected is None:
        if explicit_authority and explicit_authority != str(state / 'config.yaml') and not state.exists():
            raise ValueError('explicit config authority requires already prepared isolated state; no ambient provisioning')
        prepare_if_absent(state, repo, runtime_env)
    state = owned(state, directory=True)
    parent_mode = state.parent.stat()
    if parent_mode.st_uid != os.getuid() or parent_mode.st_mode & 0o022:
        raise ValueError('state parent must be owned and not writable by others')
    # Read-only prerequisite check before any build; authoritative Go preflight follows.
    authority = Path(os.environ.get('CERCANO_DEVELOPMENT_CONFIG_AUTHORITY', str(state / 'config.yaml')))
    if not authority.is_absolute() or authority.resolve() != authority or state not in authority.parents:
        raise ValueError('config authority must be canonical and inside isolated prepared state')
    owned(authority.parent, directory=True)
    if db.is_symlink() or not db.is_file() or not authority.is_file():
        raise ValueError('prepared database and explicit config.yaml required')
    for p in (db, authority):
        owned(p)
    compatible = compatible_store(db)
    if not compatible:
        if selected is not None:
            raise ValueError('explicit selection requires compatible store; retry ordinary cercano')
        backup = confirmed_upgrade(state, db, authority, repo, runtime_env)
        print('[cercano] upgrade complete; if startup fails, backup remains at ' + str(backup), file=sys.stderr)
    private(state)
    private(authority.parent)
    for p in (db, authority):
        if p.stat().st_mode & 0o077:
            raise ValueError('compatible database and config must be private')
    # An explicit selection reconnect never builds or creates an attachment here.
    if selected is not None:
        selection_path = selection_file(selected, state)
        selection = object_json(selection_path)
        if selection.get('ConfigAuthority') != str(authority):
            raise ValueError('selection config authority mismatch; refusing reuse')
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
    selection = subprocess.check_output([binary, 'development-launch', '--state', str(state), '--generation', generation, '--runtime-root', str(runtime), '--config-authority', str(authority)], env=runtime_env, text=True).strip()
    selection_file(selection, state)
    validate_generation(generation, state)
    cli = str(Path(generation) / 'cercano-cli')
    os.execve(cli, [cli, '--dev-host', selection, *args], runtime_env)


if __name__ == '__main__':
    try:
        main()
    except (OSError, ValueError, KeyError, IndexError, sqlite3.Error, subprocess.CalledProcessError) as error:
        print('[cercano] launch refused: ' + str(error), file=sys.stderr)
        print('Absent state is prepared automatically from existing config.yaml. Existing incompatible state requires confirmed offline upgrade; no unsafe agent shutdown is attempted.', file=sys.stderr)
        sys.exit(78)
