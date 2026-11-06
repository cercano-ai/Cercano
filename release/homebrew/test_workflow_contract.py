"""Execute both workflow tap shell bodies offline through the effective gate."""
import hashlib
import importlib.util
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('workflow_fixture', ROOT/'scripts/test-release-workflow.py')
fixture = importlib.util.module_from_spec(spec)
spec.loader.exec_module(fixture)


class WorkflowContractTest(unittest.TestCase):
    def test_main_and_recovery_full_set_and_gate(self):
        for filename, job_name in [('release-macos.yml', 'build'),
                                   ('update-homebrew.yml', 'update-homebrew')]:
            workflow = fixture.load_workflow_file(ROOT/'.github/workflows'/filename)
            job = workflow['jobs'][job_name]
            self.assertEqual(job['environment'], 'release')
            self.assertEqual(job['concurrency']['group'], 'cercano-homebrew-tap')
            self.assertFalse(job['concurrency']['cancel-in-progress'])
            step = next(s for s in job['steps'] if 'update_tap.py' in s.get('run', ''))
            self.assertIn('--formula-dir dist', step['run'])
            self.assertNotIn('--formula dist/', step['run'])
            if filename == 'update-homebrew.yml':
                provenance = next(i for i,s in enumerate(job['steps']) if 'check_release_run.py' in s.get('run', ''))
                self.assertLess(provenance, job['steps'].index(step))
                download = next(s for s in job['steps'] if s.get('uses', '').startswith('actions/download-artifact'))
                self.assertEqual(download['with']['run-id'], '${{ inputs.run_id }}')
            with tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                shutil.copytree(ROOT/'release/homebrew', root/'release/homebrew', ignore=shutil.ignore_patterns('__pycache__'))
                shutil.copytree(ROOT/'scripts', root/'scripts', ignore=shutil.ignore_patterns('__pycache__'))
                (root/'dist').mkdir()
                name = 'cercano-1.2.3-darwin-arm64.tar.gz'
                body = b'offline guard fixture'
                digest = hashlib.sha256(body).hexdigest()
                (root/'dist'/name).write_bytes(body)
                (root/'dist'/(name+'.sha256')).write_text(digest+'  '+name+'\n')
                result = subprocess.run(['bash', '-c', step['run']], cwd=root,
                                        env=dict(os.environ, VERSION='1.2.3', EXPECTED_SHA256=digest,
                                                 HOMEBREW_TAP_TOKEN='fixture-only'), capture_output=True, text=True)
                self.assertEqual(result.returncode, 1, result.stderr)
                self.assertIn('publication is disabled', result.stderr)
                self.assertNotIn('unrecognized arguments', result.stderr)
