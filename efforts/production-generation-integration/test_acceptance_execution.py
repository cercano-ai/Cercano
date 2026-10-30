"""Only fake Python children; no application, credentials, builds or network."""
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import sys
import tempfile
import time
import unittest
from datetime import datetime, timezone
from unittest.mock import patch

from acceptance_preparation import prepare_real_cloud_launch

HARNESS = Path(__file__).resolve().parents[1] / 'dev-host-generations/everyday-launcher-compiled.py'
spec = importlib.util.spec_from_file_location('compiled_harness', HARNESS)
harness = importlib.util.module_from_spec(spec)
spec.loader.exec_module(harness)


class PreparedExecutionTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.base = Path(self.tmp.name).resolve()
        self.home = self.base / 'home'
        self.root = self.base / 'private'
        self.home.mkdir(mode=0o700)
        self.root.mkdir(mode=0o700)
        self.ledger = self.root / 'usage.json'
        self.state = dict(started=datetime.now(timezone.utc).isoformat(), attempts=3,
                          input_tokens=20, output_tokens=10, halted=False, active={})
        self.write_ledger()
        self.script = self.base / 'fake.py'
        self.script.write_text('import json, os, time\nprint(json.dumps(dict(os.environ)), flush=True)\ntime.sleep(60)\n')
        self.argv = [sys.executable, '-B', str(self.script)]
        self.plan = prepare_real_cloud_launch(home=str(self.home), private_root=str(self.root),
            credential_policy='disabled', ledger_path=str(self.ledger),
            **{role+'_argv': self.argv for role in ('launcher', 'host', 'cli')})
        self.hashes = {str(p): hashlib.sha256(p.read_bytes()).hexdigest()
                       for p in (Path(sys.executable), self.script)}

    def write_ledger(self):
        self.ledger.write_text(json.dumps(self.state))
        self.ledger.chmod(0o600)

    def start(self, command=None):
        t = harness.Terminal(command or self.plan.cli, log=self.root/'fake.pty',
                             artifact_sha256=self.hashes)
        self.addCleanup(t.stop)
        return t

    def test_prepared_terminal_replaces_ambient_and_reaps_owned_child(self):
        before = self.ledger.read_bytes()
        with patch.dict(os.environ, {'HOME':'/hostile', 'OPENAI_API_KEY':'hostile',
                                     'CERCANO_DISABLE_CREDENTIALS':'0'}, clear=True):
            t = self.start()
            end = time.monotonic()+3
            while b'\n' not in t.data and time.monotonic()<end:
                t.pump()
            env = json.loads(t.data.splitlines()[0])
            self.assertEqual(env['HOME'], str(self.home))
            self.assertEqual(env['CERCANO_DISABLE_CREDENTIALS'], '1')
            self.assertNotIn('OPENAI_API_KEY', env)
            t.stop()
            self.assertIsNotNone(t.p.returncode)
        self.assertEqual(before, self.ledger.read_bytes())

    def test_nonpty_all_roles_replace_env_and_worker_inherits(self):
        self.script.write_text('''import json, os, subprocess, sys
if len(sys.argv) == 1:
    child = subprocess.run([sys.executable, '-B', __file__, 'worker'],
                           check=True, capture_output=True, text=True)
    print(json.dumps({'env':dict(os.environ), 'worker':json.loads(child.stdout)}))
else:
    print(json.dumps(dict(os.environ)))
''')
        self.hashes[str(self.script)] = hashlib.sha256(self.script.read_bytes()).hexdigest()
        with patch.dict(os.environ, {'OPENAI_API_KEY':'hostile', 'HTTP_PROXY':'hostile',
                                     'CERCANO_DISABLE_CREDENTIALS':'0'}, clear=True):
            for role in ('launcher', 'host', 'cli'):
                with self.subTest(role=role):
                    result = harness.run_prepared(getattr(self.plan, role),
                        artifact_sha256=self.hashes, timeout=3)
                    self.assertEqual(result.returncode, 0)
                    data = json.loads(result.stdout)
                    for env in (data['env'], data['worker']):
                        self.assertEqual(env['HOME'], str(self.home))
                        self.assertEqual(env['CERCANO_DISABLE_CREDENTIALS'], '1')
                        self.assertNotIn('OPENAI_API_KEY', env)
                        self.assertNotIn('HTTP_PROXY', env)

    def test_timeout_kills_only_owned_child(self):
        # Real fake executable ignores TERM, exercising bounded escalation.
        self.script.write_text('import signal, time\nsignal.signal(signal.SIGTERM, signal.SIG_IGN)\ntime.sleep(60)\n')
        self.hashes[str(self.script)] = hashlib.sha256(self.script.read_bytes()).hexdigest()
        popen = harness.subprocess.Popen
        children = []
        def spawn(*args, **kwargs):
            child = popen(*args, **kwargs)
            children.append(child)
            return child
        with patch.object(harness.subprocess, 'Popen', side_effect=spawn):
            with self.assertRaises(harness.subprocess.TimeoutExpired):
                harness.run_prepared(self.plan.host, artifact_sha256=self.hashes, timeout=.2)
        self.assertEqual(len(children), 1)
        self.assertIsNotNone(children[0].returncode)

    def test_rejects_overlay_missing_digest_and_bad_argv(self):
        from dataclasses import replace
        with patch.object(harness.subprocess, 'Popen', side_effect=AssertionError('spawned')):
            with self.assertRaises(ValueError):
                harness.Terminal(self.plan.cli, env={'OPENAI_API_KEY':'overlay'},
                                 log=self.root/'bad.pty', artifact_sha256=self.hashes)
            for command, hashes in ((self.plan.cli, {}),
                                    (replace(self.plan.cli, argv=('python3',)), self.hashes),
                                    (self.plan.cli, {sys.executable:self.hashes[sys.executable]})):
                with self.assertRaises(ValueError):
                    harness.run_prepared(command, artifact_sha256=hashes, timeout=1)
        self.ledger.unlink()
        with self.assertRaises(FileNotFoundError):
            self.start()
        self.assertFalse(self.ledger.exists())

    def test_refuses_bad_artifact_before_popen(self):
        self.script.write_text('raise Exception("must not execute")')
        with patch.object(harness.subprocess, 'Popen', side_effect=AssertionError('spawned')):
            with self.assertRaises(ValueError):
                self.start()

    def test_refuses_ledger_states_without_modification_or_spawn(self):
        for change in ({'halted': True}, {'started':'2000-01-01T00:00:00Z'},
                       {'attempts':40}, {'input_tokens':300000}, {'output_tokens':30000},
                       {'active':{'3':'2000-01-01T00:00:00Z'}},
                       {'active':{'3':'2099-01-01T00:00:00Z'}},
                       {'attempts':-1}, {'halted':'false'}, {'active':[]}):
            with self.subTest(change=change):
                old = dict(self.state)
                self.state.update(change)
                self.write_ledger()
                before = self.ledger.read_bytes()
                with patch.object(harness.subprocess, 'Popen', side_effect=AssertionError('spawned')):
                    with self.assertRaises(ValueError):
                        self.start()
                self.assertEqual(before, self.ledger.read_bytes())
                self.state = old
