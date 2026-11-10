"""Runner contract only: disposable Go modules, never native acceptance."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

RUNNER = Path(__file__).with_name("checked-go-test.py")
PARENT = "TestHistoricalProductionController"
EXPECTED = [PARENT] + [PARENT + "/" + mode for mode in
                       ("drain", "reserved", "blocked-timeout")]
VALID = '''func TestHistoricalProductionController(t *testing.T) {
 for _, mode := range []string{"drain", "reserved", "blocked-timeout"} {
  t.Run(mode, func(t *testing.T) {})
 }
}'''


class CheckedGoTest(unittest.TestCase):
    def run_fixture(self, body, pattern="^" + PARENT + "$"):
        with tempfile.TemporaryDirectory(prefix="recovery-runner-") as directory:
            root = Path(directory)
            (root / "go.mod").write_text("module runnerfixture\ngo 1.26.0\n")
            (root / "runner_test.go").write_text(
                'package runnerfixture\nimport "testing"\n' + body + '\n')
            env = dict(os.environ, GOTOOLCHAIN="local", GOPROXY="off", GOSUMDB="off",
                       GOFLAGS="", GOWORK="off")
            return subprocess.run(
                [sys.executable, str(RUNNER), ".", pattern, "20s", *EXPECTED],
                cwd=root, env=env, text=True, stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT, timeout=60)

    def assert_rejected(self, body, pattern="^" + PARENT + "$"):
        result = self.run_fixture(body, pattern)
        self.assertNotEqual(result.returncode, 0, result.stdout)
        if 't.Fatal("failure")' not in body:
            self.assertIn('Intended tests did not all execute and pass', result.stdout)
        return result.stdout

    def test_missing_selection(self):
        self.assert_rejected('func TestUnrelated(t *testing.T) {}')

    def test_unrelated_pass_does_not_satisfy_selection(self):
        self.assert_rejected('func TestUnrelated(t *testing.T) {}', '^TestUnrelated$')

    def test_skipped_parent(self):
        self.assert_rejected('func ' + PARENT + '(t *testing.T) { t.Skip("opt in") }')

    def test_missing_child(self):
        self.assert_rejected(VALID.replace(', "blocked-timeout"', ''))

    def test_skipped_child(self):
        self.assert_rejected(VALID.replace('func(t *testing.T) {}',
                            'func(t *testing.T) { if mode == "reserved" { t.Skip("skip") } }'))

    def test_failed_command(self):
        self.assert_rejected(VALID.replace('func(t *testing.T) {}',
                            'func(t *testing.T) { t.Fatal("failure") }'))

    def test_nonzero_intended_execution(self):
        result = self.run_fixture(VALID)
        self.assertEqual(result.returncode, 0, result.stdout)
        self.assertIn("Verified 4 intended tests", result.stdout)

    def test_archived_entrypoint_refuses(self):
        result = subprocess.run(['sh', str(RUNNER.with_name('run.sh'))],
                                text=True, capture_output=True, timeout=5)
        self.assertEqual(result.returncode, 2)
        self.assertIn('Archived orphan runner', result.stderr)


if __name__ == "__main__":
    unittest.main()
