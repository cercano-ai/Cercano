#!/usr/bin/env python3
"""Coordinated Git-tree publisher, gated on native/runtime acceptance."""
import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import sys
import urllib.error
import urllib.parse
import urllib.request

import formula_set as formulas
import render_formula as renderer
from promote_to_tap import read_formula_fields, PromotionError

TAP_PATH = '/repos/cercano-ai/homebrew-cercano/contents/Formula/cercano.rb'
RELEASE_PATH = '/repos/cercano-ai/Cercano/releases/tags/'
MAX_ARCHIVE = 512 * 1024 * 1024


class Failure(Exception):
    def __init__(self, message, status=None):
        super().__init__(message)
        self.status = status


def require(ok, message):
    if not ok:
        raise Failure(message)


def public_url(url):
    p = urllib.parse.urlsplit(url)
    host = p.hostname or ''
    require(p.scheme == 'https' and not p.username and not p.password
            and p.port in (None, 443)
            and any(host == d or host.endswith('.' + d)
                    for d in ('github.com', 'githubusercontent.com')),
            'Refusing non-HTTPS or non-GitHub artifact redirect')


class PublicRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        public_url(newurl)
        return super().redirect_request(req, fp, code, msg, headers, newurl)


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        raise Failure('Refusing authenticated API redirect')


def api(method, path, token, payload=None):
    # No caller-supplied host; authentication cannot leave api.github.com.
    headers = {'Accept': 'application/vnd.github+json',
               'X-GitHub-Api-Version': '2022-11-28', 'User-Agent': 'cercano-tap-release'}
    if token:
        headers['Authorization'] = 'Bearer ' + token
    data = json.dumps(payload).encode() if payload is not None else None
    if data is not None:
        headers['Content-Type'] = 'application/json'
    req = urllib.request.Request('https://api.github.com' + path, data=data,
                                 headers=headers, method=method)
    try:
        with urllib.request.build_opener(NoRedirect()).open(req, timeout=60) as r:
            body = r.read(8 * 1024 * 1024 + 1)
            require(len(body) <= 8 * 1024 * 1024, 'Oversized API response')
            return json.loads(body)
    except urllib.error.HTTPError as e:
        # Never print response bodies, URLs, headers or credentials.
        raise Failure(f'GitHub {method} failed: HTTP {e.code}', e.code) from None
    except urllib.error.URLError:
        raise Failure('GitHub API connection failed') from None


def download(url, checksum=False):
    public_url(url)
    limit = 4096 if checksum else MAX_ARCHIVE
    size, digest, chunks = 0, hashlib.sha256(), []
    try:
        with urllib.request.build_opener(PublicRedirect()).open(url, timeout=120) as r:
            public_url(r.geturl())
            require(r.status == 200, 'Public release download did not return HTTP 200')
            while chunk := r.read(1024 * 1024):
                size += len(chunk)
                require(size <= limit, 'Public release download exceeds size limit')
                digest.update(chunk)
                if checksum:
                    chunks.append(chunk)
    except (urllib.error.URLError, OSError):
        raise Failure('Public release download failed; retry the tap job after checking assets') from None
    require(size > 0, 'Empty release download')
    return b''.join(chunks).decode('utf-8') if checksum else digest.hexdigest()


def published(version, name, digest):
    release = api('GET', RELEASE_PATH + 'v' + version, os.environ.get('GITHUB_TOKEN', ''))
    require(release.get('tag_name') == 'v' + version and release.get('draft') is False,
            'Release is not published for the requested version')
    names = {a['name'] for a in release.get('assets', [])}
    require({name, name + '.sha256'} <= names, 'Published macOS assets are incomplete')
    url = renderer.URL_TEMPLATE.format(version=version, name=name)
    require(download(url) == digest, 'Published archive checksum differs from trusted build')
    require(download(url + '.sha256', checksum=True).split() == [digest, name],
            'Published checksum sidecar differs from trusted build')


def trusted_formula_matches(actual, expected, version):
    if actual == expected:
        return True
    # v0.20.3's immutable build artifact predates the organization transfer.
    # Accept only the exact former-owner variant of our trusted template;
    # prepare() still returns the canonical template, never the supplied text.
    # No other release or formula alteration gets this compatibility exception.
    if version != '0.20.3':
        return False
    legacy = expected.replace('https://github.com/cercano-ai/Cercano',
                              'https://github.com/bryancostanich/Cercano')
    return actual == legacy


def prepare(version, digest, archive, formula):
    renderer.parse_version(version)
    renderer.parse_digest(digest)
    require(renderer.hash_archive(archive) == digest, 'CI archive checksum mismatch')
    renderer.validate_archive(archive, version)
    return formulas.reviewed(formula.parent, version, digest)


REPO = '/repos/cercano-ai/homebrew-cercano'
REF = REPO + '/git/refs/heads/main'


def snapshot(token):
    head = api('GET', REPO + '/git/ref/heads/main', token)['object']['sha']
    commit = api('GET', REPO + '/git/commits/' + head, token)
    tree = commit['tree']['sha']
    listing = api('GET', REPO + '/git/trees/' + tree + '?recursive=1', token)
    require(listing.get('truncated') is False, 'Truncated tap tree')
    existing = {}
    for entry in listing['tree']:
        if entry['path'] not in {'Formula/' + n for n in formulas.NAMES}:
            continue
        require(entry['type'] == 'blob' and entry['mode'] == '100644',
                'Formula is not an ordinary file')
        blob = api('GET', REPO + '/git/blobs/' + entry['sha'], token)
        require(blob['encoding'] == 'base64', 'Unexpected blob encoding')
        existing[entry['path'].split('/')[1]] = base64.b64decode(blob['content']).decode('utf-8')
    return head, tree, existing


def publish_set(desired, version, digest, token):
    """One tree/commit per attempt; only a non-force ref update publishes.

    On an ambiguous PATCH response, reread the complete set. Exact desired state
    is success (including a concurrent identical publisher); otherwise fail
    closed, except explicit contention, which gets bounded revalidation.
    """
    formulas.require_acceptance()
    for attempt in range(3):
        head, base, existing = snapshot(token)
        if formulas.decision(existing, desired, version, digest) == 'unchanged':
            return 'unchanged'
        tree = api('POST', REPO + '/git/trees', token, {
            'base_tree': base, 'tree': [
                {'path': 'Formula/' + name, 'mode': '100644', 'type': 'blob',
                 'content': desired[name]} for name in formulas.NAMES]})['sha']
        commit = api('POST', REPO + '/git/commits', token, {
            'message': 'chore: publish Cercano ' + version,
            'tree': tree, 'parents': [head]})['sha']
        try:
            response = api('PATCH', REF, token, {'sha': commit, 'force': False})
            require(response.get('object', {}).get('sha') == commit,
                    'Unexpected ref update response')
            return 'updated'
        except (Failure, OSError, ValueError) as error:
            _, _, current = snapshot(token)
            if current == desired:
                return 'reconciled'
            # Also refuse downgrade/mutation immediately after a race.
            formulas.decision(current, desired, version, digest)
            if not isinstance(error, Failure) or error.status not in (409, 422):
                raise
    raise Failure('Tap contention exhausted; no force update attempted')


def update(version, digest, archive, formula, token):
    try:
        formulas.require_acceptance()
    except renderer.ValidationError as error:
        raise Failure(str(error)) from None
    require(bool(token), 'Missing tap token')
    desired = prepare(version, digest, archive, formula)
    published(version, archive.name, digest)
    return publish_set(desired, version, digest, token)


def main():
    p = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    p.add_argument('--version', required=True)
    p.add_argument('--sha256', required=True)
    p.add_argument('--archive', type=Path, required=True)
    p.add_argument('--formula-dir', type=Path, required=True,
                   help='reviewed directory containing all three formulas')
    a = p.parse_args()
    try:
        result = update(a.version, a.sha256, a.archive, a.formula_dir / 'cercano.rb',
                        os.environ.get('HOMEBREW_TAP_TOKEN', ''))
    except (Failure, renderer.ValidationError, PromotionError) as e:
        print(f'Tap update refused: {e}', file=sys.stderr)
        return 1
    except (OSError, ValueError, KeyError):
        print('Tap update failed: unreadable input or malformed response; no force update attempted', file=sys.stderr)
        return 1
    print(f'Homebrew tap {result}: Cercano {a.version}')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
