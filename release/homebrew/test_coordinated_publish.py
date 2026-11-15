"""Offline only. Discovery: python3 -m unittest discover -s release/homebrew -p 'test_*.py'."""
import base64
import copy
import io
import json
from pathlib import Path
import subprocess
import sys
import unittest
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
        self.fake = Fake()
        self.plan = plan_for()

    def publish(self):
        return p.publish(self.plan, SECRET, self.fake)

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


class OldCandidateRegressions(unittest.TestCase):
    def test_read_only_candidate_reproductions(self):
        # A subprocess prevents candidate imports from contaminating discovery.
        candidate = Path('/Users/bryancostanich/git_repos/cercano-ai/Cercano/.worktrees/homebrew-coordinated/release/homebrew')
        if not candidate.is_dir():
            self.skipTest('read-only historical candidate unavailable')
        script = r'''
import formula_set as f
import update_tap as u
f.PUBLICATION_ACCEPTED = True
wanted = f.expected('0.20.4', 'a' * 64)
# The old non-force ref update accepts an ancestor rewind: generated commit
# is a descendant of that ancestor, not a compare-and-swap of reviewed head.
head = 'reviewed-descendant'
ancestor = 'rewound-ancestor'
state = [head]
def snapshot(token):
    return state[0], 'tree', {}
u.snapshot = snapshot
seen = []
def api(method, path, token, payload=None):
    seen.append((method, payload))
    if path.endswith('/git/trees'): return {'sha':'tree-new'}
    if path.endswith('/git/commits'):
        state[0] = ancestor
        return {'sha':'commit-descending-from-reviewed-head'}
    assert method == 'PATCH' and payload['force'] is False
    # Git non-force allows ancestor -> descendant, even after rewind.
    return {'object': {'sha': payload['sha']}}
u.api = api
assert u.publish_set(wanted, '0.20.4', 'a' * 64, 'offline') == 'updated'
assert state[0] == ancestor
print('stale-rewind-reproduced')
# A comment containing the legacy install substring is classified as legacy.
text = 'version "0.20.3"\nsha256 "' + 'b'*64 + '"\nurl "https://invalid.example"\n#     bin.install "bin/cercano", "bin/cercano-cli"\n'
assert f.decision({'cercano.rb':text}, wanted, '0.20.4', 'a'*64) == 'update'
print('counterfeit-legacy-reproduced')
# Candidate lets a diagnostic sentinel from injected transport escape.
def failing(*args, **kwargs): raise OSError('DIAGNOSTIC_SENTINEL')
u.snapshot = failing
try: u.publish_set(wanted, '0.20.4', 'a'*64, 'offline')
except OSError as error: assert 'DIAGNOSTIC_SENTINEL' in str(error)
else: raise AssertionError('expected diagnostic propagation')
print('diagnostic-sentinel-reproduced')
'''
        result = subprocess.run([sys.executable, '-c', script], cwd=candidate,
                                capture_output=True, text=True, timeout=20)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.splitlines(), ['stale-rewind-reproduced',
            'counterfeit-legacy-reproduced', 'diagnostic-sentinel-reproduced'])


if __name__ == '__main__':
    unittest.main()
