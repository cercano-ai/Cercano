#!/usr/bin/env python3
"""Ordinary shell/Python entry, compiled migration, disposable discovery boundary.
Host acquisition/publisher/CLI are synthetic: NOT native A/B execution coverage.
Build helper with -tags ordinary_upgrade_acceptance; pass its absolute path as
CERCANO_TEST_UPGRADE_BINARY. Never run production process discovery here.
"""
import concurrent.futures
import fcntl
import importlib.util
import os
from pathlib import Path
import pty
import sqlite3
import subprocess
import tempfile
import time
import unittest

SCRIPTS = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('safety', SCRIPTS / 'test-launcher-safety.py')
safety = importlib.util.module_from_spec(spec)
spec.loader.exec_module(safety)


class OrdinaryUpgrade(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve()
        self.env = safety.LauncherSafety().fixture(self.root)
        self.state = Path(self.env['CERCANO_DEVELOPMENT_STATE'])
        self.db = self.state / 'conversations.db'
        self.db.unlink()
        with sqlite3.connect(self.db) as conn:
            conn.executescript((SCRIPTS.parent / 'internal/conversation/schema.sql').read_text())
            conn.execute("insert into conversations(id,started_at,last_turn_at) values ('original',1,2)")
            conn.execute("insert into turns(id,conversation_id,role,content,created_at) values ('turn','original','user','fixture history',3)")
            conn.executescript("""CREATE TABLE delegated_tasks (
                id TEXT PRIMARY KEY, parent_id TEXT NOT NULL, status TEXT NOT NULL,
                task TEXT NOT NULL, work_dir TEXT NOT NULL, created_at INTEGER NOT NULL,
                updated_at INTEGER NOT NULL, child_id TEXT NOT NULL DEFAULT '',
                result_json TEXT NOT NULL DEFAULT 'null', error TEXT NOT NULL DEFAULT '',
                notification_pending INTEGER NOT NULL DEFAULT 0, wake_claimed INTEGER NOT NULL DEFAULT 0);
                INSERT INTO delegated_tasks(id,parent_id,status,task,work_dir,created_at,updated_at) VALUES('task','original','running','preserve','',4,5);""")
        self.db.chmod(0o644)
        (self.state / 'config.yaml').chmod(0o644)
        self.state.chmod(0o755)
        for name in ('home', 'xdg', 'tmp', 'bin'):
            (self.root / name).mkdir(mode=0o700)
        go = self.root / 'bin/go'
        go.write_text('#!/usr/bin/env python3\nimport os,shutil,sys\nshutil.copyfile(os.environ["CERCANO_TEST_UPGRADE_BINARY"],sys.argv[sys.argv.index("-o")+1]); os.chmod(sys.argv[sys.argv.index("-o")+1],0o700)\n')
        go.chmod(0o700)
        self.env.update(HOME=str(self.root / 'home'), XDG_CONFIG_HOME=str(self.root / 'xdg'),
                        TMPDIR=str(self.root / 'tmp'), PATH=str(self.root / 'bin')+os.pathsep+os.environ['PATH'],
                        CERCANO_TEST_UPGRADE_BINARY=os.environ['CERCANO_TEST_UPGRADE_BINARY'],
                        CERCANO_ACCEPTANCE_UPGRADE_PROCESSES='empty')
        # Exercise the ordinary default original path, not a database override.
        (self.root / 'home/.config').mkdir(mode=0o700)
        original = self.root / 'home/.config/cercano'
        self.state.rename(original)
        self.state, self.db = original, original / 'conversations.db'
        self.env.pop('CERCANO_DEVELOPMENT_STATE')

    def launch(self, answer='y', extra=()):
        master, slave = pty.openpty()
        try:
            proc = subprocess.Popen(['bash', str(SCRIPTS / 'cercano-launcher.sh'), *extra],
                                    env=self.env, stdin=slave, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
            os.write(master, (answer+'\n').encode())
            out, err = proc.communicate(timeout=40)
            return proc.returncode, out, err
        finally:
            os.close(master)
            os.close(slave)

    def counts(self, path):
        with sqlite3.connect(path) as conn:
            return (conn.execute('select count(*) from conversations').fetchone()[0],
                    conn.execute('select count(*) from turns').fetchone()[0],
                    conn.execute('select count(*) from delegated_tasks').fetchone()[0],
                    conn.execute('select status,updated_at from delegated_tasks').fetchall())

    def backups(self):
        return list(self.state.glob('conversations.before-upgrade-*.db'))

    def test_confirm_preserves_and_repeats(self):
        expected = (1, 1, 1, [('running', 5)])
        rc, out, err = self.launch(extra=('hello',))
        self.assertEqual(rc, 0, err)
        self.assertIn('hello', out)
        self.assertEqual(self.counts(self.db)[:3], expected[:3])
        self.assertEqual(self.counts(self.db)[3][0][0], 'interrupted')
        self.assertEqual(len(self.backups()), 1)
        self.assertEqual(self.counts(self.backups()[0]), expected)
        for p in (self.db, self.state / 'config.yaml', self.backups()[0]):
            self.assertEqual(p.stat().st_mode & 0o777, 0o600)
        self.assertEqual(self.state.stat().st_mode & 0o777, 0o700)
        # A compatible repeat must not even invoke discovery, despite active fixture.
        self.env['CERCANO_ACCEPTANCE_UPGRADE_PROCESSES'] = 'active'
        rc, _, err = self.launch(answer='no')
        self.assertEqual(rc, 0, err)
        self.assertNotIn('Stop agents and upgrade now?', err)
        self.assertEqual(len(self.backups()), 1)

    def test_decline_unchanged(self):
        before = self.db.read_bytes()
        modes = [p.stat().st_mode for p in (self.db, self.state, self.state / 'config.yaml')]
        rc, _, err = self.launch('no')
        self.assertEqual(rc, 78)
        self.assertIn('declined', err)
        self.assertEqual(self.db.read_bytes(), before)
        self.assertEqual(modes, [p.stat().st_mode for p in (self.db, self.state, self.state / 'config.yaml')])
        self.assertFalse(self.backups())
        self.assertFalse(Path(self.env['FAKE_ENV_LOG']).exists())

    def test_only_single_y_confirmation(self):
        for answer in ('', 'n', 'N', 'yes', 'STOPPED', ' y', 'y '):
            before = self.snapshot()
            rc, _, err = self.launch(answer)
            self.assertEqual(rc, 78, err)
            self.assertEqual(before, self.snapshot())
            self.assertFalse(self.backups())
        rc, _, err = self.launch('Y')
        self.assertEqual(rc, 0, err)

    def test_active_or_unresolved_refused_before_mutation(self):
        for process in ('active', 'unknown'):
            self.env['CERCANO_ACCEPTANCE_UPGRADE_PROCESSES'] = process
            before = self.db.read_bytes()
            rc, _, err = self.launch()
            self.assertEqual(rc, 78, err)
            self.assertEqual(self.db.read_bytes(), before)
            self.assertEqual(self.db.stat().st_mode & 0o777, 0o644)
            self.assertFalse(self.backups())

    def test_unsupported_before_prompt(self):
        for kind in ('newer', 'unknown', 'corrupt'):
            if kind == 'newer':
                with sqlite3.connect(self.db) as conn:
                    conn.execute('pragma user_version=99')
            elif kind == 'unknown':
                self.db.unlink()
                with sqlite3.connect(self.db) as conn:
                    conn.execute('create table unrelated (id integer)')
                self.db.chmod(0o644)
            else:
                self.db.write_bytes(b'not sqlite')
            before = self.snapshot()
            rc, _, err = self.launch()
            self.assertEqual(rc, 78, err)
            self.assertIn('unsupported', err)
            self.assertNotIn('Stop agents and upgrade now?', err)
            self.assertEqual(before, self.snapshot())
            self.assertFalse(self.backups())

    def snapshot(self):
        return {p.name: (p.read_bytes(), p.stat().st_mode) for p in self.state.iterdir() if p.is_file()}

    def test_nested_authority(self):
        parent = self.state / 'nested'
        parent.mkdir(mode=0o755)
        authority = parent / 'config.yaml'
        (self.state / 'config.yaml').rename(authority)
        self.env['CERCANO_DEVELOPMENT_CONFIG_AUTHORITY'] = str(authority)
        rc, _, err = self.launch()
        self.assertEqual(rc, 0, err)
        self.assertEqual(parent.stat().st_mode & 0o777, 0o700)
        self.assertEqual(authority.stat().st_mode & 0o777, 0o600)

    def test_second_discovery_failure_retains_backup(self):
        self.env['CERCANO_ACCEPTANCE_UPGRADE_PROCESSES'] = 'second-active'
        self.env['CERCANO_ACCEPTANCE_DISCOVERY_RECORD'] = str(self.root / 'discovery')
        expected = self.counts(self.db)
        before = self.db.read_bytes()
        rc, _, err = self.launch()
        self.assertEqual(rc, 78, err)
        self.assertIn('retained backup', err)
        self.assertEqual(self.db.read_bytes(), before)
        self.assertEqual(self.counts(self.db), expected)
        self.assertEqual(self.counts(self.backups()[0]), expected)

    def test_unsafe_sidecars(self):
        for suffix in ('-wal', '-shm', '-journal'):
            for link in ('symlink', 'hardlink'):
                target = self.root / 'target'
                target.write_bytes(b'untouched')
                sidecar = Path(str(self.db) + suffix)
                if link == 'symlink':
                    sidecar.symlink_to(target)
                else:
                    os.link(target, sidecar)
                before = self.db.read_bytes()
                rc, _, err = self.launch()
                self.assertEqual(rc, 78, err)
                self.assertNotIn('Stop agents and upgrade now?', err)
                self.assertEqual(self.db.read_bytes(), before)
                self.assertEqual(target.read_bytes(), b'untouched')
                self.assertFalse(self.backups())
                sidecar.unlink()
                target.unlink()

    def test_wal_refusals_unchanged(self):
        self.make_wal()
        for missing_shm in (False, True):
            if missing_shm:
                Path(str(self.db) + '-shm').unlink()
            for interactive in (False, True):
                before = self.snapshot()
                modes = (self.state.stat().st_mode,)
                if interactive:
                    rc, _, err = self.launch('no')
                else:
                    result = subprocess.run(['bash', str(SCRIPTS / 'cercano-launcher.sh')],
                                            env=self.env, input='', capture_output=True, text=True)
                    rc, err = result.returncode, result.stderr
                self.assertEqual(rc, 78, err)
                self.assertEqual(before, self.snapshot())
                self.assertEqual(modes, (self.state.stat().st_mode,))
                self.assertFalse(self.backups())

    def test_startup_failure_retains_backup(self):
        self.env['FAKE_HOST_FAIL'] = '1'
        rc, _, err = self.launch()
        self.assertEqual(rc, 78)
        self.assertIn(str(self.backups()[0]), err)
        self.assertEqual(self.counts(self.db)[:3], self.counts(self.backups()[0])[:3])
        self.assertEqual(self.counts(self.db)[3][0][0], 'interrupted')

    def test_shared_lock_serializes_ordinary_launches(self):
        lock = open(self.root / 'tmp/cercano-agent-launch.lock', 'w')
        self.addCleanup(lock.close)
        fcntl.flock(lock, fcntl.LOCK_EX)
        with concurrent.futures.ThreadPoolExecutor(2) as pool:
            first, second = pool.submit(self.launch), pool.submit(self.launch)
            time.sleep(0.3)
            self.assertFalse(first.done())
            self.assertFalse(second.done())
            self.assertFalse(self.backups())
            fcntl.flock(lock, fcntl.LOCK_UN)
            for future in (first, second):
                rc, _, err = future.result()
                self.assertEqual(rc, 0, err)
        self.assertEqual(len(self.backups()), 1)

    def make_wal(self):
        # Child exits without closing SQLite: committed WAL remains, no writer remains.
        subprocess.run(['python3', '-c', 'import sqlite3,os,sys; c=sqlite3.connect(sys.argv[1]); c.execute("pragma journal_mode=wal"); c.execute("insert into turns(id,conversation_id,role,content,created_at) values (\'wal\',\'original\',\'user\',\'wal fixture\',6)"); c.commit(); os._exit(0)', str(self.db)], check=True, env=self.env)
        self.assertTrue(Path(str(self.db)+'-wal').exists())

    def test_committed_wal_backup(self):
        self.make_wal()
        rc, _, err = self.launch()
        self.assertEqual(rc, 0, err)
        self.assertEqual(self.counts(self.backups()[0]), (1, 2, 1, [('running', 5)]))
        self.assertEqual(self.counts(self.db)[:3], self.counts(self.backups()[0])[:3])
        self.assertEqual(self.counts(self.db)[3][0][0], 'interrupted')


if __name__ == '__main__':
    unittest.main()
