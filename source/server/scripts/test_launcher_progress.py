"""Disposable lock/progress regressions; no DB, config, inventory or permissions changes."""
import contextlib
import importlib.util
import io
import re
from pathlib import Path
import subprocess
import sys
import tempfile
import threading
import time
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('launcher', Path(__file__).with_name('cercano-launcher.py'))
launcher = importlib.util.module_from_spec(spec)
spec.loader.exec_module(launcher)


class ProgressTests(unittest.TestCase):
    def test_tty_replaces_shorter_line_and_cleans_up(self):
        class TTY(io.StringIO):
            def isatty(self):
                return True
        with contextlib.redirect_stderr(TTY()) as output:
            launcher.feedback('long status message')
            launcher.feedback('short')
            launcher.clear_progress()
            launcher.clear_progress()
        self.assertEqual(output.getvalue(),
                         '\r\x1b[2K[cercano] long status message'
                         '\r\x1b[2K[cercano] short\r\x1b[2K')

    def test_non_tty_has_no_controls(self):
        with contextlib.redirect_stderr(io.StringIO()) as output:
            launcher.feedback('long status message')
            launcher.feedback('short')
            launcher.clear_progress()
        self.assertEqual(output.getvalue(),
                         '[cercano] long status message\n[cercano] short\n')

    def test_tty_durable_output_and_prompt_survive(self):
        class TTY(io.StringIO):
            def isatty(self):
                return True
        with contextlib.redirect_stderr(TTY()) as output:
            launcher.feedback('inspection')
            launcher.ordinary_output('warning: retained backup', file=sys.stderr)
            launcher.feedback('inspection')
            with patch.object(sys, 'stdin', TTY('n\n')):
                with self.assertRaisesRegex(ValueError, 'upgrade declined'):
                    launcher.confirmed_upgrade(None, None, None, None, None)
            launcher.ordinary_output('launch refused', file=sys.stderr)
        text = output.getvalue()
        self.assertIn('\r\x1b[2Kwarning: retained backup\n', text)
        self.assertIn('\r\x1b[2KDatabase upgrade required', text)
        self.assertIn('Stop agents and upgrade now? [y/N]', text)
        self.assertNotIn('\x1b', text.split('Database upgrade required')[1])

    def test_tty_child_failure_and_unterminated_diagnostic_survive(self):
        class TTY(io.StringIO):
            def isatty(self):
                return True
        with contextlib.redirect_stderr(TTY()) as output:
            launcher.feedback('building')
            with self.assertRaises(subprocess.CalledProcessError) as failure:
                launcher.progress_check_output([sys.executable, '-c',
                    'import sys; print("protocol"); sys.stderr.write("build failed: detail"); sys.exit(42)'], text=True)
            launcher.feedback('finished')
            launcher.clear_progress()
        self.assertEqual(failure.exception.returncode, 42)
        self.assertEqual(failure.exception.output, 'protocol\n')
        self.assertIn('\r\x1b[2Kbuild failed: detail\n\r\x1b[2K', output.getvalue())

    def test_child_prompt_is_visible_before_child_exits(self):
        with tempfile.TemporaryDirectory() as root:
            marker = Path(root) / 'prompt-seen'
            class TTY(io.StringIO):
                def isatty(self):
                    return True

                def write(self, text):
                    if 'Continue?' in text:
                        marker.touch()
                    return super().write(text)
            with contextlib.redirect_stderr(TTY()) as output:
                launcher.feedback('working')
                result = launcher.progress_check_output([sys.executable, '-c',
                    'import pathlib,sys,time; '
                    'p=pathlib.Path(sys.argv[1]); '
                    'sys.stderr.write("Continue?"); sys.stderr.flush(); '
                    'deadline=time.monotonic()+3\n'
                    'while not p.exists() and time.monotonic()<deadline: time.sleep(.01)\n'
                    'sys.exit(0 if p.exists() else 42)', str(marker)], text=True)
                launcher.clear_progress()
            self.assertEqual(result, '')
            self.assertEqual(output.getvalue(),
                             '\r\x1b[2K[cercano] working\r\x1b[2KContinue?\n')

    def test_narrow_terminal_does_not_wrap(self):
        class TTY(io.StringIO):
            def isatty(self):
                return True
        import os
        with contextlib.redirect_stderr(TTY()) as output, \
             patch.object(launcher.shutil, 'get_terminal_size', return_value=os.terminal_size((20, 24))):
            launcher.feedback('a very long routine status')
            launcher.clear_progress()
        self.assertEqual(output.getvalue(), '\r\x1b[2K[cercano] a very lo\r\x1b[2K')

    def test_uncontended_lock_does_not_claim_wait(self):
        with tempfile.TemporaryFile() as lock, contextlib.redirect_stderr(io.StringIO()) as output:
            launcher.acquire_lock(lock, 'fixture lock')
        self.assertNotIn('Waiting', output.getvalue())
        self.assertIn('acquired elapsed=', output.getvalue())

    def test_real_contention(self):
        with tempfile.TemporaryDirectory() as root:
            path = Path(root) / 'lock'
            with path.open('w+') as lock:
                child = subprocess.Popen([sys.executable, '-c',
                    'import fcntl,sys; f=open(sys.argv[1],"r+"); fcntl.flock(f,fcntl.LOCK_EX); print("ready",flush=True); sys.stdin.readline()', str(path)],
                    stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True)
                try:
                    self.assertEqual(child.stdout.readline().strip(), 'ready')
                    def release():
                        time.sleep(.1)
                        child.stdin.write('\n')
                        child.stdin.flush()
                    thread = threading.Thread(target=release)
                    thread.start()
                    with contextlib.redirect_stderr(io.StringIO()) as output:
                        launcher.acquire_lock(lock, 'fixture lock')
                    thread.join()
                    self.assertEqual(child.wait(timeout=5), 0)
                    self.assertIn('Waiting for fixture lock (contended)', output.getvalue())
                    self.assertIn('acquired elapsed=', output.getvalue())
                    elapsed = float(re.search(r'acquired elapsed=([0-9.]+)s', output.getvalue())[1])
                    self.assertGreaterEqual(elapsed, .09)
                finally:
                    if child.poll() is None:
                        child.kill()  # only our disposable child
                        child.wait()
                    child.stdin.close()
                    child.stdout.close()

    def test_phases_report_elapsed_without_swallowing_refusal(self):
        for name in ('inspection', 'helper build', 'discovery and verified shutdown', 'backup', 'migration'):
            with contextlib.redirect_stderr(io.StringIO()) as output:
                with self.assertRaisesRegex(ValueError, 'sentinel refusal'):
                    with launcher.phase(name):
                        raise ValueError('sentinel refusal')
            self.assertIn(name + ' elapsed=', output.getvalue())


if __name__ == '__main__':
    unittest.main()
