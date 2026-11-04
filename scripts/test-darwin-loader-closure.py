#!/usr/bin/env python3
"""Offline text reports and inert staged files only; no native tools invoked."""
import copy
import json
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import unittest

import darwin_loader_closure as closure

FIXTURE = Path(__file__).resolve().parents[1] / 'release/llama/fixtures/darwin-homebrew-reports.json'


class ClosureTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name) / 'stage'
        self.observed = json.loads(FIXTURE.read_text())
        self.safe = copy.deepcopy(self.observed)
        for name, report in self.safe['reports'].items():
            # Synthesize relocated name fields; install IDs remain non-edges.
            self.safe['reports'][name] = re.sub(
                r'(name )/opt/homebrew/[^\n]+/([^/\n]+)( \(offset)',
                r'\1@rpath/\2\3', report)
            p = self.root / name
            p.parent.mkdir(parents=True, exist_ok=True)
            p.write_text('inert fixture, not Mach-O')

    def test_safe_relocated_graph(self):
        self.assertEqual(len(closure.validate(self.root, self.safe)), 15)

    def test_observed_homebrew_graph_rejected(self):
        with self.assertRaisesRegex(closure.Failure, 'unsupported/external'):
            closure.validate(self.root, self.observed)

    def replace_edge(self, edge):
        name = 'bin/llama-server'
        self.safe['reports'][name] = self.safe['reports'][name].replace(
            '@rpath/libllama-server-impl.dylib', edge)

    def test_each_observed_external_family_rejected(self):
        for edge in ['/opt/homebrew/opt/ggml/lib/libggml.0.dylib',
                     '/opt/homebrew/opt/libomp/lib/libomp.dylib',
                     '/opt/homebrew/opt/openssl@3/lib/libssl.3.dylib',
                     '/opt/homebrew/Cellar/openssl@3/3.6.5/lib/libcrypto.3.dylib']:
            with self.subTest(edge=edge):
                data = copy.deepcopy(self.safe)
                data['reports']['bin/llama-server'] = data['reports']['bin/llama-server'].replace(
                    '@rpath/libllama-server-impl.dylib', edge)
                with self.assertRaisesRegex(closure.Failure, 'unsupported/external'):
                    closure.validate(self.root, data)

    def test_loader_and_executable_paths(self):
        for token in ['@loader_path', '@executable_path']:
            data = copy.deepcopy(self.safe)
            data['reports']['bin/llama-server'] = data['reports']['bin/llama-server'].replace(
                '@rpath/libllama-server-impl.dylib', token + '/../lib/libllama-server-impl.dylib')
            self.assertEqual(len(closure.validate(self.root, data)), 15)

    def test_inherited_rpath(self):
        # ssl has no LC_RPATH; resolves crypto through executable ancestry.
        self.assertNotIn('LC_RPATH', self.safe['reports']['lib/libssl.3.dylib'])
        closure.validate(self.root, self.safe)
        self.safe['reports']['lib/libggml.0.dylib'] = self.safe['reports']['lib/libggml.0.dylib'].replace(
            '@loader_path/../lib', '@loader_path')
        closure.validate(self.root, self.safe)

    def test_missing_library_backend_and_report(self):
        for name in ['lib/libllama.0.dylib', self.safe['backends'][0]]:
            with self.subTest(name=name):
                p = self.root / name
                p.unlink()
                with self.assertRaisesRegex(closure.Failure, 'missing'):
                    closure.validate(self.root, self.safe)
                p.write_text('inert')
                data = copy.deepcopy(self.safe)
                del data['reports'][name]
                with self.assertRaisesRegex(closure.Failure, 'missing report'):
                    closure.validate(self.root, data)

    def test_unresolved_and_unsupported_edges(self):
        for edge in ['@rpath/missing.dylib', 'relative.dylib', '@unknown/lib.dylib',
                     '/usr/lib/not-allowlisted.dylib', '@loader_path/../../outside',
                     '/usr/lib/../lib/libSystem.B.dylib']:
            with self.subTest(edge=edge):
                original = copy.deepcopy(self.safe)
                self.replace_edge(edge)
                with self.assertRaises(closure.Failure):
                    closure.validate(self.root, self.safe)
                self.safe = original

    def test_symlink_escape(self):
        for name in ['lib/libllama.0.dylib', self.safe['backends'][0]]:
            p = self.root / name
            p.unlink()
            outside = Path(self.tmp.name) / 'outside'
            outside.write_text('inert')
            p.symlink_to(outside)
            with self.assertRaisesRegex(closure.Failure, 'symlink escapes'):
                closure.validate(self.root, self.safe)
            p.unlink()
            p.write_text('inert')

    def test_safe_alias(self):
        p = self.root / 'lib/libllama.0.dylib'
        p.rename(p.with_name('libllama.real.dylib'))
        p.symlink_to('libllama.real.dylib')
        closure.validate(self.root, self.safe)

    def test_ambiguous_rpath(self):
        p = self.root / 'other/libllama-server-impl.dylib'
        p.parent.mkdir()
        p.write_text('inert')
        report = self.safe['reports']['bin/llama-server']
        count = len(re.findall('Load command ', report))
        self.safe['reports']['bin/llama-server'] += (
            f'Load command {count}\n          cmd LC_RPATH\n      cmdsize 48\n'
            '         path @loader_path/../other (offset 12)\n')
        with self.assertRaisesRegex(closure.Failure, 'ambiguous dependency'):
            closure.validate(self.root, self.safe)

    def test_external_unused_rpath(self):
        self.safe['reports']['bin/llama-server'] = self.safe['reports']['bin/llama-server'].replace(
            '@loader_path/../lib', '/opt/homebrew/opt/ggml/lib')
        with self.assertRaisesRegex(closure.Failure, 'unsupported/external'):
            closure.validate(self.root, self.safe)

    def test_install_id_not_edge(self):
        deps, _ = closure.parse_report(self.observed['reports']['lib/libomp.dylib'])
        self.assertEqual(set(deps), {'/usr/lib/libSystem.B.dylib', '/usr/lib/libc++.1.dylib'})

    def test_fail_closed_reports(self):
        for text in ['', 'file:\n', 'file:\n\t/usr/lib/libSystem.B.dylib (compatibility version 1.0.0)\n',
                     'file:\nLoad command 0\n cmd LC_LOAD_DYLIB\n',
                     'file:\nLoad command 0\n cmd LC_DYLD_ENVIRONMENT\n',
                     'file:\nLoad command 0\n cmd LC_PREBOUND_DYLIB\n',
                     self.safe['reports']['bin/llama-server'] + 'second image:\n']:
            with self.subTest(text=text), self.assertRaises(closure.Failure):
                closure.parse_report(text)

    def test_backend_inventory_required(self):
        self.safe['backends'] = []
        with self.assertRaisesRegex(closure.Failure, 'backend roots required'):
            closure.validate(self.root, self.safe)

    def test_cli_opt_in_and_limited_claim(self):
        manifest = Path(self.tmp.name) / 'reports.json'
        manifest.write_text(json.dumps(self.safe))
        command = [sys.executable, '-B', str(Path(closure.__file__)),
                   '--staged-root', str(self.root), '--loader-reports', str(manifest)]
        result = subprocess.run(command, text=True, capture_output=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('NOT verified: report freshness, dynamic discovery/resources', result.stdout)
        manifest.write_text(json.dumps(self.observed))
        self.assertNotEqual(subprocess.run(command, capture_output=True).returncode, 0)


if __name__ == '__main__':
    unittest.main()
