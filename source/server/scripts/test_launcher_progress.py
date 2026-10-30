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

spec = importlib.util.spec_from_file_location('launcher', Path(__file__).with_name('cercano-launcher.py'))
launcher = importlib.util.module_from_spec(spec)
spec.loader.exec_module(launcher)


class ProgressTests(unittest.TestCase):
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
