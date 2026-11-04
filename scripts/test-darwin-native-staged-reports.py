#!/usr/bin/env python3
"""Opt-in Darwin integration: compile local C, inspect bytes, never run payloads."""
import os
from pathlib import Path
import platform
import subprocess
import sys
import tempfile
import unittest

import darwin_loader_closure as closure
import darwin_staged_reports as fresh


@unittest.skipUnless(os.environ.get('CERCANO_DARWIN_NATIVE_TESTS') == '1',
                     'opt in with CERCANO_DARWIN_NATIVE_TESTS=1')
@unittest.skipUnless(sys.platform == 'darwin', 'requires Darwin host tools')
class NativeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.architecture = platform.machine()
        if cls.architecture not in fresh.ARCHITECTURES:
            raise unittest.SkipTest('requires arm64 or x86_64 host')
        for tool in ['/usr/bin/xcrun', fresh.OTOOL]:
            if not os.access(tool, os.X_OK):
                raise unittest.SkipTest(f'missing host tool: {tool}')
        cls.env = {'PATH': '/usr/bin:/bin', 'LC_ALL': 'C'}
        try:
            found = subprocess.run(['/usr/bin/xcrun', '--find', 'clang'],
                                   env=cls.env, capture_output=True, text=True, timeout=10)
            sdk = subprocess.run(['/usr/bin/xcrun', '--show-sdk-path'],
                                 env=cls.env, capture_output=True, text=True, timeout=10)
        except (OSError, subprocess.TimeoutExpired) as error:
            raise unittest.SkipTest(f'host toolchain unavailable: {error}')
        if found.returncode or sdk.returncode:
            raise unittest.SkipTest('host xcrun clang/SDK unavailable: '
                                    + found.stderr + sdk.stderr)
        compiler = Path(found.stdout.strip())
        if not compiler.is_absolute() or not os.access(compiler, os.X_OK):
            raise unittest.SkipTest('xcrun did not resolve an executable host clang')

    def setUp(self):
        temporary = tempfile.TemporaryDirectory(prefix='cercano-native-')
        self.addCleanup(temporary.cleanup)
        self.work = Path(temporary.name).resolve()
        self.stage = self.work / 'original-stage'
        (self.stage / 'bin').mkdir(parents=True)
        (self.stage / 'lib').mkdir()
        (self.work / 'library.c').write_text('int fixture_value(void) { return 7; }\n')
        (self.work / 'main.c').write_text(
            'extern int fixture_value(void);\n'
            'int main(void) { return fixture_value(); }\n')
        self.inventory = {'executable': 'bin/fixture', 'backends': ['lib/libfixture.dylib'],
                          'images': ['bin/fixture', 'lib/libfixture.dylib']}

    def compile(self, *args):
        # No shell, downloaded sources, staged tools, inherited compiler flags or payload execution.
        result = subprocess.run(['/usr/bin/xcrun', 'clang', '-arch', self.architecture,
                                 *map(str, args)], cwd=self.work, env=self.env,
                                capture_output=True, text=True, timeout=60)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def build(self, install_name='@rpath/libfixture.dylib'):
        library = self.stage / 'lib/libfixture.dylib'
        self.compile('-dynamiclib', self.work / 'library.c', '-Wl,-install_name,' + install_name,
                     '-o', library)
        self.compile(self.work / 'main.c', library, '-Wl,-rpath,@executable_path/../lib',
                     '-o', self.stage / 'bin/fixture')

    def collect(self):
        # Default host_tool invokes real /usr/bin/otool; no injection/mocking.
        return fresh.collect(self.stage, self.inventory, self.architecture)

    def test_relocated_rpath_closure(self):
        self.build()
        moved = self.work / 'relocated stage'
        self.stage.rename(moved)
        self.stage = moved
        self.assertFalse((self.work / 'original-stage').exists())
        self.assertEqual(self.collect(), sorted(self.inventory['images']))

    def test_missing_library(self):
        self.build()
        # Keep valid explicit backend inventory while removing the dependency target.
        (self.stage / 'lib/libfixture.dylib').rename(self.stage / 'lib/backend.dylib')
        self.inventory['backends'] = ['lib/backend.dylib']
        self.inventory['images'][1] = 'lib/backend.dylib'
        with self.assertRaisesRegex(closure.Failure, 'unresolved/ambiguous dependency'):
            self.collect()

    def test_external_non_system_dependency(self):
        external = str(self.work / 'outside-stage/libfixture.dylib')
        self.build(external)
        with self.assertRaisesRegex(closure.Failure, 'unsupported/external loader path'):
            self.collect()


if __name__ == '__main__':
    unittest.main()
