#!/usr/bin/env python3
"""Structural contracts for the parallel enterprise-client CI jobs.

Run with Python and PyYAML (or Ruby), like test-release-workflow.py. These
checks do not execute Go suites or prove a hosted Actions run succeeds.
"""

import json
import subprocess
import unittest
from pathlib import Path

try:
    import yaml
except ImportError:
    yaml = None

WORKFLOW = Path(__file__).resolve().parents[1] / ".github/workflows/enterprise-client.yml"

# Keep the pre-split broad package lists exact; inference is race-only.
PACKAGES = (
    "./internal/enterprise ./internal/modelpolicy ./internal/managedsettings/... "
    "./internal/managedrouting ./internal/cloudfactory ./internal/hostsvc/providers "
    "./internal/hostsvc/persistence ./internal/contextedit ./internal/dispatch "
    "./internal/loopcompact ./internal/loop/... ./internal/usage "
    "./internal/capabilities/builtins ./internal/hostsvc/tools ./internal/runner "
    "./internal/server ./internal/mcp ./internal/agent ./internal/llm/... "
    "./internal/engine/... ./internal/localruntime/... ./internal/worker "
)
RACE = "go test -race " + PACKAGES + "./internal/inference/... ./cmd/cercano"
VET = "go vet " + PACKAGES + "./cmd/cercano"


def load_workflow():
    if yaml is not None:
        return yaml.safe_load(WORKFLOW.read_text())
    result = subprocess.run(
        ["ruby", "-e", "require 'yaml'; require 'json'; "
         "puts JSON.generate(YAML.safe_load(File.read(ARGV[0])))", str(WORKFLOW)],
        capture_output=True, text=True, check=True,
    )
    return json.loads(result.stdout)


class EnterpriseClientWorkflowTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.workflow = load_workflow()
        cls.jobs = cls.workflow["jobs"]

    def test_three_independent_dual_platform_jobs(self):
        self.assertEqual(set(self.jobs), {"race-tests", "full-suite", "static-and-build"})
        for name, job in self.jobs.items():
            with self.subTest(job=name):
                self.assertNotIn("needs", job)
                self.assertNotIn("if", job)
                self.assertNotIn("continue-on-error", job)
                self.assertEqual(job["strategy"], {
                    "matrix": {"os": ["ubuntu-latest", "macos-latest"]}})
                self.assertEqual(job["runs-on"], "${{ matrix.os }}")
                self.assertEqual(job["defaults"], {
                    "run": {"working-directory": "source/server"}})

    def test_each_job_checks_out_and_sets_up_same_go_cache(self):
        for name, job in self.jobs.items():
            with self.subTest(job=name):
                self.assertEqual(job["steps"][:2], [
                    {"uses": "actions/checkout@v4"},
                    {"uses": "actions/setup-go@v5", "with": {
                        "go-version-file": "source/server/go.mod",
                        "cache-dependency-path": "source/server/go.sum"}},
                ])

    def test_exact_commands_partition_all_existing_coverage(self):
        expected = {
            "race-tests": ["go test -race ./internal/broker", RACE],
            "full-suite": ["go test ./... -count=1"],
            "static-and-build": [
                "go test -tags=enterprise_demo ./internal/server -run '^$'",
                VET, "go build ./cmd/cercano"],
        }
        for name, commands in expected.items():
            with self.subTest(job=name):
                steps = self.jobs[name]["steps"][2:]
                self.assertEqual([step["run"] for step in steps], commands)
                for step in steps:
                    self.assertNotIn("if", step)
                    self.assertNotIn("continue-on-error", step)
                    self.assertNotIn("working-directory", step)

    def test_path_filtered_triggers_and_read_only_permissions(self):
        triggers = next(self.workflow[key] for key in ("on", True, "true")
                        if key in self.workflow)
        self.assertEqual(set(triggers), {"pull_request", "push"})
        self.assertEqual(triggers["pull_request"], triggers["push"])
        self.assertIn(".github/workflows/enterprise-client.yml",
                      triggers["pull_request"]["paths"])
        self.assertEqual(self.workflow["permissions"], {"contents": "read"})
        for job in self.jobs.values():
            self.assertNotIn("permissions", job)


if __name__ == "__main__":
    unittest.main()
