"""Offline fake-network tests. Always build inert archives, never skip for local bytes."""
import copy
import io
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest
import zipfile
from unittest.mock import patch

import pinned_provenance as p
import test_render_formula as fixtures


def logs(upload=False):
    lines = [p.SOURCE, '  ref: refs/tags/v0.27.0',
             'Release source: refs/tags/v0.27.0 at ' + p.SOURCE,
             'Workflow: cercano-ai/Cercano/.github/workflows/release-macos.yml@refs/heads/fix/macos-release-runner at ' + p.WORKFLOW]
    if upload:
        lines += ['SHA256 digest of uploaded artifact zip is ' + p.OUTER_SHA,
                  'Artifact ' + p.NAME + '.zip successfully finalized. Artifact ID ' + str(p.ARTIFACT),
                  'Artifact ' + p.NAME + ' has been successfully uploaded! Final size is ' + str(p.OUTER_SIZE) + ' bytes. Artifact ID is ' + str(p.ARTIFACT),
                  'Artifact download URL: https://github.com/' + p.REPO + '/actions/runs/' + str(p.RUN) + '/artifacts/' + str(p.ARTIFACT)]
    return ''.join('2026-11-15T00:00:00.0000000Z ' + line + '\n' for line in lines).encode()


def job(name, job_id, attempt):
    steps = [{'name': n, 'status': 'completed', 'conclusion': 'success'}
             for n in p.COMMON + p.EXTRA[name]]
    if name == p.MAC:
        steps += [{'name': n, 'status': 'completed', 'conclusion': 'skipped'} for n in p.SKIPPED]
    return dict(name=name, id=job_id, run_id=p.RUN, run_attempt=attempt,
                head_sha=p.WORKFLOW, status='completed', conclusion='success', steps=steps,
                started_at=p.REUSED.get(name, (0, '2026-11-15T00:34:51Z', '2026-11-15T00:41:04Z'))[1],
                completed_at=p.REUSED.get(name, (0, '2026-11-15T00:34:51Z', '2026-11-15T00:41:04Z'))[2])


class BoundedProcessTests(unittest.TestCase):
    def run_child(self, code, limit, *, timeout=2, fails=False):
        children = []
        reads = []
        popen, read = p.subprocess.Popen, os.read

        def spawn(*args, **kwargs):
            self.assertIs(kwargs['stdout'], p.subprocess.PIPE)
            child = popen(*args, **kwargs)
            children.append(child)
            return child

        def consume(fd, size):
            chunk = read(fd, size)
            if children and fd == children[0].stdout.fileno():
                reads.append((size, len(chunk)))
            return chunk

        with patch.object(p.subprocess, 'Popen', side_effect=spawn), \
             patch.object(p.os, 'read', side_effect=consume):
            if fails:
                with self.assertRaisesRegex(p.Failure, '^Bounded evidence transport failed$'):
                    p.bounded_process([sys.executable, '-c', code], limit, timeout=timeout)
                result = None
            else:
                result = p.bounded_process([sys.executable, '-c', code], limit, timeout=timeout)
        self.assertEqual(len(children), 1)
        self.assertIsNotNone(children[0].returncode)  # waited/reaped direct child
        self.assertTrue(children[0].stdout.closed)
        self.assertLessEqual(sum(length for _, length in reads), limit + 1)
        self.assertTrue(all(0 < size <= min(65536, limit + 1) for size, _ in reads))
        return result, children[0], reads

    def test_eight_mib_output_four_byte_cap(self):
        # Actual subprocess: the former tempfile implementation spooled all 8 MiB.
        _, child, reads = self.run_child(
            "import sys; sys.stdout.buffer.write(b'x' * (8 * 1024 * 1024)); sys.stdout.flush()",
            4, fails=True)
        self.assertEqual(sum(length for _, length in reads), 5)
        self.assertLess(child.returncode, 0)

    def test_eof_exact_cap_empty_and_chunked_success(self):
        for size, limit in ((0, 0), (4, 4), (200000, 200000)):
            with self.subTest(size=size):
                result, child, _ = self.run_child(
                    f"import sys; sys.stdout.buffer.write(b'x' * {size})", limit)
                self.assertEqual(result, b'x' * size)
                self.assertEqual(child.returncode, 0)

    def test_nonzero_exit_even_after_valid_output(self):
        self.run_child("import sys; sys.stdout.write('1234'); sys.exit(7)", 4, fails=True)

    def test_timeout_on_blocked_read_and_eof_without_exit(self):
        for prefix in ('', 'import os; os.close(1); '):
            with self.subTest(prefix=prefix):
                _, child, _ = self.run_child(prefix + 'import time; time.sleep(60)',
                                              4, timeout=0.2, fails=True)
                self.assertLess(child.returncode, 0)

    def test_slow_trickle_does_not_reset_overall_deadline(self):
        self.run_child("import os, time\nwhile True:\n os.write(1, b'x'); time.sleep(0.02)",
                       100000, timeout=0.2, fails=True)


class EvidenceTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        # Synthetic content changes only byte pins in this test, not repository,
        # source, run, attempt, release, artifact or asset identity pins.
        with patch.object(fixtures, 'VERSION', p.VERSION), patch.object(fixtures, 'TOP', p.NAME):
            path, digest = fixtures.write_archive(self.tmp.name, name=p.ARCHIVE)
        self.inner = path.read_bytes()
        self.checksum = (digest + '  ' + p.ARCHIVE + '\n').encode()
        self.entries = {p.ARCHIVE: self.inner, p.ARCHIVE + '.sha256': self.checksum,
                        'cercano.rb': b'inert', 'cercano-agent.rb': b'inert', 'cercano-cli.rb': b'inert'}
        self.outer = self.zip_bytes()
        for key, value in dict(INNER_SHA=digest, INNER_SIZE=len(self.inner),
                               OUTER_SHA=p.sha(self.outer), OUTER_SIZE=len(self.outer),
                               WORKFLOW_DIGEST=p.sha(b'pinned workflow fixture')).items():
            patcher = patch.object(p, key, value)
            patcher.start()
            self.addCleanup(patcher.stop)
        release = dict(id=p.RELEASE, tag_name='v0.27.0', draft=False, prerelease=False,
                       assets=[dict(id=p.ASSET, name=p.ARCHIVE, state='uploaded', size=p.INNER_SIZE,
                                    digest='sha256:' + p.INNER_SHA, browser_download_url=p.PUBLIC + p.ARCHIVE)])
        run = dict(id=p.RUN, run_attempt=2, head_sha=p.WORKFLOW, event='workflow_dispatch',
                   path='.github/workflows/release-macos.yml', status='completed', conclusion='success',
                   repository={'full_name': p.REPO}, head_repository={'full_name': p.REPO})
        jobs = [job(n, i, 2) for n, i in p.JOBS.items()]
        old_jobs = [job(p.LINUX, 132631632452, 1), job(p.WINDOWS, 132631632597, 1),
                    dict(name=p.MAC)]
        self.data = {
            'releases/tags/v0.27.0': release, 'commits/v0.27.0': {'sha': p.SOURCE},
            'actions/runs/' + str(p.RUN) + '/attempts/2': run,
            'actions/runs/' + str(p.RUN) + '/attempts/2/jobs?per_page=100': {'total_count': 3, 'jobs': jobs},
            'actions/runs/' + str(p.RUN) + '/attempts/1/jobs?per_page=100': {'total_count': 3, 'jobs': old_jobs},
            'actions/artifacts/' + str(p.ARTIFACT): dict(id=p.ARTIFACT, name=p.NAME, expired=False,
                size_in_bytes=p.OUTER_SIZE, digest='sha256:' + p.OUTER_SHA,
                workflow_run={'id': p.RUN, 'head_sha': p.WORKFLOW}),
        }
        self.routes = {p.RAW: b'pinned workflow fixture',
                       p.PUBLIC + p.ARCHIVE: self.inner,
                       p.PUBLIC + p.ARCHIVE + '.sha256': self.checksum,
                       p.BASE + 'actions/artifacts/' + str(p.ARTIFACT) + '/zip': self.outer}
        for j in [j for j in jobs if j['name'] == p.MAC] + old_jobs[:2]:
            self.routes[p.BASE + 'actions/jobs/' + str(j['id']) + '/logs'] = logs(j['name'] == p.MAC)
        self.calls = []

    def zip_bytes(self):
        buffer = io.BytesIO()
        with zipfile.ZipFile(buffer, 'w') as archive:
            for name, content in self.entries.items():
                archive.writestr(name, content)
        return buffer.getvalue()

    def fetch(self, url, limit):
        self.calls.append(url)
        body = (json.dumps(self.data[url[len(p.BASE):]]).encode()
                if url.startswith(p.BASE) and url[len(p.BASE):] in self.data else self.routes[url])
        self.assertLessEqual(len(body), limit)
        return body

    def validate(self):
        return p.validate(p.VERSION, p.INNER_SHA, fetch=self.fetch)

    def test_complete_evidence_not_authority(self):
        result = self.validate()
        self.assertTrue(result['ci_evidence_validated'])
        for field in ('publication_authorized', 'clean_mac_qualified', 'apple_ticket_independently_repeated'):
            self.assertIs(result[field], False)
        self.assertEqual(result['run_attempt'], 2)

    def test_wrong_identity_status_and_attempt(self):
        for path, fields in list(self.data.items()):
            if not isinstance(fields, dict):
                continue
            for field in fields:
                if field in ('assets', 'jobs', 'workflow_run'):
                    continue
                with self.subTest(path=path, field=field):
                    original = copy.deepcopy(self.data)
                    self.data[path][field] = None
                    with self.assertRaises(p.Failure):
                        self.validate()
                    self.data = original
        with self.assertRaises(p.Failure):
            p.validate('0.27.1', p.INNER_SHA, fetch=self.fetch)
        with self.assertRaises(p.Failure):
            p.validate(p.VERSION, '0' * 64, fetch=self.fetch)

    def test_wrong_jobs_missing_duplicate_failed_steps(self):
        key = 'actions/runs/' + str(p.RUN) + '/attempts/2/jobs?per_page=100'
        original = copy.deepcopy(self.data[key])
        for field, value in [('id', 1), ('run_id', 1), ('run_attempt', 1),
                             ('head_sha', p.SOURCE), ('status', 'queued'), ('conclusion', 'failure')]:
            self.data[key]['jobs'][0][field] = value
            with self.assertRaises(p.Failure):
                self.validate()
            self.data[key] = copy.deepcopy(original)
        for name in p.COMMON + p.EXTRA[p.MAC] + p.SKIPPED:
            for mode in ('missing', 'duplicate', 'wrong'):
                self.data[key] = copy.deepcopy(original)
                mac = next(j for j in self.data[key]['jobs'] if j['name'] == p.MAC)
                step = next(s for s in mac['steps'] if s['name'] == name)
                if mode == 'missing':
                    mac['steps'].remove(step)
                elif mode == 'duplicate':
                    mac['steps'].append(copy.deepcopy(step))
                else:
                    step['conclusion'] = 'failure'
                with self.subTest(step=name, mode=mode), self.assertRaises(p.Failure):
                    self.validate()

    def test_wrong_asset_and_reused_prerequisite_evidence(self):
        release = self.data['releases/tags/v0.27.0']
        original = copy.deepcopy(release['assets'])
        for field in original[0]:
            release['assets'][0][field] = None
            with self.subTest(asset_field=field), self.assertRaises(p.Failure):
                self.validate()
            release['assets'] = copy.deepcopy(original)
        key = 'actions/runs/' + str(p.RUN) + '/attempts/1/jobs?per_page=100'
        previous = copy.deepcopy(self.data[key])
        for field in ('id', 'run_id', 'run_attempt', 'head_sha', 'status', 'conclusion'):
            self.data[key]['jobs'][0][field] = None
            with self.subTest(previous_field=field), self.assertRaises(p.Failure):
                self.validate()
            self.data[key] = copy.deepcopy(previous)

    def test_transport_is_bounded_and_credential_free(self):
        class Response(io.BytesIO):
            status = 200
            headers = {}
        opener = unittest.mock.Mock()
        for headers, body in (({'Content-Length': '20'}, b'a'),
                              ({'Content-Encoding': 'gzip'}, b'a'), ({}, b'12345')):
            response = Response(body)
            response.headers = headers
            opener.open.return_value = response
            with patch.object(p.urllib.request, 'build_opener', return_value=opener), \
                 self.assertRaises(p.Failure):
                p._download(p.PUBLIC + p.ARCHIVE, 4)
        opener.open.return_value = Response(b'1234')
        with patch.object(p.urllib.request, 'build_opener', return_value=opener), \
             patch.dict(os.environ,
                        {'GITHUB_TOKEN': 'DO-NOT-SEND', 'HTTPS_PROXY': 'https://bad.invalid'}):
            self.assertEqual(p._download(p.PUBLIC + p.ARCHIVE, 4), b'1234')
        request = opener.open.call_args.args[0]
        self.assertEqual(request.get_method(), 'GET')
        self.assertNotIn('Authorization', request.headers)
        self.assertNotIn('Cookie', request.headers)
        self.assertEqual(opener.open.call_args.kwargs['timeout'], 20)

    def test_late_blocking_read_including_eof_refuses(self):
        for returned in (b'', b'x'):
            clock = [0]
            class SlowResponse(io.BytesIO):
                status = 200
                headers = {}
                def read(self, size):
                    clock[0] = 1000
                    return returned
            opener = unittest.mock.Mock()
            opener.open.return_value = SlowResponse()
            with patch.object(p.urllib.request, 'build_opener', return_value=opener), \
                 patch.object(p.time, 'monotonic', side_effect=lambda: clock[0]), \
                 self.assertRaises(p.Failure):
                p._download(p.PUBLIC + p.ARCHIVE, 4)

    def test_process_deadline_kills_blocked_transport(self):
        # A real sleeping child is killed, not left to an inactivity timeout.
        with patch.object(p.time, 'monotonic', side_effect=[0, 1000]), \
             self.assertRaises(p.Failure):
            p.bounded_process([sys.executable, '-c', 'import time; time.sleep(60)'], 4)
        with self.assertRaises(p.Failure):
            p.bounded_process([sys.executable, '-c', 'print("oversized")'], 4)

    def test_only_fixed_actions_gets_use_gh(self):
        with patch.object(p, 'bounded_process', return_value=b'') as run:
            for path in p.AUTH_PATHS:
                p.download(p.BASE + path, 10)
                argv = run.call_args.args[0]
                self.assertEqual(argv[:7], ['gh', 'api', '--hostname', 'github.com',
                                           '--method', 'GET', 'repos/' + p.REPO + '/' + path])
            for url in (p.PUBLIC + p.ARCHIVE, p.PUBLIC + p.ARCHIVE + '.sha256',
                        p.BASE + 'actions/jobs/132634405095/logs',
                        p.BASE + 'actions/artifacts/1/zip'):
                p.download(url, 10)
                self.assertEqual(run.call_args.args[0][0], sys.executable)
                self.assertIn('--public-download', run.call_args.args[0])

    def test_actual_attempt_metadata_reused_not_alias_logs(self):
        root = Path(__file__).parent / 'fixtures/pinned-v0.27.0'
        for attempt in (1, 2):
            self.data[f'actions/runs/{p.RUN}/attempts/{attempt}/jobs?per_page=100'] = \
                json.loads((root / f'attempt-{attempt}-jobs.json').read_text())
        self.validate()
        self.assertNotIn(p.BASE + 'actions/jobs/132634405095/logs', self.calls)
        self.assertNotIn(p.BASE + 'actions/jobs/132634438965/logs', self.calls)
        key = f'actions/runs/{p.RUN}/attempts/2/jobs?per_page=100'
        original = copy.deepcopy(self.data[key])
        for field in ('started_at', 'completed_at', 'steps'):
            self.data[key]['jobs'][0][field] = None
            with self.subTest(field=field), self.assertRaises(p.Failure):
                self.validate()
            self.data[key] = copy.deepcopy(original)

    def test_matching_aliases_cannot_change_pinned_interval(self):
        for attempt in (1, 2):
            key = f'actions/runs/{p.RUN}/attempts/{attempt}/jobs?per_page=100'
            next(j for j in self.data[key]['jobs'] if j['name'] == p.LINUX)['started_at'] = \
                '2026-11-14T23:48:24Z'
        with self.assertRaises(p.Failure):
            self.validate()

    def test_redirect_drops_authorization_and_cookies(self):
        request = p.urllib.request.Request(p.BASE + 'actions/jobs/132631632452/logs',
                                          headers={'Authorization': 'sentinel',
                                                   'Cookie': 'sentinel'})
        redirected = p.Redirects('api.github.com').redirect_request(
            request, None, 302, '', {}, 'https://productionresultssa0.blob.core.windows.net/log')
        self.assertEqual(dict(redirected.header_items()),
                         {'User-agent': 'cercano-pinned-provenance'})

    def test_logs_are_exact_records_not_substrings(self):
        url = p.BASE + 'actions/jobs/' + str(p.JOBS[p.MAC]) + '/logs'
        original = self.routes[url]
        for bad in (b'<html>sign in</html>', original.replace(p.SOURCE.encode(), b'bad'),
                    original.replace(str(p.ARTIFACT).encode(), b'123'),
                    original.replace(p.OUTER_SHA.encode(), b'0' * 64),
                    original.replace(b'Release source:', b'echo Release source:'),
                    original + original):
            self.routes[url] = bad
            with self.assertRaises(p.Failure):
                self.validate()

    def test_digest_workflow_public_and_expired_fail_closed(self):
        for url in list(self.routes):
            if '/logs' in url:
                continue
            original = self.routes[url]
            self.routes[url] = original[:-1] + bytes([original[-1] ^ 1])
            with self.subTest(url=url), self.assertRaises(p.Failure):
                self.validate()
            self.routes[url] = original
        self.data['actions/artifacts/' + str(p.ARTIFACT)]['expired'] = True
        with self.assertRaises(p.Failure):
            self.validate()

    def test_strict_zip_checksum_and_inventory(self):
        for checksum in (b'', self.checksum + b'\n', self.checksum.replace(b'  ', b' '),
                         self.checksum.replace(p.ARCHIVE.encode(), b'../archive'), b'0' * 64 + self.checksum[64:]):
            self.entries[p.ARCHIVE + '.sha256'] = checksum
            outer = self.zip_bytes()
            with patch.object(p, 'OUTER_SHA', p.sha(outer)), patch.object(p, 'OUTER_SIZE', len(outer)):
                with self.assertRaises(p.Failure):
                    p.validate_bytes(outer, self.inner, self.checksum)
        self.entries[p.ARCHIVE + '.sha256'] = self.checksum
        self.entries['../escape'] = b'bad'
        outer = self.zip_bytes()
        with patch.object(p, 'OUTER_SHA', p.sha(outer)), patch.object(p, 'OUTER_SIZE', len(outer)):
            with self.assertRaises(p.Failure):
                p.validate_bytes(outer, self.inner, self.checksum)

    def test_missing_private_or_expired_download_is_not_bypassed(self):
        for missing in list(self.routes):
            original = self.routes.pop(missing)
            with self.assertRaises(p.Failure):
                self.validate()
            self.routes[missing] = original

    def test_redirect_policy(self):
        for url in ('http://release-assets.githubusercontent.com/a', 'https://evil.invalid/a',
                    'https://github.com/login', 'https://user:pass@release-assets.githubusercontent.com/a',
                    'https://release-assets.githubusercontent.com:444/a',
                    'https://release-assets.githubusercontent.com/a#fragment'):
            with self.subTest(url=url), self.assertRaises(p.Failure):
                p.Redirects('github.com').redirect_request(None, None, 302, '', {}, url)
        redirect = p.Redirects('github.com')
        for unused in range(3):
            request = redirect.redirect_request(None, None, 302, '', {},
                                                'https://release-assets.githubusercontent.com/a?sig=redacted')
            self.assertNotIn('Authorization', request.headers)
        with self.assertRaises(p.Failure):
            redirect.redirect_request(None, None, 302, '', {}, 'https://release-assets.githubusercontent.com/a')


class ActualLogTests(unittest.TestCase):
    def test_actual_log_syntax_and_changed_unsupported_records(self):
        root = Path(__file__).parent / 'fixtures/pinned-v0.27.0'
        for name in ('linux', 'windows', 'mac'):
            body = (root / (name + '.log')).read_bytes()
            p.validate_logs(body, upload=name == 'mac')
            for bad in (body + b'unsupported continuation\n',
                        body.replace(b'dist/cercano-', b'dist/changed-'),
                        body.replace(p.SOURCE.encode(), b'0' * 40),
                        body.replace(b'Release source:', b'echo Release source:'),
                        body + body):
                with self.subTest(name=name), self.assertRaises(p.Failure):
                    p.validate_logs(bad, upload=name == 'mac')


if __name__ == '__main__':
    unittest.main()
