"""Offline review only: inert archives and private tap snapshots, no brew."""
import contextlib
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import prepare_promotion as p
import render_formula as r
import update_tap as u
import promote_to_tap as local
from test_render_formula import write_archive, VERSION


class PreparationTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.archive, self.digest = write_archive(self.root)
        self.formulas = self.root / 'rendered'
        self.formulas.mkdir()
        r.build(['--version', VERSION, '--sha256', self.digest,
                 '--archive', str(self.archive), '--output', str(self.formulas / 'cercano.rb')])
        self.tap = self.root / 'tap'
        (self.tap / 'Formula').mkdir(parents=True)

    def prepare(self):
        return p.prepare(VERSION, self.digest, self.archive, self.formulas, self.tap)

    def test_complete_set_and_no_io_side_effects(self):
        with patch.object(u, 'api') as api, patch.object(u, 'download') as download:
            plan = self.prepare()
            api.assert_not_called()
            download.assert_not_called()
        self.assertEqual([x['path'] for x in plan['files']], ['Formula/' + n for n in p.NAMES])
        self.assertEqual([x['action'] for x in plan['files']], ['update'] * 3)
        self.assertEqual(list((self.tap / 'Formula').iterdir()), [])
        self.assertEqual(plan['status'], 'review-only-publication-blocked')

    def test_snapshot_base_is_recorded_but_never_authorizes_publication(self):
        base = 'a' * 40
        plan = p.prepare(VERSION, self.digest, self.archive, self.formulas, self.tap, base)
        self.assertEqual(plan['tap_base_sha'], base)
        self.assertEqual(plan['status'], 'review-only-publication-blocked')
        self.assertIsNone(self.prepare()['tap_base_sha'])
        for invalid in ('main', 'abc123', 'A' * 40, 'a' * 41, '../tap'):
            with self.subTest(base=invalid), self.assertRaises(u.Failure):
                p.prepare(VERSION, self.digest, self.archive, self.formulas, self.tap, invalid)

    def test_identical_set_is_noop(self):
        for name in p.NAMES:
            (self.tap / 'Formula' / name).write_text((self.formulas / name).read_text())
        self.assertEqual([x['action'] for x in self.prepare()['files']], ['unchanged'] * 3)

    def test_monolith_transition_records_old_hash_not_mutation(self):
        old = 'url "https://example.invalid/old"\nversion "1.2.2"\nsha256 "' + '0' * 64 + '"\n'
        target = self.tap / 'Formula/cercano.rb'
        target.write_text(old)
        plan = self.prepare()
        self.assertEqual(plan['files'][0]['before_sha256'], p.sha(old))
        self.assertIn('explicit-operator-only', plan['migration_policy'])
        self.assertEqual(target.read_text(), old)

    def test_each_missing_tampered_or_symlink_component_refused(self):
        for name in p.NAMES:
            source = self.formulas / name
            text = source.read_text()
            for kind in ('missing', 'tampered', 'symlink'):
                with self.subTest(name=name, kind=kind):
                    source.unlink()
                    if kind == 'tampered':
                        source.write_text(text + '\n# altered\n')
                    elif kind == 'symlink':
                        source.symlink_to(self.archive)
                    with self.assertRaises(u.Failure):
                        self.prepare()
                    if source.exists() or source.is_symlink():
                        source.unlink()
                    source.write_text(text)

    def test_newer_and_same_version_conflicts_refused_for_every_formula(self):
        for name in p.NAMES:
            target = self.tap / 'Formula' / name
            desired = (self.formulas / name).read_text()
            for text in (desired.replace('version "1.2.3"', 'version "9.0.0"'),
                         desired.replace(self.digest, '0' * 64), desired + '\n# drift\n'):
                target.write_text(text)
                with self.assertRaises(u.Failure):
                    self.prepare()
                self.assertEqual(target.read_text(), text)
            target.unlink()

    def test_same_version_monolith_requires_review_not_silent_replacement(self):
        target = self.tap / 'Formula/cercano.rb'
        old = (self.formulas / 'cercano.rb').read_text() + '\n# historical monolith\n'
        target.write_text(old)
        with self.assertRaisesRegex(u.Failure, 'Same version has different formula'):
            self.prepare()
        self.assertEqual(target.read_text(), old)

    def test_cli_failure_emits_no_partial_plan_or_tap_changes(self):
        (self.tap / 'Formula/cercano-cli.rb').write_text('malformed historical formula')
        output, errors = io.StringIO(), io.StringIO()
        with contextlib.redirect_stdout(output), contextlib.redirect_stderr(errors):
            result = p.main(['--version', VERSION, '--sha256', self.digest,
                             '--archive', str(self.archive), '--formulas', str(self.formulas),
                             '--tap', str(self.tap)])
        self.assertEqual(result, 1)
        self.assertEqual(output.getvalue(), '')
        self.assertIn('Preparation refused', errors.getvalue())
        self.assertEqual(len(list((self.tap / 'Formula').iterdir())), 1)

    def test_archive_digest_refused_before_plan(self):
        self.archive.write_bytes(b'not the trusted archive')
        with self.assertRaisesRegex(u.Failure, 'checksum mismatch'):
            self.prepare()

    def test_tap_symlink_refused(self):
        (self.tap / 'Formula/cercano-cli.rb').symlink_to(self.root / 'absent')
        with self.assertRaises(u.Failure):
            self.prepare()

    def test_successful_plan_never_unlocks_publication(self):
        self.prepare()
        with patch.object(u, 'api') as api:
            with self.assertRaises(u.Failure):
                u.update(VERSION, self.digest, self.archive, self.formulas / 'cercano.rb', 'fixture')
            with self.assertRaises(local.PromotionError):
                local.promote(self.formulas / 'cercano.rb', self.tap, VERSION, True)
            api.assert_not_called()

    def test_cli_emits_complete_json(self):
        output = io.StringIO()
        with contextlib.redirect_stdout(output):
            result = p.main(['--version', VERSION, '--sha256', self.digest,
                             '--archive', str(self.archive), '--formulas', str(self.formulas),
                             '--tap', str(self.tap), '--tap-base-sha', 'b' * 40])
        self.assertEqual(result, 0)
        plan = json.loads(output.getvalue())
        self.assertEqual(len(plan['files']), 3)
        self.assertEqual(plan['tap_base_sha'], 'b' * 40)


if __name__ == '__main__':
    unittest.main()
