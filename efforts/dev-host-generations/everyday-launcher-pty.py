#!/usr/bin/env python3
"""Actual ordinary entrypoint refusal acceptance (NOT compiled A/B acceptance).

Runs the repository shell script under a PTY. Publication failures use a clearly
labelled mock publisher in an isolated repository; no host/CLI substitute runs.
No native credential opener or installed launcher is used.
"""
import argparse
import errno
import hashlib
import json
import os
from pathlib import Path
import pty
import select
import sqlite3
import subprocess
import tempfile
import time


def snapshot(root):
    return {str(p.relative_to(root)): hashlib.sha256(p.read_bytes()).hexdigest()
            for p in sorted(root.rglob('*')) if p.is_file()}


def invoke(script, env):
    master, slave = pty.openpty()
    process = subprocess.Popen(['bash', str(script)], stdin=slave, stdout=slave,
                               stderr=slave, env=env, start_new_session=True)
    os.close(slave)
    output = bytearray()
    deadline = time.monotonic() + 20
    try:
        while True:
            if time.monotonic() >= deadline:
                raise AssertionError('ordinary launcher refusal timed out')
            if not select.select([master], [], [], .1)[0]:
                continue
            try:
                data = os.read(master, 65536)
            except OSError as error:
                if error.errno != errno.EIO:
                    raise
                break
            if not data:
                break
            output.extend(data)
        code = process.wait(timeout=5)
    finally:
        # Only this test-owned launcher child; never discover/kill existing hosts.
        if process.poll() is None:
            process.kill()
            process.wait()
        os.close(master)
    return code, output.decode(errors='replace')


def run(repo, evidence):
    script = repo / 'source/server/scripts/cercano-launcher.sh'
    results = []
    with tempfile.TemporaryDirectory(prefix='everyday-refusal-') as temporary:
        root = Path(temporary).resolve()
        state, mock = root / 'state', root / 'mock-repo'
        state.mkdir(mode=0o700)
        scripts = mock / 'source/server/scripts'
        scripts.mkdir(parents=True)
        # Actual Python orchestration is copied unchanged; only publisher mocked.
        (scripts / 'cercano-launcher.py').write_bytes(
            (repo / 'source/server/scripts/cercano-launcher.py').read_bytes())
        trace = root / 'publisher.trace'
        publisher = scripts / 'publish-dev-generation.py'
        publisher.write_text('import os,sys\nfrom pathlib import Path\n'
                             'with open(os.environ["TEST_TRACE"], "a") as f: f.write(repr(sys.argv)+"\\n")\n'
                             'if sys.argv[1] == "fingerprint": print("synthetic-fingerprint")\n'
                             'else: print("injected publication failure",file=sys.stderr); sys.exit(42)\n')
        config = state / 'config.yaml'
        config.write_text('locus_mode: open_only\n')
        config.chmod(0o600)
        db = state / 'conversations.db'
        with sqlite3.connect(db) as connection:
            connection.execute('create table legacy_data (value text)')
            connection.execute('insert into legacy_data values ("preserve me")')
        db.chmod(0o600)
        # Opaque existing records are preservation sentinels, not live-host proof.
        hosts = state / 'hosts'
        hosts.mkdir(mode=0o700)
        (hosts / 'existing.json').write_text('{"sentinel":"never touch"}')
        (hosts / 'existing.token').write_text('test-only sentinel')
        env = dict(os.environ, CERCANO_DISABLE_CREDENTIALS='1',
                   CERCANO_DEVELOPMENT_STATE=str(state), CERCANO_REPO=str(mock),
                   TEST_TRACE=str(trace))
        before = snapshot(state)
        code, output = invoke(script, env)
        assert code == 78 and 'launch refused' in output, (code, output)
        assert snapshot(state) == before, 'legacy refusal mutated state'
        assert not trace.exists(), 'legacy refusal reached publisher'
        results.append({'case': 'legacy-state actual entrypoint PTY', 'exit': code,
                        'unchanged_state': True, 'output': output})
        with sqlite3.connect(db) as connection:
            connection.execute('create table coexistence_metadata (contract text, schema_version integer)')
            connection.execute('insert into coexistence_metadata values ("cercano-development-owned-v2",2)')
            connection.execute('pragma user_version=2')
        # This SQL prerequisite fixture is NOT a prepared production database.
        # It cannot reach any server: publication must fail first.
        with tempfile.TemporaryDirectory(prefix='el-', dir='/tmp') as rt:
            env['CERCANO_RUNTIME_ROOT'] = str(Path(rt).resolve())
            before = snapshot(state)
            code, output = invoke(script, env)
            assert code == 78 and 'injected publication failure' in output, (code, output)
            assert snapshot(state) == before, 'publication refusal changed file contents'
            calls = trace.read_text().splitlines()
            assert len(calls) == 2 and 'fingerprint' in calls[0] and 'publish' in calls[1], calls
            assert '--sign' in calls[1] and '--reuse' in calls[1], calls
            assert not (state / 'selections').exists(), 'failed publication acquired a session'
            results.append({'case': 'mock publication failure actual entrypoint PTY',
                            'exit': code, 'unchanged_file_contents': True,
                            'output': output, 'publisher_calls': calls})
    report = {'scope': 'negative entrypoint only; compiled production A/B journey NOT verified',
              'results': results}
    evidence.parent.mkdir(parents=True, exist_ok=True)
    evidence.write_text(json.dumps(report, indent=2) + '\n')
    print('PASS: ordinary entrypoint PTY legacy refusal and mock publication failure; NOT compiled A/B acceptance')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--repo', required=True, type=Path)
    parser.add_argument('--evidence', required=True, type=Path)
    args = parser.parse_args()
    run(args.repo.resolve(), args.evidence)
