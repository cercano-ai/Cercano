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
        with sqlite3.connect(state / 'conversations.db') as db:
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
        (scripts / 'publish-dev-generation.py').write_text('''import sys, pathlib, os, fcntl
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
print(gen)
''')
        (state / 'cercano').write_text('''#!/usr/bin/env python3
import sys, os, pathlib, tempfile, json
assert sys.argv[1]=='development-launch'
assert os.environ['CERCANO_DISABLE_CREDENTIALS']=='1'
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
assert os.environ['CERCANO_DISABLE_CREDENTIALS']=='1'
print(json.dumps(sys.argv[1:]))
''')
        return dict(os.environ, CERCANO_REPO=str(root / 'repo'), CERCANO_DEVELOPMENT_STATE=str(state))

    def source_launch(self, env, *args):
        return subprocess.run(['/bin/bash', str(SCRIPTS / 'cercano-launcher.sh'), *args], env=env, capture_output=True, text=True)

    def test_source_launch_concurrent_selections_and_argument_boundaries(self):
        with tempfile.TemporaryDirectory() as root:
            env = self.fixture(root)
            with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
                results = list(pool.map(lambda _: self.source_launch(env, 'one argument', '--flag=x'), range(2)))
            paths = []
            for result in results:
                self.assertEqual(result.returncode, 0, result.stderr)
                args = json.loads(result.stdout)
                self.assertEqual(args[0], '--dev-host')
                self.assertEqual(args[2:], ['one argument', '--flag=x'])
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
            with sqlite3.connect(Path(env['CERCANO_DEVELOPMENT_STATE']) / 'conversations.db') as db:
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
