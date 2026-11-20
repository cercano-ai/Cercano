#!/usr/bin/env python3
"""Portable regression checks for review-only Chocolatey workflow boundaries."""
import importlib.util
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("release_workflow_tests", Path(__file__).with_name("test-release-workflow.py"))
helpers = importlib.util.module_from_spec(spec)
spec.loader.exec_module(helpers)
REPO = Path(__file__).resolve().parents[1]


class ChocolateyWorkflowTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.workflow = helpers.load_workflow_file(REPO / ".github/workflows/package-chocolatey.yml")
        cls.job = cls.workflow["jobs"]["package"]
        cls.steps = cls.job["steps"]

    def test_manual_only_no_submission_credentials_or_builds(self):
        trigger = next(self.workflow[key] for key in ("on", True, "true") if key in self.workflow)
        self.assertEqual(set(trigger), {"workflow_dispatch"})
        self.assertEqual(set(trigger["workflow_dispatch"]["inputs"]), {"version", "sha256"})
        self.assertEqual(self.workflow["permissions"], {"contents": "read"})
        self.assertNotIn("environment", self.job)
        text = str(self.workflow).lower()
        for forbidden in ("secrets.", "choco push", "choco install", "--api-key", "gh release", "git push", "build-windows-release", "workflow_call"):
            self.assertNotIn(forbidden, text)
        for step in self.steps:
            self.assertNotIn("continue-on-error", step)
            self.assertNotIn("if", step)
            self.assertNotIn("${{ inputs.", step.get("run", ""))

    def test_exact_tooling_not_release_tag_and_artifact_boundary(self):
        checkout = next(s for s in self.steps if s.get("uses", "").startswith("actions/checkout@"))
        self.assertEqual(checkout["with"]["ref"], "${{ github.workflow_sha }}")
        self.assertFalse(checkout["with"]["persist-credentials"])
        self.assertEqual(checkout["with"]["path"], "packaging-tools")
        scripts = "\n".join(s.get("run", "") for s in self.steps)
        self.assertIn('git -C packaging-tools rev-parse HEAD', scripts)
        for tool in ("download_release.py", "pack_packages.py", "check_packages.py"):
            self.assertIn(f"packaging-tools/release/chocolatey/{tool}", scripts)
        self.assertIn('--sha256 "$SHA256"', scripts)
        self.assertNotIn("sha256sum", scripts)  # never invent expected digest from download
        upload = self.steps[-1]
        self.assertEqual(upload["with"]["path"].splitlines(), [
            "dist/chocolatey/packages/*.nupkg", "dist/chocolatey/packages/*.nupkg.sha256"])
        self.assertEqual(upload["with"]["if-no-files-found"], "error")

    def test_validation_executes_before_checkout_and_rejects_bad_inputs(self):
        script = self.steps[0]["run"]
        good = dict(VERSION="1.2.3", SHA256="a" * 64, TOOLING_SHA="b" * 40)
        with tempfile.TemporaryDirectory() as directory:
            for key, bad in [("VERSION", "v1.2.3"), ("VERSION", "01.2.3"),
                             ("VERSION", "$(touch injected)"), ("VERSION", "1.2.3\n"),
                             ("SHA256", "a" * 63), ("SHA256", "g" * 64),
                             ("TOOLING_SHA", "main"), ("TOOLING_SHA", "b" * 39)]:
                with self.subTest(key=key, bad=bad):
                    result = subprocess.run([helpers.bash_executable(), "-c", script],
                                            env=dict(os.environ, **(good | {key: bad})), cwd=directory)
                    self.assertNotEqual(result.returncode, 0)
            self.assertFalse((Path(directory) / "injected").exists())
            self.assertEqual(subprocess.run([helpers.bash_executable(), "-c", script],
                                           env=dict(os.environ, **good), cwd=directory).returncode, 0)

    def test_revision_verification_accepts_different_source_and_rejects_moved_tools(self):
        integrated = helpers.load_workflow_file(REPO / ".github/workflows/release-macos.yml")
        scripts = [next(step["run"] for step in steps
                        if step.get("name") == "Verify packaging tooling revision")
                   for steps in (self.steps, integrated["jobs"]["build-windows"]["steps"])]
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            tools = root / "packaging-tools"
            tools.mkdir()

            def git(path, *args):
                return subprocess.check_output(["git", "-C", str(path), *args], text=True).strip()

            for repo, message in ((root, "old source without packaging tools"),
                                  (tools, "corrected immutable tooling")):
                git(repo, "init", "-q")
                git(repo, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
                    "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", message)
            source_sha, tooling_sha = git(root, "rev-parse", "HEAD"), git(tools, "rev-parse", "HEAD")
            self.assertNotEqual(source_sha, tooling_sha)
            for script in scripts:
                for expected, code in ((tooling_sha, 0), (source_sha, 1)):
                    result = subprocess.run([helpers.bash_executable(), "-c", script], cwd=root,
                                            env=dict(os.environ, TOOLING_SHA=expected), capture_output=True)
                    self.assertEqual(result.returncode, code)
            self.assertEqual(git(root, "rev-parse", "HEAD"), source_sha)

    def test_integrated_source_tag_and_tools_have_distinct_checkouts(self):
        workflow = helpers.load_workflow_file(REPO / ".github/workflows/release-macos.yml")
        job = workflow["jobs"]["build-windows"]
        checkouts = [s["with"] for s in job["steps"] if s.get("uses", "").startswith("actions/checkout@")]
        self.assertEqual(checkouts[0]["ref"], "${{ steps.source.outputs.ref }}")
        self.assertEqual(checkouts[1]["ref"], "${{ github.workflow_sha }}")
        self.assertEqual(checkouts[1]["path"], "packaging-tools")
        self.assertTrue(all(c["persist-credentials"] is False for c in checkouts))
        scripts = "\n".join(s.get("run", "") for s in job["steps"])
        self.assertIn("python scripts/build-windows-release.py", scripts)
        self.assertIn("python packaging-tools/release/chocolatey/pack_packages.py", scripts)
        self.assertNotIn("python release/chocolatey/pack_packages.py", scripts)
        self.assertNotIn("environment", job)


if __name__ == "__main__":
    unittest.main()
