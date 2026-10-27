#!/usr/bin/env python3
"""Safety checks only; NOT production generation-launch acceptance."""
from pathlib import Path
import os
import subprocess
import tempfile
import unittest
import sqlite3
import json
import shutil
import concurrent.futures
from contextlib import closing

SCRIPTS = Path(__file__).resolve().parent


class LauncherSafety(unittest.TestCase):
    def test_blocked_launcher_does_not_execute_external_commands(self):
        with tempfile.TemporaryDirectory() as root:
            # cat is the only external dependency in the refusal path.
            bin_dir = Path(root) / 'bin'
            bin_dir.mkdir()
            (bin_dir / 'cat').symlink_to('/bin/cat')
            result = subprocess.run(['/bin/bash', str(SCRIPTS / 'cercano-launcher.sh')],
                                    env={'HOME': root, 'PATH': str(bin_dir)}, capture_output=True)
            self.assertEqual(result.returncode, 78, result.stderr)
            self.assertIn(b'No database or running session has been changed', result.stderr)
            self.assertEqual(sorted(p.name for p in Path(root).iterdir()), ['bin'])

    def fixture(self, root):
        root = Path(root).resolve()
        state = root / 'state'
        state.mkdir(mode=0o700)
        (state / 'config.yaml').write_text('{}')
        with closing(sqlite3.connect(state / 'conversations.db')) as db, db:
            db.execute('create table coexistence_metadata(contract, schema_version)')
            db.execute("insert into coexistence_metadata values ('cercano-development-owned-v2', 2)")
            db.execute('pragma user_version=2')
        (state / 'config.yaml').chmod(0o600)
        (state / 'conversations.db').chmod(0o600)
        scripts = root / 'repo/source/server/scripts'
        scripts.mkdir(parents=True)
        shutil.copy(SCRIPTS / 'cercano-launcher.py', scripts)
        # Fake publisher and generation binaries; the actual source shell and
        # orchestration helper execute, never a replacement launcher.
        (scripts / 'publish-dev-generation.py').write_text('''import sys, pathlib, os, fcntl, json, hashlib
with open(os.environ['FAKE_ENV_LOG'], 'a') as log:
 log.write(json.dumps([sys.argv[1], os.environ.get('CERCANO_DISABLE_CREDENTIALS')])+'\\n')
assert os.environ['CERCANO_DISABLE_CREDENTIALS']=='1'
assert '--sign' in sys.argv
state=pathlib.Path(os.environ['CERCANO_DEVELOPMENT_STATE'])
if sys.argv[1]=='fingerprint': print('abc'); sys.exit()
assert '--reuse' in sys.argv and '--fingerprint' in sys.argv
gen=state/'generations/abc'
with (state/'publish.lock').open('w') as lock:
 fcntl.flock(lock, fcntl.LOCK_EX)
 gen.mkdir(exist_ok=True)
 for name in ['cercano', 'cercano-cli']:
  target=gen/name
  if not target.exists():
   target.write_bytes((state/name).read_bytes()); target.chmod(0o755)
 manifest={'schema':1, 'generation':'abc', 'build_id':'fake', 'binaries':{name:{'sha256':hashlib.sha256((gen/name).read_bytes()).hexdigest(), 'size':(gen/name).stat().st_size} for name in ['cercano', 'cercano-cli']}}
 if not (gen/'manifest.json').exists(): (gen/'manifest.json').write_text(json.dumps(manifest))
print(gen)
''')
        (state / 'cercano').write_text('''#!/usr/bin/env python3
import sys, os, pathlib, tempfile, json
assert sys.argv[1]=='development-launch'
with open(os.environ['FAKE_ENV_LOG'], 'a') as log:
 log.write(json.dumps(['development-launch', os.environ.get('CERCANO_DISABLE_CREDENTIALS')])+'\\n')
state=pathlib.Path(sys.argv[sys.argv.index('--state')+1])
assert pathlib.Path(sys.argv[sys.argv.index('--generation')+1])==pathlib.Path(sys.argv[0]).parent
if os.environ.get('FAKE_HOST_FAIL'): sys.exit(9)
selections=state/'selections'; selections.mkdir(mode=0o700,exist_ok=True)
fd,path=tempfile.mkstemp(prefix='selection-',dir=selections)
with os.fdopen(fd,'w') as f: json.dump({'Generation':str(pathlib.Path(sys.argv[0]).parent)},f)
print(path)
''')
        (state / 'cercano-cli').write_text('''#!/usr/bin/env python3
import sys, json, os
assert os.environ['CERCANO_LAUNCHED_DEVELOPMENT']=='1'
with open(os.environ['FAKE_ENV_LOG'], 'a') as log:
 log.write(json.dumps(['cli', os.environ.get('CERCANO_DISABLE_CREDENTIALS')])+'\\n')
print(json.dumps(sys.argv[1:]))
''')
        return dict(os.environ, CERCANO_REPO=str(root / 'repo'),
                    CERCANO_DEVELOPMENT_STATE=str(state), CERCANO_DISABLE_CREDENTIALS='1',
                    FAKE_ENV_LOG=str(root / 'child-environments.jsonl'))

    def source_launch(self, env, *args):
        return subprocess.run(['/bin/bash', str(SCRIPTS / 'cercano-launcher.sh'), *args], env=env, capture_output=True, text=True)

    def test_runtime_preserves_credential_policy_while_builds_disable_credentials(self):
        # Only these fake-command probes opt out of the outer disabled policy.
        for policy in (None, '0', '1'):
            with self.subTest(policy=policy), tempfile.TemporaryDirectory() as root:
                env = self.fixture(root)
                if policy is None:
                    env.pop('CERCANO_DISABLE_CREDENTIALS')
                else:
                    env['CERCANO_DISABLE_CREDENTIALS'] = policy
                result = self.source_launch(env)
                self.assertEqual(result.returncode, 0, result.stderr)
                log = Path(env['FAKE_ENV_LOG'])
                with self.subTest(phase='launch'):
                    self.assertEqual([json.loads(line) for line in log.read_text().splitlines()], [
                        ['fingerprint', '1'], ['publish', '1'],
                        ['development-launch', policy], ['cli', policy],
                    ])
                selection = json.loads(result.stdout)[1]
                log.write_text('')
                result = self.source_launch(env, '--dev-host', selection)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual([json.loads(line) for line in log.read_text().splitlines()],
                                 [['cli', policy]])

    def test_source_launch_concurrent_selections_and_argument_boundaries(self):
        with tempfile.TemporaryDirectory() as root:
            env = self.fixture(root)
            with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
                results = list(pool.map(lambda _: self.source_launch(env, 'one argument'), range(2)))
            paths = []
            for result in results:
                self.assertEqual(result.returncode, 0, result.stderr)
                args = json.loads(result.stdout)
                self.assertEqual(args[0], '--dev-host')
                self.assertEqual(args[2:], ['one argument'])
                paths.append(args[1])
            self.assertNotEqual(*paths)
            self.assertTrue(all(Path(p).is_file() for p in paths))

    def test_explicit_selection_reconnect_never_builds_or_acquires(self):
        with tempfile.TemporaryDirectory() as root:
            env = self.fixture(root)
            initial = self.source_launch(env)
            self.assertEqual(initial.returncode, 0, initial.stderr)
            selection = Path(json.loads(initial.stdout)[1])
            before = selection.read_bytes()
            (Path(env['CERCANO_REPO']) / 'source/server/scripts/publish-dev-generation.py').unlink()
            result = self.source_launch(env, '--dev-host', str(selection), 'reconnect argument')
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(json.loads(result.stdout), ['--dev-host', str(selection), 'reconnect argument'])
            self.assertEqual(selection.read_bytes(), before)
            self.assertEqual(list(selection.parent.iterdir()), [selection])

    def test_flag_preflight_has_no_side_effects(self):
        for args in [('--dev-host=',), ('-dev-host=',), ('--dev-host', ''),
                     ('--dev-host', 'a', '-dev-host=b'), ('--dev-host=a', '--dev-host=a'),
                     ('positional', '--dev-host='), ('--unknown',),
                     ('--help',), ('-help',), ('--version',), ('-version',)]:
            with self.subTest(args=args), tempfile.TemporaryDirectory() as root:
                env = self.fixture(root)
                result = self.source_launch(env, *args)
                self.assertEqual(result.returncode, 0 if 'help' in args[0] or 'version' in args[0] else 78, result.stderr)
                self.assertFalse(Path(env['FAKE_ENV_LOG']).exists())
                self.assertFalse((Path(env['CERCANO_DEVELOPMENT_STATE']) / 'generations').exists())

    def test_equals_reconnect_and_tampering(self):
        for damage in ('none', 'cli', 'server', 'cli-symlink', 'manifest-symlink', 'manifest', 'selection-type', 'generation-type'):
            with self.subTest(damage=damage), tempfile.TemporaryDirectory() as root:
                env = self.fixture(root)
                initial = self.source_launch(env)
                self.assertEqual(initial.returncode, 0, initial.stderr)
                selection = Path(json.loads(initial.stdout)[1])
                generation = Path(json.loads(selection.read_text())['Generation'])
                log = Path(env['FAKE_ENV_LOG']); log.write_text('')
                if damage in ('cli', 'server'):
                    with (generation / ('cercano-cli' if damage == 'cli' else 'cercano')).open('a') as f:
                        f.write('\n# tampered\n')
                elif damage.endswith('-symlink'):
                    target = generation / ('cercano-cli' if damage == 'cli-symlink' else 'manifest.json')
                    copy = generation.parent / 'copy'; copy.write_bytes(target.read_bytes()); copy.chmod(0o755)
                    target.unlink(); target.symlink_to(copy)
                elif damage == 'manifest':
                    (generation / 'manifest.json').write_text('[]')
                elif damage == 'selection-type':
                    selection.write_text('[]')
                elif damage == 'generation-type':
                    selection.write_text('{"Generation": []}')
                result = self.source_launch(env, '-dev-host=' + str(selection))
                self.assertEqual(result.returncode, 0 if damage == 'none' else 78, result.stderr)
                self.assertNotIn('Traceback', result.stderr)
                events = [json.loads(line)[0] for line in log.read_text().splitlines()]
                self.assertEqual(events, ['cli'] if damage == 'none' else [])

    def test_reconnect_all_go_spellings(self):
        with tempfile.TemporaryDirectory() as root:
            env = self.fixture(root)
            initial = self.source_launch(env)
            self.assertEqual(initial.returncode, 0, initial.stderr)
            selection = json.loads(initial.stdout)[1]
            for args in [('--dev-host', selection), ('-dev-host', selection),
                         ('--dev-host=' + selection,), ('-dev-host=' + selection,)]:
                log = Path(env['FAKE_ENV_LOG']); log.write_text('')
                result = self.source_launch(env, *args)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(json.loads(result.stdout), ['--dev-host', selection])
                self.assertEqual([json.loads(line)[0] for line in log.read_text().splitlines()], ['cli'])

    def test_information_without_prepared_state(self):
        with tempfile.TemporaryDirectory() as root:
            env = self.fixture(root)
            env.pop('CERCANO_DEVELOPMENT_STATE')
            for flag in ('--version', '--help', '-h'):
                result = self.source_launch(env, flag)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertFalse(Path(env['FAKE_ENV_LOG']).exists())

    def test_host_failure_never_executes_cli(self):
        with tempfile.TemporaryDirectory() as root:
            env = self.fixture(root)
            env['FAKE_HOST_FAIL'] = '1'
            result = self.source_launch(env)
            self.assertEqual(result.returncode, 78)
            self.assertEqual(result.stdout, '')

    def test_unprepared_and_long_runtime_fail_before_publisher(self):
        with tempfile.TemporaryDirectory() as root:
            env = self.fixture(root)
            long = Path(root).resolve() / ('long' * 15)
            long.mkdir(mode=0o700)
            env['CERCANO_RUNTIME_ROOT'] = str(long)
            result = self.source_launch(env)
            self.assertEqual(result.returncode, 78)
            self.assertIn('38 resolved bytes', result.stderr)
            self.assertFalse((Path(env['CERCANO_DEVELOPMENT_STATE']) / 'generations').exists())
            env.pop('CERCANO_RUNTIME_ROOT')
            with closing(sqlite3.connect(Path(env['CERCANO_DEVELOPMENT_STATE']) / 'conversations.db')) as db, db:
                db.execute('delete from coexistence_metadata')
            result = self.source_launch(env)
            self.assertEqual(result.returncode, 78)
            self.assertIn('unprepared', result.stderr)
            self.assertFalse((Path(env['CERCANO_DEVELOPMENT_STATE']) / 'generations').exists())

    def install(self, *args):
        return subprocess.run(['/bin/bash', str(SCRIPTS / 'install-launcher.sh'), *args],
                              capture_output=True)

    def test_explicit_destination_required(self):
        self.assertEqual(self.install().returncode, 2)
        self.assertEqual(self.install('--force').returncode, 2)
        self.assertEqual(self.install('--destination', 'relative').returncode, 2)

    def test_install_and_refuse_overwrite_or_symlink(self):
        with tempfile.TemporaryDirectory(prefix='launcher safety ') as root:
            destination = Path(root) / 'cercano'
            result = self.install('--destination', str(destination))
            self.assertEqual(result.returncode, 0, result.stderr)
            original = destination.read_bytes()
            self.assertTrue(os.access(destination, os.X_OK))
            self.assertNotEqual(self.install('--destination', str(destination)).returncode, 0)
            self.assertEqual(destination.read_bytes(), original)
            alias = Path(root) / 'alias'
            alias.symlink_to(destination)
            self.assertNotEqual(self.install('--destination', str(alias)).returncode, 0)
            self.assertEqual(destination.read_bytes(), original)
            self.assertEqual(sorted(p.name for p in Path(root).iterdir()), ['alias', 'cercano'])


if __name__ == '__main__':
    unittest.main()
