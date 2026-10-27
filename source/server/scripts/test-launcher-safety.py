#!/usr/bin/env python3
"""Safety checks only; NOT production generation-launch acceptance."""
from pathlib import Path
import os
import subprocess
import tempfile
import unittest

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
