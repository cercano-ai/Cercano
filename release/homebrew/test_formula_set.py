import base64
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import formula_set as f
import update_tap as u
import promote_to_tap as p

D = 'a' * 64
V = '1.2.3'


class SetTest(unittest.TestCase):
    def test_review_missing_tampered_mixed(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            desired = f.expected(V, D)
            for name, text in desired.items():
                (root / name).write_text(text)
            self.assertEqual(f.reviewed(root, V, D), desired)
            for bad in (None, desired['cercano-cli.rb'] + '# injected',
                        f.expected('1.2.2', D)['cercano-cli.rb']):
                path = root / 'cercano-cli.rb'
                if bad is None:
                    path.unlink()
                else:
                    path.write_text(bad)
                with self.assertRaises((OSError, f.r.ValidationError)):
                    f.reviewed(root, V, D)

    def test_decisions(self):
        desired = f.expected(V, D)
        self.assertEqual(f.decision(desired, desired, V, D), 'unchanged')
        self.assertEqual(f.decision({}, desired, V, D), 'update')
        old = f.expected('1.2.2', D)
        self.assertEqual(f.decision(old, desired, V, D), 'update')
        self.assertEqual(f.decision({'cercano.rb': old['cercano.rb'] + '\n    bin.install "bin/cercano", "bin/cercano-cli"\n'}, desired, V, D), 'update')
        for bad in (f.expected('1.2.4', D), f.expected(V, 'b'*64),
                    {'cercano-agent.rb': old['cercano-agent.rb']},
                    dict(old, **{'cercano-cli.rb': desired['cercano-cli.rb']}),
                    dict(old, **{'cercano.rb': old['cercano.rb'] + '# tampered'}),
                    {'cercano.rb': desired['cercano.rb']}):
            with self.assertRaises(f.r.ValidationError):
                f.decision(bad, desired, V, D)


class GitAPI:
    def __init__(self, mode=None):
        self.files = {'README.md': 'keep me'}
        self.mode = mode
        self.calls = []
        self.advances = 0
        self.head = 'head-0'
        self.tree = 'base-0'
        self.commits = {}
        self.trees = {}

    def __call__(self, method, path, token, payload=None):
        self.calls.append((method, path, payload))
        if '/git/ref/heads/' in path:
            return {'object': {'sha': self.head}}
        if method == 'GET' and '/git/commits/' in path:
            assert path.endswith('/' + self.head)
            return {'tree': {'sha': self.tree}}
        if method == 'GET' and '/git/trees/' in path:
            assert path.endswith('/' + self.tree + '?recursive=1')
            return {'truncated': False, 'tree': [
                {'path': n, 'type': 'blob', 'mode': '100644', 'sha': n}
                for n in self.files]}
        if '/git/blobs/' in path:
            name = path.split('/git/blobs/')[1]
            return {'encoding': 'base64', 'content': base64.b64encode(self.files[name].encode()).decode()}
        if method == 'POST' and path.endswith('/trees'):
            assert payload['base_tree'] == self.tree
            assert {e['path'] for e in payload['tree']} == {'Formula/' + n for n in f.NAMES}
            self.pending = dict(self.files)
            self.pending.update({e['path']: e['content'] for e in payload['tree']})
            if self.mode == 'tree-failure':
                raise u.Failure('tree failed', 500)
            tree = 'tree-' + str(len(self.trees))
            self.trees[tree] = self.pending
            return {'sha': tree}
        if method == 'POST' and path.endswith('/commits'):
            assert payload['parents'] == [self.head]
            commit = 'commit-' + str(len(self.commits))
            self.commits[commit] = payload
            return {'sha': commit}
        if method == 'PATCH':
            assert payload['force'] is False
            commit = self.commits[payload['sha']]
            if commit['parents'] != [self.head]:
                raise u.Failure('stale parent', 422)
            if self.mode == 'conflict':
                raise u.Failure('conflict', 409)
            if self.mode == 'network-before':
                raise u.Failure('connection lost before mutation')
            if self.mode == 'newer':
                self.files.update({'Formula/' + n: t for n, t in f.expected('1.2.4', D).items()})
                raise u.Failure('conflict', 409)
            if self.mode == 'auth':
                raise u.Failure('auth', 401)
            if self.mode == 'once':
                self.mode = None
                self.files['concurrent.txt'] = 'preserved'
                self.head = 'concurrent-head'
                self.tree = 'concurrent-tree'
                raise u.Failure('conflict', 422)
            self.files = self.trees[commit['tree']]
            self.head = payload['sha']
            self.tree = commit['tree']
            self.advances += 1
            if self.mode == 'lost':
                raise u.Failure('connection lost')
            return {'object': {'sha': self.head}}
        raise AssertionError((method, path))


class PublicationTest(unittest.TestCase):
    def test_verified_archive_complete_pipeline(self):
        from test_render_formula import write_archive
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            archive, digest = write_archive(root)
            for name, text in f.expected(V, digest).items():
                (root/name).write_text(text)
            fake = GitAPI()
            with patch.object(f, 'PUBLICATION_ACCEPTED', True), patch.object(u, 'api', fake), \
                    patch.object(u, 'published') as public:
                self.assertEqual(u.update(V, digest, archive, root/'cercano.rb', 'token'), 'updated')
                public.assert_called_once_with(V, archive.name, digest)
            self.assertEqual(fake.advances, 1)
            archive.write_bytes(b'corrupt')
            with self.assertRaises(u.Failure):
                u.prepare(V, digest, archive, root/'cercano.rb')


    def test_closed_gate_blocks_direct_mutation_boundary(self):
        with patch.object(u, 'api') as api:
            with self.assertRaisesRegex(f.r.ValidationError, 'publication is disabled'):
                u.publish_set(f.expected(V, D), V, D, 'token')
            api.assert_not_called()

    def test_one_advance_and_noop(self):
        fake = GitAPI()
        with patch.object(f, 'PUBLICATION_ACCEPTED', True), patch.object(u, 'api', fake):
            self.assertEqual(u.publish_set(f.expected(V, D), V, D, 'token'), 'updated')
            self.assertEqual(u.publish_set(f.expected(V, D), V, D, 'token'), 'unchanged')
        self.assertEqual(fake.advances, 1)
        self.assertEqual(fake.files['README.md'], 'keep me')

    def test_contention_and_lost_response(self):
        for mode, outcome in [('once', 'updated'), ('lost', 'reconciled')]:
            fake = GitAPI(mode)
            with patch.object(f, 'PUBLICATION_ACCEPTED', True), patch.object(u, 'api', fake):
                self.assertEqual(u.publish_set(f.expected(V, D), V, D, 'token'), outcome)
            self.assertEqual(fake.advances, 1)
            if mode == 'once':
                self.assertEqual(fake.files['concurrent.txt'], 'preserved')
                commits = list(fake.commits.values())
                self.assertEqual([c['parents'] for c in commits], [['head-0'], ['concurrent-head']])
                self.assertNotEqual(commits[0]['tree'], commits[1]['tree'])
                with self.assertRaisesRegex(u.Failure, 'stale parent'):
                    fake('PATCH', u.REF, 'token', {'sha': 'commit-0', 'force': False})

    def test_failures(self):
        for mode, patches in [('tree-failure', 0), ('conflict', 3), ('auth', 1), ('network-before', 1), ('newer', 1)]:
            fake = GitAPI(mode)
            with patch.object(f, 'PUBLICATION_ACCEPTED', True), patch.object(u, 'api', fake), self.assertRaises((u.Failure, f.r.ValidationError)):
                u.publish_set(f.expected(V, D), V, D, 'token')
            self.assertEqual(fake.advances, 0)
            self.assertEqual(sum(c[0] == 'PATCH' for c in fake.calls), patches)

    def test_public_validation_precedes_git(self):
        with patch.object(f, 'PUBLICATION_ACCEPTED', True), \
                patch.object(u, 'prepare', return_value=f.expected(V, D)), \
                patch.object(u, 'published', side_effect=u.Failure('unavailable')), \
                patch.object(u, 'publish_set') as publish:
            with self.assertRaises(u.Failure):
                u.update(V, D, Path('archive'), Path('cercano.rb'), 'token')
            publish.assert_not_called()


class LocalTest(unittest.TestCase):
    def test_private_index_preserves_staged_untracked_and_worktree(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            def git(*args):
                return subprocess.check_output(['git', '-C', tmp, *args])
            git('init', '-q')
            git('config', 'user.name', 'Fixture')
            git('config', 'user.email', 'fixture@example.invalid')
            (root/'README').write_text('committed')
            git('add', '.')
            git('commit', '-qm', 'fixture')
            head = git('rev-parse', 'HEAD')
            (root/'README').write_text('staged')
            git('add', '.')
            (root/'README').write_text('unstaged')
            (root/'untracked').write_text('keep')
            index = (root/'.git/index').read_bytes()
            status = git('status', '--porcelain')
            commit, changed, _ = p.stage_reviewed_set(root, f.expected(V, D), V, D)
            self.assertTrue(changed)
            self.assertEqual(git('rev-parse', 'HEAD'), head)
            self.assertEqual((root/'.git/index').read_bytes(), index)
            self.assertEqual(git('status', '--porcelain'), status)
            self.assertEqual(git('show', commit + ':README'), b'committed')
            for name, text in f.expected(V, D).items():
                self.assertEqual(git('show', commit + ':Formula/' + name).decode(), text)
            self.assertFalse((root/'Formula').exists())
