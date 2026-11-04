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


# Real arm64 host otool -l __TEXT excerpt from a temporary clang-built
# int main(void) { return 0; }. Only the report header/ordinal are normalized.
SEGMENT_REPORT = '''file:
Load command 0
      cmd LC_SEGMENT_64
  cmdsize 232
  segname __TEXT
   vmaddr 0x0000000100000000
   vmsize 0x0000000000004000
  fileoff 0
 filesize 16384
  maxprot 0x00000005
 initprot 0x00000005
   nsects 2
    flags 0x0
Section
  sectname __text
   segname __TEXT
      addr 0x0000000100000328
      size 0x0000000000000014
    offset 808
     align 2^2 (4)
    reloff 0
    nreloc 0
     flags 0x80000400
 reserved1 0
 reserved2 0
Section
  sectname __unwind_info
   segname __TEXT
      addr 0x000000010000033c
      size 0x0000000000000058
    offset 828
     align 2^2 (4)
    reloff 0
    nreloc 0
     flags 0x00000000
 reserved1 0
 reserved2 0
'''


# Real arm64 host otool -l libSystem load from the local native fixture.
# Only the pathname header and load command ordinal are normalized.
DYLIB_REPORT = '''file:
Load command 0
          cmd LC_LOAD_DYLIB
      cmdsize 56
         name /usr/lib/libSystem.B.dylib (offset 24)
   time stamp 2 Wed Dec 31 16:00:02 1969
      current version 1356.0.0
compatibility version 1.0.0
'''


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

    def test_unknown_command_rejected(self):
        with self.assertRaisesRegex(closure.Failure, 'unsupported.*LC_BOGUS'):
            closure.parse_report('file:\nLoad command 0\n cmd LC_BOGUS\n')

    def test_missing_cmdsize_rejected(self):
        for fields in [' cmd LC_UUID\n',
                       ' cmd LC_LOAD_DYLIB\n name /usr/lib/libSystem.B.dylib (offset 24)\n',
                       ' cmd LC_RPATH\n path @loader_path (offset 12)\n']:
            with self.subTest(fields=fields), self.assertRaisesRegex(closure.Failure, 'cmdsize'):
                closure.parse_report('file:\nLoad command 0\n' + fields)

    def test_valid_nondependency_command(self):
        self.assertEqual(closure.parse_report(
            'file:\nLoad command 0\n cmd LC_UUID\n cmdsize 24\n'
            ' uuid 12345678-1234-1234-1234-123456789ABC\n'), ([], []))

    def test_native_segment_section_markers(self):
        self.assertEqual(closure.parse_report(SEGMENT_REPORT), ([], []))
        # Synthetic 32-bit counterpart exercises the other existing segment
        # command; payload interpretation is intentionally outside this parser.
        self.assertEqual(closure.parse_report(SEGMENT_REPORT.replace(
            'LC_SEGMENT_64', 'LC_SEGMENT')), ([], []))

    def test_section_marker_rejected_in_nonsegment_commands(self):
        for cmd in sorted((closure.NONDEPENDENCY | closure.STRING_COMMANDS)
                          - {'LC_SEGMENT', 'LC_SEGMENT_64'}):
            with self.subTest(cmd=cmd), self.assertRaisesRegex(
                    closure.Failure, 'unexpected report header or diagnostic'):
                closure.parse_report(
                    f'file:\nLoad command 0\n cmd {cmd}\n cmdsize 32\nSection\n')

    def test_segment_unindented_diagnostics_rejected(self):
        for cmd in ['LC_SEGMENT', 'LC_SEGMENT_64']:
            for line in ['otool: unexpected diagnostic', 'second image:',
                         'Section extra', 'Section ', 'Sections', 'section']:
                with self.subTest(cmd=cmd, line=line), self.assertRaisesRegex(
                        closure.Failure, 'unexpected report header or diagnostic'):
                    closure.parse_report(SEGMENT_REPORT.replace(
                        'LC_SEGMENT_64', cmd) + line + '\n')

    def test_native_dylib_versions(self):
        for cmd in sorted(closure.LOADS | {'LC_ID_DYLIB'}):
            for current in ['      current version', 'current version']:
                with self.subTest(cmd=cmd, current=current):
                    report = DYLIB_REPORT.replace('LC_LOAD_DYLIB', cmd).replace(
                        '      current version', current)
                    expected = [] if cmd == 'LC_ID_DYLIB' else ['/usr/lib/libSystem.B.dylib']
                    self.assertEqual(closure.parse_report(report), (expected, []))

    def test_dylib_versions_rejected_in_wrong_commands(self):
        for cmd in sorted((closure.NONDEPENDENCY | closure.STRING_COMMANDS)
                          - closure.LOADS - {'LC_ID_DYLIB'}):
            for field in ['current', 'compatibility']:
                with self.subTest(cmd=cmd, field=field), self.assertRaisesRegex(
                        closure.Failure, 'unexpected report header or diagnostic'):
                    closure.parse_report(
                        f'file:\nLoad command 0\n cmd {cmd}\n cmdsize 32\n'
                        f'{field} version 1.0.0\n')

    def test_dylib_malformed_versions_and_diagnostics_rejected(self):
        for cmd in sorted(closure.LOADS | {'LC_ID_DYLIB'}):
            for field in ['current', 'compatibility']:
                for version in ['', '1', '1.0', '1.0.0.0', '-1.0.0', '+1.0.0',
                                '1.x.0', '1.0.0 extra', '1.0.0 ', '１.0.0', '1..0']:
                    with self.subTest(cmd=cmd, field=field, version=version), self.assertRaisesRegex(
                            closure.Failure, 'unexpected report header or diagnostic'):
                        closure.parse_report(DYLIB_REPORT.replace('LC_LOAD_DYLIB', cmd)
                                             + f'{field} version {version}\n')
            for line in ['otool: unexpected diagnostic', 'second image:',
                         'compatibility version 1.0.0: warning', 'current versions 1.0.0']:
                with self.subTest(cmd=cmd, line=line), self.assertRaisesRegex(
                        closure.Failure, 'unexpected report header or diagnostic'):
                    closure.parse_report(DYLIB_REPORT.replace('LC_LOAD_DYLIB', cmd) + line + '\n')

    def test_malformed_command_structure(self):
        for fields in [
                'cmdsize 24\n',
                'cmd LC_UUID\ncmd LC_UUID\ncmdsize 24\n',
                'cmd LC_UUID\ncmd garbage\ncmdsize 24\n',
                'cmd LC_UUID\ncmdsize 24\ncmdsize 24\n',
                'cmd LC_UUID\ncmdsize 24\ncmdsize invalid\n',
                *('cmd LC_UUID\ncmdsize ' + size + '\n'
                  for size in ['invalid', '-8', '0', '4', '9', '24 extra']),
                'cmd LC_LOAD_DYLIB\ncmdsize 32\n',
                'cmd LC_RPATH\ncmdsize 32\n',
                'cmd LC_LOAD_DYLIB\ncmdsize 32\nname x (offset 24)\nname y (offset 24)\n',
        ]:
            text = 'file:\nLoad command 0\n' + ''.join(
                ' ' + line + '\n' for line in fields.splitlines())
            with self.subTest(text=text), self.assertRaises(closure.Failure):
                closure.parse_report(text)

    def test_unknown_command_with_cmdsize_rejected(self):
        for cmd in ['LC_BOGUS', 'LC_FUTURE', 'LC_DYLD_ENVIRONMENT',
                    'LC_PREBOUND_DYLIB', 'LC_FILESET_ENTRY']:
            with self.subTest(cmd=cmd), self.assertRaisesRegex(
                    closure.Failure, 'unsupported.*' + cmd):
                closure.parse_report(f'file:\nLoad command 0\n cmd {cmd}\n cmdsize 32\n')

    def test_fail_closed_reports(self):
        for text in ['', 'file:\n', 'file:\n\t/usr/lib/libSystem.B.dylib (compatibility version 1.0.0)\n',
                     'file:\nLoad command 0\n cmd LC_LOAD_DYLIB\n cmdsize 32\n',
                     'file:\nLoad command 0\n cmd LC_DYLD_ENVIRONMENT\n cmdsize 32\n',
                     'file:\nLoad command 0\n cmd LC_PREBOUND_DYLIB\n cmdsize 32\n',
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
