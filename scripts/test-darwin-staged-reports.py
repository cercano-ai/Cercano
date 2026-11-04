#!/usr/bin/env python3
"""Inert temporary bytes and injected host output; no native tools/payloads."""
import copy
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import darwin_loader_closure as closure
import darwin_staged_reports as fresh


def report(*edges):
    text = 'image:\nLoad command 0\n cmd LC_UUID\n cmdsize 24\n'
    for number, edge in enumerate(edges, 1):
        text += (f'Load command {number}\n cmd LC_LOAD_DYLIB\n cmdsize 64\n'
                 f' name {edge} (offset 24)\n')
    return text


class FreshTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name).resolve() / 'stage'
        self.inventory = {'executable': 'bin/server', 'backends': ['libexec/cpu'],
                          'images': ['bin/server', 'lib/core', 'libexec/cpu']}
        for name in self.inventory['images']:
            path = self.root / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(b'\xcf\xfa\xed\xfeINERT, NOT A NATIVE BINARY')
        self.reports = {'bin/server': report('@loader_path/../lib/core'),
                        'lib/core': report('/usr/lib/libSystem.B.dylib'),
                        'libexec/cpu': report('@executable_path/../lib/core')}
        self.calls = []

    def tool(self, command):
        self.calls.append(command)
        self.assertEqual(command[0], '/usr/bin/otool')
        name = str(Path(command[2]).relative_to(self.root))
        if command[1] == '-hv':
            kind = 'EXECUTE' if name == 'bin/server' else 'DYLIB'
            return ('image:\nMach header\n magic cputype cpusubtype caps filetype ncmds sizeofcmds flags\n'
                    f' MH_MAGIC_64 ARM64 ALL 0x00 {kind} 1 24 NOUNDEFS DYLDLINK\n')
        self.assertEqual(command[1], '-l')
        return self.reports[name]

    def collect(self, tool=None):
        return fresh.collect(self.root, self.inventory, 'arm64', tool or self.tool)

    def test_complete_success_feeds_real_closure(self):
        with patch.object(closure, 'validate', wraps=closure.validate) as validate:
            self.assertEqual(self.collect(), sorted(self.inventory['images']))
            validate.assert_called_once()
        self.assertEqual(len(self.calls), 6)

    def test_x86_64_success(self):
        def tool(command):
            return self.tool(command).replace('ARM64', 'X86_64')
        self.assertEqual(fresh.collect(self.root, self.inventory, 'x86_64', tool),
                         sorted(self.inventory['images']))

    def test_tool_failure_stops_collection(self):
        def tool(command):
            raise closure.Failure('stub host tool failed')
        with self.assertRaisesRegex(closure.Failure, 'stub host tool failed'):
            self.collect(tool)

    def test_missing_inventory(self):
        for key in self.inventory:
            data = copy.deepcopy(self.inventory)
            del data[key]
            with self.subTest(key=key), self.assertRaises(closure.Failure):
                fresh.collect(self.root, data, 'arm64', self.tool)
        for key in ['backends', 'images']:
            data = copy.deepcopy(self.inventory)
            data[key] = []
            with self.assertRaises(closure.Failure):
                fresh.collect(self.root, data, 'arm64', self.tool)
        self.assertEqual(self.calls, [])

    def test_wrong_paths_and_duplicate_aliases(self):
        for bad in ['../outside', '/absolute', 'bin/./server', 'missing', 42]:
            with self.subTest(bad=bad):
                self.inventory['images'].append(bad)
                with self.assertRaises(closure.Failure):
                    self.collect()
                self.inventory['images'].pop()
        outside = Path(self.tmp.name) / 'outside'
        outside.write_text('inert')
        (self.root / 'escape').symlink_to(outside)
        (self.root / 'alias').symlink_to('lib/core')
        for bad in ['escape', 'alias', 'lib/core']:
            self.inventory['images'].append(bad)
            with self.assertRaises(closure.Failure):
                self.collect()
            self.inventory['images'].pop()
        self.assertEqual(self.calls, [])

    def test_safe_internal_alias(self):
        path = self.root / 'lib/core'
        path.rename(self.root / 'lib/physical')
        path.symlink_to('physical')
        self.reports['lib/physical'] = self.reports['lib/core']
        self.assertIn('lib/physical', self.collect())

    def test_missing_root_or_dependency_inventory(self):
        for name in self.inventory['images'][:]:
            self.inventory['images'].remove(name)
            with self.subTest(name=name), self.assertRaisesRegex(closure.Failure, 'missing report'):
                self.collect()
            self.inventory['images'].append(name)

    def test_unsupported_bytes(self):
        for data in [b'inert text', b'\xca\xfe\xba\xbe', b'\xbe\xba\xfe\xca',
                     b'\xce\xfa\xed\xfe', b'']:
            (self.root / 'bin/server').write_bytes(data)
            with self.subTest(data=data), self.assertRaisesRegex(closure.Failure, 'unsupported/fat/non-Mach-O'):
                self.collect()
        self.assertEqual(self.calls, [])

    def test_unsupported_host_identity(self):
        for old, new in [('ARM64', 'X86_64'), ('MH_MAGIC_64', 'FAT_MAGIC'),
                         ('EXECUTE', 'OBJECT'), ('Mach header', 'diagnostic')]:
            def tool(command):
                return self.tool(command).replace(old, new)
            with self.subTest(new=new), self.assertRaises(closure.Failure):
                self.collect(tool)

    def test_mutation_during_collection(self):
        def tool(command):
            text = self.tool(command)
            if command[1] == '-l':
                Path(command[2]).write_bytes(b'changed')
            return text
        with self.assertRaisesRegex(closure.Failure, 'changed during collection'):
            self.collect(tool)

    def test_mutation_of_earlier_image_caught_at_end(self):
        def tool(command):
            text = self.tool(command)
            if command[1] == '-l' and command[2].endswith('/cpu'):
                (self.root / 'bin/server').write_bytes(b'changed later')
            return text
        with self.assertRaisesRegex(closure.Failure, 'changed by end'):
            self.collect(tool)

    def test_retargeted_alias_caught(self):
        path = self.root / 'lib/core'
        path.rename(self.root / 'lib/physical')
        path.symlink_to('physical')
        (self.root / 'lib/other').write_bytes((self.root / 'lib/physical').read_bytes())
        self.reports['lib/physical'] = self.reports['lib/core']
        def tool(command):
            text = self.tool(command)
            if command[1] == '-l' and command[2].endswith('/cpu'):
                path.unlink()
                path.symlink_to('other')
            return text
        with self.assertRaises(closure.Failure):
            self.collect(tool)

    def test_bad_collected_report_and_external_edge(self):
        for text in ['diagnostic', report('/opt/homebrew/lib/external')]:
            self.reports['bin/server'] = text
            with self.assertRaises(closure.Failure):
                self.collect()

    def test_host_errors_and_timeout(self):
        command = [fresh.OTOOL, '-l', '/inert']
        for error in [OSError('missing'), subprocess.TimeoutExpired(command, 10)]:
            with patch.object(fresh.subprocess, 'run', side_effect=error):
                with self.assertRaises(closure.Failure):
                    fresh.host_tool(command)
        for result in [subprocess.CompletedProcess(command, 1, b'', b'x' * 2000),
                       subprocess.CompletedProcess(command, 0, b'', b'warning'),
                       subprocess.CompletedProcess(command, 0, b'\xff', b''),
                       subprocess.CompletedProcess(command, 0, b'x' * (fresh.MAX_OUTPUT + 1), b'')]:
            with patch.object(fresh.subprocess, 'run', return_value=result) as run:
                with self.assertRaises(closure.Failure) as caught:
                    fresh.host_tool(command)
                self.assertLess(len(str(caught.exception)), 1000)
                self.assertEqual(run.call_args.kwargs['timeout'], 10)
                self.assertEqual(run.call_args.kwargs['env'], {'PATH': '/usr/bin:/bin', 'LC_ALL': 'C'})

    def test_cli_success_and_failure(self):
        inventory = Path(self.tmp.name) / 'inventory.json'
        inventory.write_text(json.dumps(self.inventory))
        args = ['--staged-root', str(self.root), '--image-inventory', str(inventory),
                '--architecture', 'arm64']
        collect = fresh.collect
        with patch.object(fresh, 'collect', side_effect=lambda root, inv, arch: collect(
                root, inv, arch, self.tool)):
            self.assertEqual(fresh.main(args), 0)
        inventory.write_text('{"images": [], "images": []}')
        self.assertEqual(fresh.main(args), 1)
        inventory.unlink()
        self.assertEqual(fresh.main(args), 1)


if __name__ == '__main__':
    unittest.main()
