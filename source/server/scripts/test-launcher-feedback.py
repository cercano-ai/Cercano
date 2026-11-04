#!/usr/bin/env python3
"""Output ordering checks; no live state, database, build, or host acquisition."""
import importlib.util
import io
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

SCRIPTS = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('launcher', SCRIPTS / 'cercano-launcher.py')
launcher = importlib.util.module_from_spec(spec)
spec.loader.exec_module(launcher)


class FlushedOutput(io.StringIO):
    def __init__(self):
        super().__init__()
        self.flushed = ''

    def flush(self):
        self.flushed = self.getvalue()


class LauncherFeedback(unittest.TestCase):
    def test_feedback_is_flushed_before_launch_lock_and_database_check(self):
        stderr = FlushedOutput()
        stdout = io.StringIO()
        with tempfile.TemporaryDirectory() as root:
            def blocked_lock(*args):
                self.assertIn('[cercano] Starting', stderr.flushed)
                self.assertNotIn('waiting for launch lock', stderr.flushed)

            def database_phase():
                # Represents the later DB compatibility/prompt boundary. No DB opens.
                self.assertIn('[cercano] Starting', stderr.flushed)

            with patch.dict(os.environ, {'TMPDIR': root}), \
                 patch.object(sys, 'argv', ['launcher', '--repo', root]), \
                 patch.object(sys, 'stderr', stderr), patch.object(sys, 'stdout', stdout), \
                 patch.object(launcher.fcntl, 'flock', side_effect=blocked_lock), \
                 patch.object(launcher, 'launch_main', side_effect=database_phase):
                launcher.main()
        self.assertEqual(stdout.getvalue(), '')

    def test_database_check_has_flushed_feedback_before_inspection(self):
        class StopBeforeInspection(Exception):
            pass

        stderr = FlushedOutput()
        with tempfile.TemporaryDirectory() as root:
            # The state parent must be private, even when TMPDIR is shared /tmp.
            state = Path(root).resolve() / 'state'
            state.mkdir(mode=0o700)
            for name in ('config.yaml', 'conversations.db'):
                (state / name).write_text('disposable sentinel; never opened')
                (state / name).chmod(0o600)

            def inspect(db):
                self.assertEqual(db, state / 'conversations.db')
                self.assertIn('Checking local state and database compatibility', stderr.flushed)
                raise StopBeforeInspection()

            with patch.dict(os.environ, {'CERCANO_DEVELOPMENT_STATE': str(state),
                                         'CERCANO_DEVELOPMENT_CONFIG_AUTHORITY': str(state / 'config.yaml'),
                                         'CERCANO_CONVERSATIONS_DB': str(state / 'conversations.db')}), \
                 patch.object(sys, 'argv', ['launcher', '--repo', root]), \
                 patch.object(sys, 'stderr', stderr), \
                 patch.object(launcher, 'prepare_if_absent'), \
                 patch.object(launcher, 'compatible_store', side_effect=inspect), \
                 patch.object(launcher.sqlite3, 'connect', side_effect=AssertionError('unexpected DB open')):
                with self.assertRaises(StopBeforeInspection):
                    launcher.launch_main()

    def test_mocked_launch_phases_preserve_stdout_protocol(self):
        # Reuse the synthetic publisher/CLI fixture, never the real Go programs.
        safety_spec = importlib.util.spec_from_file_location('safety', SCRIPTS / 'test-launcher-safety.py')
        safety = importlib.util.module_from_spec(safety_spec)
        safety_spec.loader.exec_module(safety)
        with tempfile.TemporaryDirectory() as root, \
             tempfile.TemporaryDirectory(prefix='cf-', dir='/tmp') as runtime:
            fixture = safety.LauncherSafety()
            env = fixture.fixture(root)
            env.update(TMPDIR=root, CERCANO_RUNTIME_ROOT=str(Path(runtime).resolve()))
            result = fixture.source_launch(env)
            self.assertEqual(result.returncode, 0, result.stderr)
            import re
            lines = [re.sub(r'elapsed=[0-9.]+s', 'elapsed=<time>', line)
                     for line in result.stderr.splitlines()]
            self.assertEqual(lines, [
                '[cercano] Starting Cercano...',
                '[cercano] launch lock acquired elapsed=<time>',
                '[cercano] Checking local state and database compatibility...',
                '[cercano] state preparation lock acquired elapsed=<time>',
                '[cercano] inspection started',
                '[cercano] inspection elapsed=<time>',
                '[cercano] Checking source fingerprint...',
                '[cercano] Preparing binaries (building if needed)...',
                '[cercano] Acquiring development host...',
            ])
            import json
            args = json.loads(result.stdout)
            self.assertEqual(args[0], '--dev-host')
            self.assertEqual(len(args), 2)

    def test_real_tty_fixture_clears_before_cli_handoff(self):
        import pty
        import subprocess
        safety_spec = importlib.util.spec_from_file_location('safety', SCRIPTS / 'test-launcher-safety.py')
        safety = importlib.util.module_from_spec(safety_spec)
        safety_spec.loader.exec_module(safety)
        with tempfile.TemporaryDirectory() as root, \
             tempfile.TemporaryDirectory(prefix='cf-', dir='/tmp') as runtime:
            env = safety.LauncherSafety().fixture(root)
            env.update(TMPDIR=root, CERCANO_RUNTIME_ROOT=str(Path(runtime).resolve()))
            master, slave = pty.openpty()
            try:
                child = subprocess.Popen(['/bin/bash', str(SCRIPTS / 'cercano-launcher.sh')],
                                         env=env, stdout=slave, stderr=slave)
                os.close(slave)
                slave = None
                chunks = []
                import select
                import time
                deadline = time.monotonic() + 15
                while True:
                    ready, _, _ = select.select([master], [], [], max(0, deadline - time.monotonic()))
                    if not ready:
                        child.kill()  # only this disposable fixture child
                        child.wait()
                        self.fail('fixture PTY timed out')
                    try:
                        chunk = os.read(master, 8192)
                    except OSError as error:
                        import errno
                        if error.errno != errno.EIO:
                            raise
                        break
                    if not chunk:
                        break
                    chunks.append(chunk)
                self.assertEqual(child.wait(timeout=5), 0)
                text = b''.join(chunks).decode()
                self.assertIn('Starting Cercano...', text)
                self.assertIn('Acquiring development host...', text)
                # No routine step ended a line, and the final erase precedes
                # the fixture CLI's first ordinary output.
                self.assertEqual(text.count('\n'), 1)
                self.assertIn('\r\x1b[2K["--dev-host",', text)
            finally:
                os.close(master)
                if slave is not None:
                    os.close(slave)

    def test_information_is_unchanged_and_never_locks(self):
        for flag in ('-h', '--h', '-help', '--help', '-version', '--version'):
            with self.subTest(flag=flag):
                stderr, stdout = io.StringIO(), io.StringIO()
                with patch.object(sys, 'argv', ['launcher', '--repo', '/unused', flag]), \
                     patch.object(sys, 'stderr', stderr), patch.object(sys, 'stdout', stdout), \
                     patch.object(launcher.os, 'open', side_effect=AssertionError('unexpected lock')):
                    launcher.main()
                self.assertEqual(stderr.getvalue(), '')
                expected = ('cercano development source launcher (binary version requires an explicit generation)\n'
                            if 'version' in flag else
                            'Usage: cercano [--dev-host SELECTION] [--dev-host-action ACTION]\nDevelopment only; no legacy discovery. --help / --version never acquire a host.\n')
                self.assertEqual(stdout.getvalue(), expected)


if __name__ == '__main__':
    unittest.main()
