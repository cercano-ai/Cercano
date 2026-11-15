"""Read-only, exact v0.27.0 two-stage provenance; evidence, never permission.

Intentionally separate from check_release_run.check: that general gate must NOT
accept a workflow/source mismatch or skipped publication steps.
"""
import hashlib
import io
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import time
import urllib.parse
import urllib.request
import zipfile

import render_formula

REPO = 'cercano-ai/Cercano'
SOURCE = 'c91699b079164138bff083206594f49c56dd5509'
WORKFLOW = '4b3cd19b4e505239c7a875952a7bb40fd46754e0'
WORKFLOW_DIGEST = 'c69cdfb23708b02414fc2067f9e6c5cc83560e61cd594ffe41ae1dd5b88a5be8'
RUN = 43655050601
ATTEMPT = 2
RELEASE = 445713847
ARTIFACT = 14139226701
ASSET = 749310766
OUTER_SHA = '89a85b1902925be5c4fadb67d5088bacc29fb29eb8d248663536cb64ecdc7bf6'
INNER_SHA = '18c68ccc69662da6d18b8017a3eba7eaebc67e057065b90bb66deba2838d2aa0'
OUTER_SIZE = 45630515
INNER_SIZE = 45728133
VERSION = '0.27.0'
NAME = 'cercano-0.27.0-darwin-arm64'
ARCHIVE = NAME + '.tar.gz'
BASE = 'https://api.github.com/repos/' + REPO + '/'
PUBLIC = 'https://github.com/' + REPO + '/releases/download/v0.27.0/'
RAW = 'https://raw.githubusercontent.com/' + REPO + '/' + WORKFLOW + '/.github/workflows/release-macos.yml'
MAC = 'Build, sign, notarize, verify and release'
LINUX = 'Build unsigned experimental Linux x86_64 archive'
WINDOWS = 'Build unsigned experimental Windows x64 archive'
JOBS = {MAC: 132634405380, LINUX: 132634405095, WINDOWS: 132634438965}
REUSED = {
    LINUX: (132631632452, '2026-11-14T23:48:25Z', '2026-11-14T23:50:06Z'),
    WINDOWS: (132631632597, '2026-11-14T23:48:26Z', '2026-11-14T23:52:02Z'),
}
COMMON = ('Validate source version', 'Run actions/checkout@v4', 'Validate inputs',
          'Run release test gate', 'Upload artifacts for review')
EXTRA = {
    MAC: ('Import signing identity', 'Build and sign release archive', 'Notarize',
          'Verify finished archive', 'Render Homebrew formula', 'Destroy signing keychain'),
    LINUX: ('Build unsigned Linux archive', 'Smoke test binaries with isolated user directories',
            'Run Linux-native helper tests'),
    WINDOWS: ('Build unsigned Windows archive', 'Smoke test binaries with isolated user directories',
              'Run Windows-native helper tests'),
}
SKIPPED = ('Refuse to replace an existing published artifact', 'Publish release',
           'Test tap updater', 'Publish verified formula to tap')


class Failure(Exception):
    pass


def require(value):
    if not value:
        raise Failure('Pinned release evidence unavailable or invalid')


def sha(body):
    return hashlib.sha256(body).hexdigest()


def safe_url(url, hosts):
    p = urllib.parse.urlsplit(url)
    require(p.scheme == 'https' and p.hostname in hosts and p.port in (None, 443)
            and p.username is None and p.password is None and not p.fragment
            and not any(ord(c) < 33 or ord(c) > 126 for c in url))


class Redirects(urllib.request.HTTPRedirectHandler):
    def __init__(self, origin):
        self.origin = origin
        self.count = 0

    def redirect_request(self, request, fp, code, msg, headers, newurl):
        self.count += 1
        require(self.count <= 3)
        # No wildcard hosts, HTTP downgrade, API-to-login redirect or credentials.
        allowed = {
            'api.github.com': {'productionresultssa0.blob.core.windows.net'},
            'github.com': {'release-assets.githubusercontent.com'},
            'raw.githubusercontent.com': set(),
        }.get(self.origin, set())
        safe_url(newurl, allowed)
        return urllib.request.Request(newurl, headers={'User-Agent': 'cercano-pinned-provenance'})


def _download(url, limit):
    """Credential/cookie/proxy-free GET; finite bytes, hops and wall time.

    Socket timeout is an inactivity bound only. The parent process enforces
    the overall wall deadline, including DNS, TLS, redirects and blocking reads.
    """
    try:
        hosts = {'api.github.com', 'github.com', 'raw.githubusercontent.com'}
        safe_url(url, hosts)
        origin = urllib.parse.urlsplit(url).hostname
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), Redirects(origin))
        request = urllib.request.Request(url, headers={'User-Agent': 'cercano-pinned-provenance'})
        deadline = time.monotonic() + 120
        with opener.open(request, timeout=20) as response:
            require(time.monotonic() < deadline)
            require(response.status == 200)
            require(response.headers.get('Content-Encoding', 'identity') == 'identity')
            length = response.headers.get('Content-Length')
            require(length is None or (length.isdecimal() and int(length) <= limit))
            chunks, total = [], 0
            while True:
                require(time.monotonic() < deadline)
                chunk = response.read(min(65536, limit + 1 - total))
                require(time.monotonic() < deadline)
                if not chunk:
                    break
                total += len(chunk)
                require(total <= limit)
                chunks.append(chunk)
            return b''.join(chunks)
    except Exception:
        raise Failure('Public evidence download failed; no credentials attempted') from None


# Only these three executed jobs and this artifact may use gh authentication.
AUTH_PATHS = {f'actions/jobs/{i}/logs' for i in
              (132631632452, 132631632597, 132634405380)} | {
                  f'actions/artifacts/{ARTIFACT}/zip'}


def bounded_process(argv, limit):
    # Do not expose gh diagnostics, redirect signatures, environment or credentials.
    # gh's host-scoped authentication transport does not authorize storage hosts;
    # Go's HTTP redirect handling also strips Authorization on unrelated domains.
    env = os.environ.copy()
    for key in ('GH_DEBUG', 'DEBUG', 'GH_HOST', 'GH_REPO'):
        env.pop(key, None)
    try:
        with tempfile.TemporaryFile() as output:
            deadline = time.monotonic() + 120
            with subprocess.Popen(argv, stdout=output, stderr=subprocess.DEVNULL,
                                  env=env) as child:
                try:
                    while child.poll() is None:
                        require(time.monotonic() < deadline)
                        require(output.tell() <= limit)
                        time.sleep(0.02)
                    require(time.monotonic() < deadline and child.returncode == 0)
                    require(output.tell() <= limit)
                finally:
                    if child.poll() is None:
                        child.kill()
                    child.wait()
            output.seek(0)
            body = output.read(limit + 1)
            require(time.monotonic() < deadline and len(body) <= limit)
            return body
    except Exception:
        raise Failure('Bounded evidence transport failed') from None


def download(url, limit):
    """Public bytes stay credential-free; only fixed Actions GETs use existing gh."""
    if url.startswith(BASE) and url[len(BASE):] in AUTH_PATHS:
        return bounded_process(['gh', 'api', '--hostname', 'github.com', '--method',
                                'GET', 'repos/' + REPO + '/' + url[len(BASE):]], limit)
    safe_url(url, {'api.github.com', 'github.com', 'raw.githubusercontent.com'})
    return bounded_process([sys.executable, str(Path(__file__).resolve()),
                            '--public-download', url, str(limit)], limit)


def json_body(body):
    def pairs(items):
        result = {}
        for key, value in items:
            require(key not in result)
            result[key] = value
        return result
    def invalid(value):
        require(False)
    return json.loads(body.decode('utf-8'), object_pairs_hook=pairs, parse_constant=invalid)


def exact_steps(job, names, conclusion):
    for name in names:
        matches = [s for s in job['steps'] if s.get('name') == name]
        require(len(matches) == 1 and matches[0].get('status') == 'completed'
                and matches[0].get('conclusion') == conclusion)


def log_lines(body):
    # Raw GitHub job logs only: require timestamps and compare complete output
    # records, not substrings in echoed shell commands or arbitrary HTML.
    records = []
    continuations = []
    paths = {
        'dist/' + ARCHIVE: ['dist/' + ARCHIVE + '.sha256', 'dist/cercano.rb',
                            'dist/cercano-agent.rb', 'dist/cercano-cli.rb', ''],
        'dist/cercano-0.27.0-linux-x64.tar.gz':
            ['dist/cercano-0.27.0-linux-x64.tar.gz.sha256', ''],
        'dist/cercano-0.27.0-windows-x64.zip':
            ['dist/cercano-0.27.0-windows-x64.zip.sha256', ''],
    }
    for line in body.decode('utf-8-sig').splitlines():
        if continuations:
            require(line == continuations.pop(0))
            continue
        match = re.fullmatch(r'\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d+Z (.*)', line)
        require(match is not None)
        record = match[1]
        if record.startswith('  path: '):
            require(record[8:] in paths)
            continuations = list(paths[record[8:]])
        records.append(record)
    require(records and not continuations)
    return records


def validate_logs(body, upload=False):
    records = log_lines(body)
    # These are output records of the pinned checkout/validation implementation.
    for expected in (SOURCE,
                     'Release source: refs/tags/v0.27.0 at ' + SOURCE,
                     'Workflow: cercano-ai/Cercano/.github/workflows/release-macos.yml@refs/heads/fix/macos-release-runner at ' + WORKFLOW):
        require(records.count(expected) == 1)
    require(records.count('  ref: refs/tags/v0.27.0') == 1)
    if upload:
        for expected in (
            'SHA256 digest of uploaded artifact zip is ' + OUTER_SHA,
            'Artifact ' + NAME + '.zip successfully finalized. Artifact ID ' + str(ARTIFACT),
            'Artifact ' + NAME + ' has been successfully uploaded! Final size is ' + str(OUTER_SIZE) + ' bytes. Artifact ID is ' + str(ARTIFACT),
            'Artifact download URL: https://github.com/' + REPO + '/actions/runs/' + str(RUN) + '/artifacts/' + str(ARTIFACT)):
            require(records.count(expected) == 1)


def validate_bytes(outer, public, public_checksum):
    # Digest verification precedes ZIP parsing; never extract ZIP paths.
    require(len(outer) == OUTER_SIZE and sha(outer) == OUTER_SHA)
    expected = {ARCHIVE: INNER_SIZE, ARCHIVE + '.sha256': 256,
                'cercano.rb': 32768, 'cercano-agent.rb': 32768, 'cercano-cli.rb': 32768}
    with zipfile.ZipFile(io.BytesIO(outer)) as archive:
        entries = archive.infolist()
        require(len(entries) == len(expected) and {e.filename for e in entries} == set(expected))
        for entry in entries:
            require(not entry.is_dir() and not entry.flag_bits & 1
                    and entry.file_size <= expected[entry.filename]
                    and (entry.external_attr >> 16) & 0o170000 in (0, 0o100000))
        inner = archive.read(ARCHIVE)
        require(len(inner) == INNER_SIZE and sha(inner) == INNER_SHA)
        checksum = (INNER_SHA + '  ' + ARCHIVE + '\n').encode()
        require(archive.read(ARCHIVE + '.sha256') == checksum)
    require(public_checksum == checksum)
    require(len(public) == INNER_SIZE and sha(public) == INNER_SHA and public == inner)
    with tempfile.TemporaryDirectory(prefix='cercano-provenance-') as directory:
        path = Path(directory) / ARCHIVE
        path.write_bytes(inner)
        render_formula.validate_archive(path, VERSION)
        # The existing helper permits optional metadata; this pinned validation
        # requires the complete canonical inventory, not just the executables.
        import tarfile
        with tarfile.open(path, 'r:gz') as archive:
            files = {m.name for m in archive if m.isfile()}
        require(files == {NAME + '/' + n for n in
                          render_formula.EXECUTABLES + render_formula.METADATA})


def _validate(version, digest, fetch):
    require(version == VERSION and digest == INNER_SHA)
    def get(path):
        return json_body(fetch(BASE + path, 2 * 1024 * 1024))
    release = get('releases/tags/v0.27.0')
    require(release['id'] == RELEASE and release['tag_name'] == 'v0.27.0'
            and release['draft'] is False and release['prerelease'] is False)
    require(get('commits/v0.27.0')['sha'] == SOURCE)
    run = get('actions/runs/' + str(RUN) + '/attempts/2')
    require(run['id'] == RUN and run['run_attempt'] == ATTEMPT
            and run['head_sha'] == WORKFLOW and run['event'] == 'workflow_dispatch'
            and run['path'] == '.github/workflows/release-macos.yml'
            and run['status'] == 'completed' and run['conclusion'] == 'success'
            and run['repository']['full_name'] == REPO
            and run['head_repository']['full_name'] == REPO)
    require(sha(fetch(RAW, 65536)) == WORKFLOW_DIGEST)
    jobs = get('actions/runs/' + str(RUN) + '/attempts/2/jobs?per_page=100')
    require(jobs['total_count'] == 3 and len(jobs['jobs']) == 3)
    require({j['name'] for j in jobs['jobs']} == set(JOBS))
    for job in jobs['jobs']:
        name = job['name']
        require(job['id'] == JOBS[name] and job['run_id'] == RUN
                and job['run_attempt'] == ATTEMPT and job['head_sha'] == WORKFLOW
                and job['status'] == 'completed' and job['conclusion'] == 'success')
        exact_steps(job, COMMON + EXTRA[name], 'success')
        if name == MAC:
            exact_steps(job, SKIPPED, 'skipped')
            validate_logs(fetch(BASE + 'actions/jobs/' + str(job['id']) + '/logs',
                                16 * 1024 * 1024), upload=True)
    # The first-stage successful non-macOS prerequisites were retained. Validate
    # those original jobs too, not only a latest-jobs view or the macOS rerun.
    previous = get('actions/runs/' + str(RUN) + '/attempts/1/jobs?per_page=100')
    require(previous['total_count'] == 3 and len(previous['jobs']) == 3)
    for name, (job_id, started, completed) in REUSED.items():
        matches = [j for j in previous['jobs'] if j['name'] == name]
        require(len(matches) == 1)
        job = matches[0]
        require(job['id'] == job_id and job['run_id'] == RUN and job['run_attempt'] == 1
                and job['head_sha'] == WORKFLOW and job['status'] == 'completed'
                and job['conclusion'] == 'success')
        exact_steps(job, COMMON + EXTRA[name], 'success')
        alias = next(j for j in jobs['jobs'] if j['name'] == name)
        # GitHub reuses the executed job under an attempt-specific alias.
        for field in ('started_at', 'completed_at', 'steps', 'head_sha',
                      'run_id', 'name', 'status', 'conclusion'):
            require(alias[field] == job[field])
        require(job['started_at'] == started and job['completed_at'] == completed)
        validate_logs(fetch(BASE + 'actions/jobs/' + str(job_id) + '/logs', 16 * 1024 * 1024))
    artifact = get('actions/artifacts/' + str(ARTIFACT))
    require(artifact['id'] == ARTIFACT and artifact['name'] == NAME
            and artifact['expired'] is False and artifact['size_in_bytes'] == OUTER_SIZE
            and artifact['digest'] == 'sha256:' + OUTER_SHA
            and artifact['workflow_run']['id'] == RUN
            and artifact['workflow_run']['head_sha'] == WORKFLOW)
    assets = [a for a in release['assets'] if a['name'] == ARCHIVE]
    require(len(assets) == 1)
    asset = assets[0]
    require(asset['id'] == ASSET and asset['state'] == 'uploaded'
            and asset['size'] == INNER_SIZE and asset['digest'] == 'sha256:' + INNER_SHA
            and asset['browser_download_url'] == PUBLIC + ARCHIVE)
    outer = fetch(BASE + 'actions/artifacts/' + str(ARTIFACT) + '/zip', OUTER_SIZE)
    # No inner parsing or public download before the outer digest check.
    require(len(outer) == OUTER_SIZE and sha(outer) == OUTER_SHA)
    validate_bytes(outer, fetch(PUBLIC + ARCHIVE, INNER_SIZE),
                   fetch(PUBLIC + ARCHIVE + '.sha256', 256))
    return {'schema': 1, 'repository': REPO, 'version': VERSION, 'source_sha': SOURCE,
            'workflow_sha': WORKFLOW, 'run_id': RUN, 'run_attempt': ATTEMPT,
            'release_id': RELEASE, 'artifact_id': ARTIFACT, 'asset_id': ASSET,
            'outer_sha256': OUTER_SHA, 'archive_sha256': INNER_SHA,
            'ci_evidence_validated': True, 'apple_ticket_independently_repeated': False,
            'clean_mac_qualified': False, 'publication_authorized': False}


def validate(version=VERSION, digest=INNER_SHA, *, fetch=download):
    """No arbitrary run, ref, pins, credentials or acceptance override inputs."""
    try:
        return _validate(version, digest, fetch)
    except Exception:
        raise Failure('Pinned release evidence unavailable or invalid; publication refused') from None


if __name__ == '__main__':
    try:
        if len(sys.argv) == 4 and sys.argv[1] == '--public-download':
            sys.stdout.buffer.write(_download(sys.argv[2], int(sys.argv[3])))
        else:
            require(len(sys.argv) == 1)
            print(json.dumps(validate(), sort_keys=True))
    except Failure as error:
        raise SystemExit(str(error))
