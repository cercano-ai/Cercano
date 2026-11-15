"""Offline only. Discovery: python3 -m unittest discover -s release/homebrew -p 'test_*.py'."""
import base64
import copy
import io
import json
from pathlib import Path
import unittest
import tempfile

import prepare_promotion as preparation
import render_formula as renderer
from test_render_formula import write_archive, VERSION
from unittest.mock import patch

import coordinated_publish as p

BASE, NEXT, OTHER = '1' * 40, '2' * 40, '3' * 40
SECRET = 'DIAGNOSTIC_SENTINEL_https://secret.invalid/token'


def plan_for(before=None):
    before = before or dict.fromkeys(p.PATHS)
    desired = p.expected('0.20.4', 'a' * 64)
    return {'schema': 1, 'version': '0.20.4', 'archive_sha256': 'a' * 64,
            'tap_base_sha': BASE, 'files': [
                {'path': path, 'content': desired[path].decode(),
                 'before_sha256': p.sha(before[path]) if before[path] is not None else None,
                 'after_sha256': p.sha(desired[path])} for path in p.PATHS]}


ARTIFACTS = Path(__file__).resolve().parents[2] / 'efforts/homebrew-native-migration-20261115/artifacts'


def historical_before():
    return {p.PATHS[0]: (ARTIFACTS / 'historical-cercano.rb').read_bytes(),
            p.PATHS[1]: None, p.PATHS[2]: None}


def historical_plan(before=None, base=p.REVIEWED_TRANSITION[0],
                    version=p.REVIEWED_TRANSITION[2], digest=p.REVIEWED_TRANSITION[3]):
    before = historical_before() if before is None else before
    desired = p.expected(version, digest)
    return {'schema': 1, 'version': version, 'archive_sha256': digest,
            'tap_base_sha': base, 'files': [
                {'path': path, 'content': desired[path].decode(),
                 'before_sha256': p.sha(before[path]) if before[path] is not None else None,
                 'after_sha256': p.sha(desired[path])} for path in p.PATHS]}


class Fake:
    def __init__(self):
        self.head = BASE
        self.contents = dict.fromkeys(p.PATHS)
        self.parents = []
        self.message = 'old'
        self.calls = []
        self.mode = 'success'
        self.after_read_failure = False
        self.race = None

    def __call__(self, query, variables, token):
        self.calls.append((query, copy.deepcopy(variables), token))
        if self.after_read_failure and query != p.MUTATION:
            raise TimeoutError(SECRET)
        if query == p.HEAD:
            return {'data': {'repository': {'ref': {'target': {
                'oid': self.head, 'message': self.message,
                'parents': {'nodes': [{'oid': x} for x in self.parents]}}}}}}
        if query == p.FILES:
            objects = {alias: None if self.contents[path] is None else
                {'text': self.contents[path].decode(), 'byteSize': len(self.contents[path])}
                for alias, path in zip('abc', p.PATHS)}
            objects['d'] = {'entries': [
                {'name': path.split('/')[1], 'mode': 33188, 'type': 'blob'}
                for path in p.PATHS if self.contents[path] is not None]}
            return {'data': {'repository': objects}}
        assert query == p.MUTATION
        args = variables['input']
        if self.race:
            self.head = self.race
        if args['expectedHeadOid'] != self.head:
            return {'errors': [{'message': SECRET}]}
        if self.mode not in ('timeout-before', 'malformed-before'):
            self.parents = [self.head]
            self.head = NEXT
            self.message = args['message']['headline']
            self.contents = {x['path']: base64.b64decode(x['contents'])
                             for x in args['fileChanges']['additions']}
        if self.mode == 'read-failure':
            self.after_read_failure = True
        if self.mode in ('timeout-before', 'lost'):
            raise TimeoutError(SECRET)
        if self.mode in ('malformed-before', 'malformed-after'):
            return {'data': {'unexpected': SECRET}}
        return {'data': {'createCommitOnBranch': {'commit': {'oid': NEXT}}}}

    @property
    def mutations(self):
        return [v['input'] for q, v, _ in self.calls if q == p.MUTATION]


class PublisherTests(unittest.TestCase):
    def setUp(self):
        self.gate = patch.object(p, 'PUBLICATION_ACCEPTED', True)
        self.gate.start()
        self.addCleanup(self.gate.stop)
        # Legacy state-machine tests isolate the separately tested network gate.
        self.provenance = patch.object(p.pinned_provenance, 'validate', return_value={})
        self.provenance_mock = self.provenance.start()
        self.addCleanup(self.provenance.stop)
        self.fake = Fake()
        self.plan = plan_for()

    def publish(self):
        return p.publish(self.plan, SECRET, self.fake)

    def historical(self, before=None, **kwargs):
        self.fake = Fake()
        self.fake.contents = historical_before() if before is None else before
        self.plan = historical_plan(self.fake.contents, **kwargs)
        self.fake.head = self.plan['tap_base_sha']

    def test_provenance_failure_precedes_any_tap_call(self):
        self.provenance_mock.side_effect = RuntimeError(SECRET)
        with self.assertRaisesRegex(p.Failure, '^Pinned release provenance unavailable; publication refused$'):
            self.publish()
        self.assertEqual(self.fake.calls, [])
        self.provenance_mock.assert_called_once_with(self.plan['version'], self.plan['archive_sha256'])

    def test_exact_historical_transition_atomic_and_reconciled(self):
        before = historical_before()
        self.assertEqual(len(before[p.PATHS[0]]), 2239)
        self.assertEqual(tuple(p.sha(before[x]) if before[x] is not None else None
                               for x in p.PATHS), p.REVIEWED_TRANSITION[1])
        for mode in ('success', 'lost', 'malformed-after'):
            with self.subTest(mode=mode):
                self.historical()
                self.fake.mode = mode
                self.assertEqual(self.publish(), 'updated' if mode == 'success' else 'reconciled')
                self.assertEqual(self.publish(), 'reconciled')
                self.assertEqual(len(self.fake.mutations), 1)
                mutation = self.fake.mutations[0]
                self.assertEqual(mutation['expectedHeadOid'], p.REVIEWED_TRANSITION[0])
                self.assertEqual(mutation['branch'], {
                    'repositoryNameWithOwner': p.REPOSITORY, 'branchName': 'main'})
                self.assertEqual(set(mutation['fileChanges']), {'additions'})
                self.assertEqual(self.fake.contents, p.expected(*p.REVIEWED_TRANSITION[2:]))
                self.assertEqual([x['path'] for x in mutation['fileChanges']['additions']], list(p.PATHS))

    def test_historical_transition_never_retries_uncertain_mutation(self):
        for mode in ('timeout-before', 'malformed-before', 'read-failure'):
            self.historical()
            self.fake.mode = mode
            with self.assertRaises(p.Failure):
                self.publish()
            self.assertEqual(len(self.fake.mutations), 1)

    def test_historical_altered_bytes_even_with_fresh_before_hash_refused(self):
        original = historical_before()[p.PATHS[0]]
        for content in (original + b' ', original.replace(b'\n', b'\r\n'),
                        b'# counterfeit version "0.25.2"\n' + original):
            before = historical_before()
            before[p.PATHS[0]] = content
            self.historical(before)
            with self.assertRaises(p.Failure):
                self.publish()
            self.assertEqual(self.fake.mutations, [])
        # A reviewed hash is also checked against actual snapshot bytes.
        self.historical()
        self.fake.contents[p.PATHS[0]] = original + b' '
        with self.assertRaisesRegex(p.Failure, 'Before files differ'):
            self.publish()
        self.assertEqual(self.fake.mutations, [])

    def test_historical_wrong_base_destination_digest_or_partial_presence_refused(self):
        for kwargs in ({'base': BASE}, {'version': '0.27.1'},
                       {'version': '0.25.1'}, {'version': '0.25.2'},
                       {'digest': 'b' * 64}):
            with self.subTest(kwargs=kwargs):
                self.historical(**kwargs)
                with self.assertRaises(p.Failure):
                    self.publish()
                self.assertEqual(self.fake.mutations, [])
        for path in p.PATHS[1:]:
            before = historical_before()
            before[path] = p.expected(*p.REVIEWED_TRANSITION[2:])[path]
            self.historical(before)
            with self.assertRaises(p.Failure):
                self.publish()
            self.assertEqual(self.fake.mutations, [])

    @unittest.skipUnless((ARTIFACTS / 'cercano-0.27.0-darwin-arm64.tar.gz').is_file(),
                         'local release archive required; never downloaded by tests')
    def test_actual_historical_prepare_to_publish_fake_api(self):
        archive = ARTIFACTS / 'cercano-0.27.0-darwin-arm64.tar.gz'
        base, _, version, digest = p.REVIEWED_TRANSITION
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            formulas = root / 'rendered'
            formulas.mkdir()
            renderer.build(['--version', version, '--sha256', digest,
                            '--archive', str(archive), '--output', str(formulas / 'cercano.rb')])
            tap = root / 'tap'
            (tap / 'Formula').mkdir(parents=True)
            old = historical_before()[p.PATHS[0]]
            (tap / p.PATHS[0]).write_bytes(old)
            self.plan = preparation.prepare(version, digest, archive, formulas, tap, base)
            self.fake.head, self.fake.contents = base, historical_before()
            self.assertEqual(self.publish(), 'updated')
            self.assertEqual(len(self.fake.mutations), 1)
            self.assertEqual((tap / p.PATHS[0]).read_bytes(), old)
            self.assertFalse((tap / p.PATHS[1]).exists())
            self.assertFalse((tap / p.PATHS[2]).exists())

    def test_disabled_no_network_even_with_bad_plan(self):
        with patch.object(p, 'PUBLICATION_ACCEPTED', False):
            with self.assertRaisesRegex(p.Failure, 'disabled'):
                p.publish(None, SECRET, self.fake)
        self.assertEqual(self.fake.calls, [])

    def test_boundary_gate_rechecked(self):
        original = self.fake
        def injected(q, v, t):
            result = original(q, v, t)
            if q == p.FILES:
                p.PUBLICATION_ACCEPTED = False
            return result
        with self.assertRaisesRegex(p.Failure, 'disabled'):
            p.publish(self.plan, SECRET, injected)
        self.assertEqual(self.fake.mutations, [])

    def test_one_atomic_mutation_expected_head_and_exact_bytes(self):
        self.assertEqual(self.publish(), 'updated')
        self.assertEqual(len(self.fake.mutations), 1)
        mutation = self.fake.mutations[0]
        self.assertEqual(mutation['expectedHeadOid'], BASE)
        self.assertEqual(set(mutation['fileChanges']), {'additions'})
        self.assertEqual(self.fake.contents, p.expected('0.20.4', 'a' * 64))
        self.assertEqual([x['path'] for x in mutation['fileChanges']['additions']], list(p.PATHS))

    def test_advance_and_rewind_require_review(self):
        for changed in (NEXT, OTHER):
            with self.subTest(changed=changed):
                self.fake = Fake()
                self.fake.head = changed
                with self.assertRaisesRegex(p.Failure, 'fresh review'):
                    self.publish()
                self.assertEqual(self.fake.mutations, [])

    def test_race_advance_and_rewind_cannot_retry_at_new_base(self):
        for head in (NEXT, OTHER):
            self.fake = Fake()
            self.fake.race = head
            with self.assertRaises(p.Failure):
                self.publish()
            self.assertEqual(len(self.fake.mutations), 1)
            self.assertTrue(all(x is None for x in self.fake.contents.values()))

    def test_exact_reviewed_before_revalidated(self):
        self.fake.contents = p.expected('0.20.3', 'b' * 64)
        self.plan = plan_for(self.fake.contents)
        self.assertEqual(self.publish(), 'updated')
        self.fake = Fake()
        with self.assertRaisesRegex(p.Failure, 'Before files differ'):
            self.publish()
        self.assertEqual(self.fake.mutations, [])

    def test_missing_invalid_partial_before_plans_rejected_without_writes(self):
        cases = []
        missing = plan_for(); del missing['files'][0]['before_sha256']; cases.append(missing)
        invalid = plan_for(); invalid['files'][0]['before_sha256'] = 'bogus'; cases.append(invalid)
        partial = plan_for(); partial['files'].pop(); cases.append(partial)
        no_base = plan_for(); no_base['tap_base_sha'] = None; cases.append(no_base)
        duplicate = plan_for(); duplicate['files'][1] = duplicate['files'][0]; cases.append(duplicate)
        tampered = plan_for(); tampered['files'][0]['content'] += SECRET; cases.append(tampered)
        for plan in cases:
            with self.subTest(plan=plan):
                with self.assertRaises(p.Failure):
                    p.publish(plan, SECRET, self.fake)
                self.assertEqual(self.fake.calls, [])

    def test_exact_partial_or_counterfeit_before_is_not_migration_authority(self):
        counterfeit = (b'version "0.20.3"\nsha256 "' + b'b' * 64 +
                       b'"\nurl "https://example.invalid"\n'
                       b'#     bin.install "bin/cercano", "bin/cercano-cli"\n')
        for before in ({p.PATHS[0]: counterfeit, p.PATHS[1]: None, p.PATHS[2]: None},
                       dict.fromkeys(p.PATHS, counterfeit)):
            self.fake = Fake(); self.fake.contents = before
            self.plan = plan_for(before)
            with self.assertRaises(p.Failure):
                self.publish()
            self.assertEqual(self.fake.contents, before)
            self.assertEqual(self.fake.mutations, [])

    def test_real_prepared_monolith_plan_refused_even_test_enabled(self):
        # Real preparer, inert archive, historical-shaped DSL; not an old checkout.
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            archive, digest = write_archive(root)
            formulas = root / 'rendered'
            formulas.mkdir()
            renderer.build(['--version', VERSION, '--sha256', digest,
                            '--archive', str(archive), '--output', str(formulas / 'cercano.rb')])
            tap = root / 'tap'
            (tap / 'Formula').mkdir(parents=True)
            monolith = (b'class Cercano < Formula\n'
                        b'  url "https://example.invalid/old"\n'
                        b'  version "1.2.2"\n  sha256 "' + b'b' * 64 + b'"\n'
                        b'  def install\n'
                        b'    bin.install "bin/cercano", "bin/cercano-cli"\n'
                        b'  end\nend\n')
            (tap / 'Formula/cercano.rb').write_bytes(monolith)
            plan = preparation.prepare(VERSION, digest, archive, formulas, tap, BASE)
            self.assertEqual(plan['files'][0]['before_sha256'], p.sha(monolith))
            self.assertEqual([x['action'] for x in plan['files']], ['update'] * 3)
            self.fake.contents[p.PATHS[0]] = monolith
            before = self.fake.contents.copy()
            self.assertTrue(p.PUBLICATION_ACCEPTED)  # test-local gate only
            with self.assertRaisesRegex(p.Failure, 'Partial before set refused'):
                p.publish(plan, SECRET, self.fake)
            self.assertEqual(self.fake.contents, before)
            self.assertTrue(self.fake.calls)  # Reached before-state validation, not gate refusal.
            self.assertEqual(self.fake.mutations, [])
            self.assertEqual((tap / 'Formula/cercano.rb').read_bytes(), monolith)

    def test_same_version_conflict_and_downgrade(self):
        for version in ('0.20.4', '0.20.5'):
            self.fake = Fake(); self.fake.contents = p.expected(version, 'b' * 64)
            self.plan = plan_for(self.fake.contents)
            with self.assertRaises(p.Failure):
                self.publish()
            self.assertEqual(self.fake.mutations, [])

    def test_unchanged_exact_set_at_reviewed_base(self):
        self.fake.contents = p.expected('0.20.4', 'a' * 64)
        self.plan = plan_for(self.fake.contents)
        self.assertEqual(self.publish(), 'unchanged')
        self.assertEqual(self.fake.mutations, [])

    def test_lost_response_and_same_operation_again(self):
        for mode in ('lost', 'malformed-after'):
            self.fake = Fake(); self.fake.mode = mode
            self.assertEqual(self.publish(), 'reconciled')
            self.assertEqual(self.publish(), 'reconciled')
            self.assertEqual(len(self.fake.mutations), 1)

    def test_exact_files_alone_do_not_prove_our_operation(self):
        self.publish()
        for parents, message in (([OTHER], self.fake.message), ([BASE], 'unrelated')):
            self.fake.parents, self.fake.message = parents, message
            with self.assertRaises(p.Failure):
                self.publish()
        self.assertEqual(len(self.fake.mutations), 1)

    def test_timeout_malformed_read_failure_no_false_success_no_secret(self):
        for mode in ('timeout-before', 'malformed-before', 'read-failure'):
            self.fake = Fake(); self.fake.mode = mode
            with self.assertRaises(p.Failure) as caught:
                self.publish()
            self.assertNotIn(SECRET, str(caught.exception))
            self.assertEqual(len(self.fake.mutations), 1)

    def test_read_failure_and_oversize_never_mutate_or_leak(self):
        for response in ({'errors': [{'message': SECRET}]},
                         {'data': {'x': SECRET * p.LIMIT}}, {'data': {}}):
            with self.assertRaises(p.Failure) as caught:
                p.publish(self.plan, SECRET, lambda *args: response)
            self.assertNotIn(SECRET, str(caught.exception))

    def test_nonordinary_or_missing_commit_evidence_refused(self):
        self.fake.contents = p.expected('0.20.3', 'b' * 64)
        self.plan = plan_for(self.fake.contents)
        for kind in ('mode', 'parents', 'size'):
            def injected(q, v, t):
                result = self.fake(q, v, t)
                if kind == 'mode' and q == p.FILES:
                    result['data']['repository']['d']['entries'][0]['mode'] = 40960
                if kind == 'size' and q == p.FILES:
                    result['data']['repository']['a']['byteSize'] += 1
                if kind == 'parents' and q == p.HEAD:
                    del result['data']['repository']['ref']['target']['parents']
                return result
            with self.assertRaises(p.Failure):
                p.publish(self.plan, SECRET, injected)
        self.assertEqual(self.fake.mutations, [])

    def test_transport_credentials_only_in_header_and_redacted(self):
        captured = []
        class Opener:
            def open(self, request, **kwargs):
                captured.append(request)
                raise OSError(SECRET)
        with patch.object(p.urllib.request, 'build_opener', return_value=Opener()):
            with self.assertRaises(p.Failure) as caught:
                p.api(p.HEAD, {}, SECRET)
        request = captured[0]
        self.assertEqual(request.get_header('Authorization'), 'Bearer ' + SECRET)
        self.assertNotIn(SECRET, request.full_url)
        self.assertNotIn(SECRET.encode(), request.data)
        self.assertNotIn(SECRET, str(caught.exception))


class CLITests(unittest.TestCase):
    def invoke(self, args):
        out, err = io.StringIO(), io.StringIO()
        with patch('sys.stdout', out), patch('sys.stderr', err):
            status = p.main(args)
        return status, out.getvalue(), err.getvalue()

    def test_disabled_before_plan_credentials_or_network_reads(self):
        self.assertFalse(p.PUBLICATION_ACCEPTED)
        class NoCredentialRead(dict):
            def get(self, key, default=None):
                if key == 'HOMEBREW_TAP_TOKEN':
                    raise AssertionError('credential read')
                return super().get(key, default)
        with patch.object(Path, 'open', side_effect=AssertionError('file read')), \
             patch.object(p.os, 'environ', NoCredentialRead(p.os.environ)), \
             patch.object(p, 'api', side_effect=AssertionError('network')):
            self.assertEqual(self.invoke(['--plan', SECRET]),
                             (1, '', 'Publication disabled: acceptance pending.\n'))

    def test_no_overrides_or_abbreviations(self):
        for flag in ('--force', '--enable', '--repository', '--branch', '--token', '--pla'):
            with self.subTest(flag=flag), patch('sys.stderr', io.StringIO()) as err:
                with self.assertRaises(SystemExit) as caught:
                    p.main(['--plan', 'unused', flag, SECRET])
                self.assertEqual(caught.exception.code, 2)
                self.assertEqual(err.getvalue(), 'Publication refused: invalid command line.\n')

    def test_strict_json_rejections_no_publication_no_input_leaks(self):
        bodies = [b'{"schema":1,"schema":1}', b'{"files":[{"x":1,"x":2}]}',
                  b'{} {}', b'{}' + SECRET.encode(), b' ' * (p.LIMIT + 1),
                  b'{"x":NaN}', b'{"x":Infinity}', b'\xff', b'[] trailing']
        with tempfile.TemporaryDirectory() as tmp, \
             patch.object(p, 'PUBLICATION_ACCEPTED', True), \
             patch.object(p, 'publish') as publish:
            path = Path(tmp) / 'plan.json'
            for body in bodies:
                path.write_bytes(body)
                result = self.invoke(['--plan', str(path)])
                self.assertEqual(result, (1, '',
                    'Publication refused: invalid inputs or unconfirmed publication; no retry attempted.\n'))
            publish.assert_not_called()

    def test_cli_historical_plan_uses_real_publisher_with_fake_api(self):
        original = p.publish
        fake = Fake()
        fake.head, fake.contents = p.REVIEWED_TRANSITION[0], historical_before()
        with tempfile.TemporaryDirectory() as tmp, \
             patch.object(p, 'PUBLICATION_ACCEPTED', True), \
             patch.dict(p.os.environ, {'HOMEBREW_TAP_TOKEN': SECRET}), \
             patch.object(p.pinned_provenance, 'validate', return_value={'publication_authorized': False}), \
             patch.object(p, 'publish', side_effect=lambda plan, token: original(plan, token, fake)):
            path = Path(tmp) / 'plan.json'
            path.write_text(json.dumps(historical_plan()))
            self.assertEqual(self.invoke(['--plan', str(path)]),
                             (0, 'Publication confirmed.\n', ''))
            self.assertEqual(len(fake.mutations), 1)
            self.assertTrue(all(token == SECRET for _, _, token in fake.calls))

    def test_env_token_and_fixed_outputs(self):
        with tempfile.TemporaryDirectory() as tmp, \
             patch.object(p, 'PUBLICATION_ACCEPTED', True), \
             patch.dict(p.os.environ, {'HOMEBREW_TAP_TOKEN': SECRET}):
            path = Path(tmp) / 'plan.json'
            path.write_text(json.dumps(plan_for()))
            for result in ('updated', 'unchanged', 'reconciled'):
                with patch.object(p, 'publish', return_value=result) as publish:
                    self.assertEqual(self.invoke(['--plan', str(path)]),
                                     (0, 'Publication confirmed.\n', ''))
                    publish.assert_called_once_with(plan_for(), SECRET)
            with patch.object(p, 'publish', side_effect=OSError(SECRET)):
                status, out, err = self.invoke(['--plan', str(path)])
                self.assertEqual(status, 1)
                self.assertEqual(out, '')
                self.assertNotIn(SECRET, err)


if __name__ == '__main__':
    unittest.main()
