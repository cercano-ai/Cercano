#!/usr/bin/env python3
"""Offline fixtures only; no installed runtime or model is executed."""
import copy
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

import llama_baseline as baseline
import runtime_inventory


class BaselineTests(unittest.TestCase):
    def setUp(self):
        self.lock = baseline.load()

    def test_repository_lock_and_source_identity(self):
        self.assertEqual(self.lock['source']['revision'],
                         '74976e1aefd8d64845ae5510674b36079c855c0c')
        self.assertIsNone(self.lock['source']['upstream_tag'])
        self.assertEqual(self.lock['delivery']['qualified_platforms'], [])

    def test_reject_invalid_and_unearned_claims(self):
        cases = [
            (('schema_version',), True), (('schema_version',), 2),
            (('source', 'revision'), '74976e1ae'),
            (('source', 'revision'), 'a' * 40),
            (('source', 'revision'), '74976e1aefd8d64845ae5510674b36079c855c0c\n'),
            (('source', 'upstream_tag'), 'b9890'),
            (('source', 'source_archive_sha256'), 'a' * 64),
            (('observed_build', 'platform'), 'linux-x64'),
            (('observed_build', 'package_version'), ''),
            (('observed_build', 'built_as_bottle'), 1),
            (('delivery', 'bundled_in_current_archives'), True),
            (('delivery', 'qualified_platforms'), ['darwin-arm64']),
            (('delivery', 'artifact_sha256'), 'a' * 64),
            (('delivery', 'build_recipe'), 'imagined'),
        ]
        for keys, value in cases:
            with self.subTest(keys=keys, value=value):
                lock = copy.deepcopy(self.lock)
                target = lock
                for key in keys[:-1]:
                    target = target[key]
                target[keys[-1]] = value
                with self.assertRaises(ValueError):
                    baseline.validate(lock)

    def test_missing_extra_and_dependencies(self):
        for key in self.lock:
            lock = copy.deepcopy(self.lock)
            del lock[key]
            with self.assertRaises(ValueError):
                baseline.validate(lock)
        for change in ('extra', 'duplicate', 'missing', 'negative', 'bool'):
            lock = copy.deepcopy(self.lock)
            deps = lock['observed_build']['runtime_dependencies']
            if change == 'extra':
                lock['source']['invented'] = 'value'
            elif change == 'duplicate':
                deps[1] = deps[0]
            elif change == 'missing':
                deps.pop()
            else:
                deps[0]['revision'] = -1 if change == 'negative' else False
            with self.assertRaises(ValueError):
                baseline.validate(lock)

    def test_json_and_cli_fail_closed(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'lock.json'
            for text in ('{', '{"schema_version":1,"schema_version":1}', 'NaN'):
                path.write_text(text)
                with self.assertRaises(ValueError):
                    baseline.load(path)
            path.write_text(json.dumps(self.lock))
            command = [sys.executable, '-B', str(Path(baseline.__file__)), str(path)]
            self.assertEqual(subprocess.run(command, capture_output=True).returncode, 0)
            path.write_text('{}')
            self.assertNotEqual(subprocess.run(command, capture_output=True).returncode, 0)
            path.unlink()
            self.assertNotEqual(subprocess.run(command, capture_output=True).returncode, 0)

    def test_schema_unknown_keyword_refused(self):
        with self.assertRaises(ValueError):
            baseline.validate_schema({}, {'$ref': 'https://example.invalid/schema'})

    def test_release_gate_not_payload_claim(self):
        workflow = (baseline.ROOT.parents[1] / '.github/workflows/release-macos.yml').read_text()
        self.assertLess(workflow.index('python3 scripts/llama_baseline.py'),
                        workflow.index('- name: Import signing identity'))
        for platform in ('darwin-arm64', 'linux-x64', 'windows-x64'):
            self.assertFalse(any('llama' in p for p in runtime_inventory.payload_modes(platform)))
            inventory = json.loads(runtime_inventory.encode('1.2.3', platform, {'bin/cercano': (b'fixture', 0o755)}))
            self.assertEqual(set(inventory), {'schema_version', 'version', 'platform', 'members'})


if __name__ == '__main__':
    unittest.main()
