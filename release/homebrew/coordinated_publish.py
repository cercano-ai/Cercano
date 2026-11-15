"""Isolated, disabled publisher. Plans are comparisons, never authorization.

The bounded CLI cannot override acceptance. A future acceptance change must be reviewed.
The injectable API has signature (query, variables, token); tests need no network.
"""
import argparse
import base64
import hashlib
import json
import os
import sys
from pathlib import Path
import re
import urllib.request

import render_formula as renderer

PUBLICATION_ACCEPTED = False
PATHS = ('Formula/cercano.rb', 'Formula/cercano-agent.rb', 'Formula/cercano-cli.rb')
REPOSITORY = 'cercano-ai/homebrew-cercano'
LIMIT = 256 * 1024
OID = re.compile(r'[0-9a-f]{40}')
DIGEST = re.compile(r'[0-9a-f]{64}')
# Immutable reviewed transition: base, full before tuple, destination, archive.
# Neither a preparation plan nor CLI input can extend this authorization.
REVIEWED_TRANSITION = (
    'caffd060597d6ca9706c9fa4db0c7c042722d7ef',
    ('50f68b11106b7ca865bd931a0e945fc454c4275283d9e93ade230c2f548be5be', None, None),
    '0.27.0',
    '18c68ccc69662da6d18b8017a3eba7eaebc67e057065b90bb66deba2838d2aa0',
)
HISTORICAL_SIZE = 2239


class Failure(Exception):
    """Only fixed local diagnostics may cross this boundary."""


def require(condition, message):
    if not condition:
        raise Failure(message)


def strict_json(body):
    require(len(body) <= LIMIT, 'JSON exceeds limit')

    def pairs(items):
        result = {}
        for key, value in items:
            require(key not in result, 'Duplicate JSON key')
            result[key] = value
        return result

    def invalid_constant(value):
        raise Failure('Invalid JSON constant')

    return json.loads(body.decode('utf-8'), object_pairs_hook=pairs,
                      parse_constant=invalid_constant)


def sha(content):
    return hashlib.sha256(content).hexdigest()


def expected(version, digest):
    renderer.parse_version(version)
    renderer.parse_digest(digest)
    return {path: renderer.render(
        Path(__file__).with_name(path.split('/')[1] + '.in').read_text(),
        version, 'cercano-' + version + '-darwin-arm64.tar.gz', digest).encode('utf-8')
        for path in PATHS}


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        raise Failure('API redirect refused')


def api(query, variables, token):
    """Fixed host; credentials occur only in the Authorization header."""
    request = urllib.request.Request('https://api.github.com/graphql',
        data=json.dumps({'query': query, 'variables': variables}).encode(),
        headers={'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json',
                 'User-Agent': 'cercano-coordinated-publisher'}, method='POST')
    try:
        with urllib.request.build_opener(NoRedirect()).open(request, timeout=30) as response:
            body = response.read(LIMIT + 1)
        require(len(body) <= LIMIT, 'API response exceeds limit')
        return strict_json(body)
    except Exception:
        raise Failure('API request failed') from None


def call(client, query, variables, token):
    try:
        result = client(query, variables, token)
        require(type(result) is dict and len(json.dumps(result).encode()) <= LIMIT,
                'Invalid API response')
        require(not result.get('errors') and type(result.get('data')) is dict,
                'Invalid API response')
        return result['data']
    except Exception:
        raise Failure('API response unavailable or invalid') from None


HEAD = '''query { repository(owner:"cercano-ai", name:"homebrew-cercano") {
 ref(qualifiedName:"refs/heads/main") { target { ... on Commit {
 oid message parents(first:2) { nodes { oid } } } } } } }'''
FILES = '''query($a:String!, $b:String!, $c:String!, $d:String!) {
 repository(owner:"cercano-ai", name:"homebrew-cercano") {
 a:object(expression:$a) { ... on Blob { text byteSize } }
 b:object(expression:$b) { ... on Blob { text byteSize } }
 c:object(expression:$c) { ... on Blob { text byteSize } }
 d:object(expression:$d) { ... on Tree { entries { name mode type } } } } }'''
MUTATION = '''mutation($input:CreateCommitOnBranchInput!) {
 createCommitOnBranch(input:$input) { commit { oid } } }'''


def snapshot(client, token):
    try:
        target = call(client, HEAD, {}, token)['repository']['ref']['target']
        head = target['oid']
        require(type(head) is str and OID.fullmatch(head), 'Invalid branch head')
        message = target['message']
        parents = target['parents']['nodes']
        require(type(message) is str and len(message) <= 4096 and type(parents) is list
                and len(parents) <= 2, 'Invalid commit evidence')
        require(all(type(p) is dict and type(p.get('oid')) is str and
                    OID.fullmatch(p['oid']) for p in parents), 'Invalid parent evidence')
        variables = dict(zip('abc', (head + ':' + p for p in PATHS)))
        variables['d'] = head + ':Formula'
        objects = call(client, FILES, variables, token)['repository']
        directory = objects['d']
        entries = [] if directory is None else directory['entries']
        require(type(entries) is list and len(entries) <= 1000, 'Invalid Formula tree')
        modes = {}
        for entry in entries:
            require(type(entry) is dict and type(entry.get('name')) is str,
                    'Invalid tree entry')
            path = 'Formula/' + entry['name']
            if path in PATHS:
                require(path not in modes and entry.get('mode') == 33188 and
                        entry.get('type') == 'blob', 'Nonordinary formula refused')
                modes[path] = True
        contents = {}
        for alias, path in zip('abc', PATHS):
            blob = objects[alias]
            if blob is None:
                require(path not in modes, 'Missing formula object')
                contents[path] = None
            else:
                require(path in modes, 'Missing ordinary formula entry')
                require(type(blob) is dict and type(blob.get('text')) is str,
                        'Invalid formula blob')
                content = blob['text'].encode('utf-8')
                require(type(blob.get('byteSize')) is int and
                        blob['byteSize'] == len(content) and len(content) <= 32768,
                        'Invalid formula size')
                contents[path] = content
        return head, contents, [p['oid'] for p in parents], message
    except Exception:
        raise Failure('Unable to read exact branch snapshot') from None


def validate_plan(plan):
    try:
        require(type(plan) is dict and type(plan['schema']) is int and plan['schema'] == 1, 'Invalid plan')
        base = plan['tap_base_sha']
        require(type(base) is str and OID.fullmatch(base), 'Reviewed base required')
        desired = expected(plan['version'], plan['archive_sha256'])
        entries = plan['files']
        require(type(entries) is list and len(entries) == 3, 'Exact three-file plan required')
        before = {}
        for entry in entries:
            path = entry['path']
            require(path in PATHS and path not in before, 'Invalid plan paths')
            value = entry['before_sha256']
            require(value is None or (type(value) is str and DIGEST.fullmatch(value)),
                    'Exact before hash required')
            require(entry['after_sha256'] == sha(desired[path]) and
                    entry['content'].encode('utf-8') == desired[path], 'Noncanonical desired set')
            before[path] = value
        marker = 'chore: coordinated Cercano ' + sha(json.dumps(
            [base, sorted(before.items()), [(p, sha(desired[p])) for p in PATHS]],
            separators=(',', ':')).encode())
        return base, before, desired, marker
    except Exception:
        raise Failure('Invalid reviewed preparation plan') from None


def validate_before(contents, before, desired, version, base, digest):
    require({p: sha(v) if v is not None else None for p, v in contents.items()} == before,
            'Before files differ from reviewed plan; fresh review required')
    transition = (base, tuple(before[p] for p in PATHS), version, digest)
    if transition == REVIEWED_TRANSITION:
        require(len(contents[PATHS[0]]) == HISTORICAL_SIZE,
                'Historical byte size differs')
        return
    if all(v is None for v in contents.values()):
        return
    require(all(v is not None for v in contents.values()), 'Partial before set refused')
    # No substring-based historical recognition or implicit migration permission.
    try:
        text = contents[PATHS[0]].decode('utf-8')
        old_version = re.findall(r'^\s*version "([^"\n]+)"$', text, re.M)
        old_digest = re.findall(r'^\s*sha256 "([^"\n]+)"$', text, re.M)
        require(len(old_version) == len(old_digest) == 1, 'Invalid before set')
        require(contents == expected(old_version[0], old_digest[0]), 'Noncanonical before set')
        old, new = (tuple(map(int, v.split('.'))) for v in (old_version[0], version))
        require(old <= new, 'Downgrade refused')
        require(old != new or contents == desired, 'Same-version conflict refused')
    except Exception:
        raise Failure('Before set is not an eligible canonical set') from None


def publish(plan, token, api_fn=api):
    # Early gate prevents even reads. The second gate is at the mutation boundary.
    require(PUBLICATION_ACCEPTED, 'Publication disabled: acceptance pending')
    try:
        return _publish(plan, token, api_fn)
    except Failure:
        raise
    except Exception:
        raise Failure('Publication failed; fresh review required') from None


def _publish(plan, token, client):
    base, before, desired, marker = validate_plan(plan)
    require(type(token) is str and bool(token), 'Credential required')
    head, contents, parents, message = snapshot(client, token)
    if head != base:
        if contents == desired and parents == [base] and message == marker:
            return 'reconciled'
        raise Failure('Branch base changed; fresh review required')
    validate_before(contents, before, desired, plan['version'], base, plan['archive_sha256'])
    if contents == desired:
        return 'unchanged'
    payload = {'branch': {'repositoryNameWithOwner': REPOSITORY, 'branchName': 'main'},
               'expectedHeadOid': base, 'message': {'headline': marker},
               'fileChanges': {'additions': [
                   {'path': p, 'contents': base64.b64encode(desired[p]).decode('ascii')}
                   for p in PATHS]}}
    require(PUBLICATION_ACCEPTED, 'Publication disabled: acceptance pending')
    commit = None
    try:
        commit = call(client, MUTATION, {'input': payload}, token)['createCommitOnBranch']['commit']['oid']
        require(type(commit) is str and OID.fullmatch(commit), 'Invalid mutation evidence')
    except Exception:
        commit = None
    # Never retry a mutation, including on explicit contention or read failures.
    try:
        head, contents, parents, message = snapshot(client, token)
    except Failure:
        raise Failure('Publication uncertain; no retry attempted') from None
    if contents == desired and parents == [base] and message == marker:
        if commit is None or head == commit:
            return 'updated' if commit else 'reconciled'
    raise Failure('Publication uncertain or base changed; fresh review required')


class SafeParser(argparse.ArgumentParser):
    def error(self, message):
        self.exit(2, 'Publication refused: invalid command line.\n')


def main(argv=None):
    parser = SafeParser(description=__doc__, allow_abbrev=False)
    parser.add_argument('--plan', type=Path, required=True)
    args = parser.parse_args(argv)
    # Before opening the plan, reading credentials, or performing any API reads.
    if not PUBLICATION_ACCEPTED:
        print('Publication disabled: acceptance pending.', file=sys.stderr)
        return 1
    try:
        with args.plan.open('rb') as source:
            plan = strict_json(source.read(LIMIT + 1))
        result = publish(plan, os.environ.get('HOMEBREW_TAP_TOKEN', ''))
        require(result in ('updated', 'unchanged', 'reconciled'), 'Invalid result')
    except Exception:
        print('Publication refused: invalid inputs or unconfirmed publication; no retry attempted.',
              file=sys.stderr)
        return 1
    print('Publication confirmed.')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
