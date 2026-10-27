#!/usr/bin/env python3
"""Deterministic fixture-only publisher tests. Never invokes launcher/live app.
Run: python3 source/server/scripts/publish-dev-generation_test.py
Only fake Go outputs are validated, not executed. All homes/repos/roots are temp.
"""
import hashlib
import json
import os
import shutil
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

PUBLISHER = Path(__file__).with_name("publish-dev-generation.py").resolve()


class PublicationTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="publisher fixtures with spaces ")
        self.base = Path(self.temp.name).resolve()
        self.home = self.base / "empty home"
        self.home.mkdir()
        self.repo = self.base / "source repo"
        self.root = self.base / "generation root"
        for module in ("source/server", "source/clients/cli"):
            directory = self.repo / module
            directory.mkdir(parents=True)
            (directory / "go.mod").write_text("module fixture\n")
        self.env = {"PATH": os.environ["PATH"], "HOME": str(self.home),
                    "XDG_CONFIG_HOME": str(self.home / "config"),
                    "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.devnull,
                    "VERSION": "fixture-1", "FAKE_ROOT": str(self.root)}
        self.fake = self.base / "fake go"
        self.fake.write_text(f"#!{sys.executable}\n" + '''import os, sys
from pathlib import Path
if sys.argv[1:] == ["version"]:
    print("go version fixture")
    sys.exit(0)
assert sys.argv[1] == "build", sys.argv
if os.environ.get("MUTATE_SOURCE"):
    Path(os.environ["MUTATE_SOURCE"]).write_text("mutation")
output = Path(sys.argv[sys.argv.index("-o") + 1])
with (Path(os.environ["HOME"]) / "build-log").open("a") as log:
    log.write(str(output) + "\\n")
mode = os.environ.get("FAKE_MODE", "ok")
if output.name == "cercano-cli":
    if mode == "fail":
        sys.exit(42)
    if mode == "race":
        (Path(os.environ["FAKE_ROOT"]) / "test-gen").mkdir()
    if mode == "symlink":
        output.symlink_to(output.parent / "cercano")
        sys.exit(0)
output.write_bytes((b"bad!" if mode == "invalid" and output.name == "cercano-cli"
                    else b"\\x7fELF") + b"FIXTURE NEVER EXECUTE" * 8)
output.chmod(0o755)
''')
        self.fake.chmod(0o755)
        self.git("init", "-q")
        self.git("add", ".")
        self.git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
                 "-c", "commit.gpgsign=false", "commit", "-qm", "fixture")

    def tearDown(self):
        # Test-owned fixture generations only; publisher never deletes published dirs.
        for directory, _, _ in os.walk(self.base):
            Path(directory).chmod(0o700)
        self.temp.cleanup()

    def git(self, *args):
        subprocess.run(["git", "-C", str(self.repo), *args], env=self.env,
                       check=True, capture_output=True)

    def publish(self, mode="ok", extra=()):
        result = subprocess.run([sys.executable, str(PUBLISHER), "publish",
                                 "--root", str(self.root), "--generation", "test-gen",
                                 "--repo", str(self.repo), "--go", str(self.fake), *extra],
                                env={**self.env, "FAKE_MODE": mode}, text=True,
                                capture_output=True)
        self.assertEqual(list(self.root.glob(".building-*")), [])
        return result

    def fingerprint(self):
        return subprocess.run([sys.executable, str(PUBLISHER), "fingerprint",
                               "--repo", str(self.repo), "--go", str(self.fake)],
                              env=self.env, text=True, capture_output=True)

    def test_fingerprint_content_not_mtime(self):
        first = self.fingerprint()
        self.assertEqual(first.returncode, 0, first.stderr)
        source = self.repo / "source/server/new.go"
        source.write_text("package main\n")
        second = self.fingerprint()
        self.assertNotEqual(first.stdout, second.stdout)
        os.utime(source, (1, 1))
        self.assertEqual(second.stdout, self.fingerprint().stdout)
        source.write_text("package changed\n")
        self.assertNotEqual(second.stdout, self.fingerprint().stdout)

    def test_unsafe_goflags_refused(self):
        self.env["GOFLAGS"] = "-overlay=/tmp/unknown.json"
        result = self.publish()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.root / "test-gen").exists())

    def test_fingerprint_version_and_flags(self):
        first = self.fingerprint().stdout
        self.env["VERSION"] = "other"
        self.assertNotEqual(first, self.fingerprint().stdout)
        second = self.fingerprint().stdout
        self.env["GOARCH"] = "arm64"
        self.assertNotEqual(second, self.fingerprint().stdout)

    def test_expected_fingerprint_and_reuse(self):
        fingerprint = self.fingerprint().stdout.strip()
        extra = ["--fingerprint", fingerprint, "--reuse"]
        self.assertEqual(self.publish(extra=extra).returncode, 0)
        log = (self.home / "build-log").read_bytes()
        self.assertEqual(self.publish(extra=extra).returncode, 0)
        self.assertEqual(log, (self.home / "build-log").read_bytes())
        (self.repo / "source/server/new.go").write_text("changed")
        self.assertNotEqual(self.publish(extra=extra).returncode, 0)

    def test_source_changes_during_build_rejected(self):
        self.env["MUTATE_SOURCE"] = str(self.repo / "source/server/new.go")
        result = self.publish()
        self.assertNotEqual(result.returncode, 0, result.stderr)
        self.assertFalse((self.root / "test-gen").exists())

    @unittest.skipUnless(sys.platform == "darwin", "Darwin signing")
    def test_signing_adhoc_and_failure(self):
        tools = self.base / "tools"
        tools.mkdir()
        signer = tools / "codesign"
        signer.write_text(f"#!{sys.executable}\n" +
                          "import os,sys\n" +
                          "assert '--sign' not in sys.argv or sys.argv[sys.argv.index('--sign')+1] == '-'\n" +
                          "sys.exit(int(os.environ.get('SIGN_FAIL', '0')))\n")
        signer.chmod(0o755)
        self.env["PATH"] = str(tools) + os.pathsep + self.env["PATH"]
        self.env["SIGN_FAIL"] = "1"
        self.assertNotEqual(self.publish(extra=["--sign"]).returncode, 0)
        self.assertFalse((self.root / "test-gen").exists())
        self.env["SIGN_FAIL"] = "0"
        result = self.publish(extra=["--sign"])
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_concurrent_reuse_publishes_one_complete_pair(self):
        fingerprint = self.fingerprint().stdout.strip()
        command = [sys.executable, str(PUBLISHER), "publish", "--root", str(self.root),
                   "--generation", "test-gen", "--repo", str(self.repo),
                   "--go", str(self.fake), "--fingerprint", fingerprint, "--reuse"]
        processes = [subprocess.Popen(command, env=self.env, stdout=subprocess.PIPE,
                                      stderr=subprocess.PIPE, text=True) for _ in range(2)]
        for process in processes:
            out, err = process.communicate(timeout=30)
            self.assertEqual(process.returncode, 0, err)
            self.assertEqual(out.strip(), str(self.root / "test-gen"))
        self.assertEqual(len(list((self.root / "test-gen").iterdir())), 3)
        self.assertEqual(list(self.root.glob(".building-*")), [])

    @unittest.skipUnless(sys.platform == "darwin", "Darwin signing")
    def test_requested_signing_unavailable(self):
        tools = self.base / "only git"
        tools.mkdir()
        (tools / "git").symlink_to(shutil.which("git"))
        self.env["PATH"] = str(tools)
        result = self.publish(extra=["--sign"])
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("signing unavailable", result.stderr)
        self.assertFalse((self.root / "test-gen").exists())

    def test_pair_manifest_and_spaces(self):
        result = self.publish()
        self.assertEqual(result.returncode, 0, result.stderr)
        generation = self.root / "test-gen"
        self.assertEqual(result.stdout.strip(), str(generation))
        self.assertEqual({p.name for p in generation.iterdir()},
                         {"cercano", "cercano-cli", "manifest.json"})
        manifest = json.loads((generation / "manifest.json").read_text())
        self.assertEqual(manifest["version"], "fixture-1")
        self.assertFalse(manifest["source_dirty"])
        self.assertTrue(manifest["build_id"])
        for name in ("cercano", "cercano-cli"):
            binary = generation / name
            self.assertEqual(manifest["binaries"][name]["sha256"],
                             hashlib.sha256(binary.read_bytes()).hexdigest())
            self.assertEqual(binary.stat().st_mode & 0o777, 0o555)
        outputs = (self.home / "build-log").read_text().splitlines()
        self.assertEqual(len(outputs), 2)
        self.assertEqual(Path(outputs[0]).parent, Path(outputs[1]).parent)
        self.assertEqual(Path(outputs[0]).parent.parent, self.root)
        self.assertFalse((self.home / "bin").exists())

    def test_partial_build_failure_never_publishes(self):
        result = self.publish("fail")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.root / "test-gen").exists())
        self.assertEqual(len((self.home / "build-log").read_text().splitlines()), 2)

    def test_existing_generation_unchanged_without_build(self):
        self.assertEqual(self.publish().returncode, 0)
        manifest = (self.root / "test-gen/manifest.json").read_bytes()
        log = (self.home / "build-log").read_bytes()
        result = self.publish()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("already exists", result.stderr)
        self.assertEqual((self.root / "test-gen/manifest.json").read_bytes(), manifest)
        self.assertEqual((self.home / "build-log").read_bytes(), log)

    def test_racing_empty_destination_not_replaced(self):
        result = self.publish("race")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("already exists", result.stderr)
        self.assertEqual(list((self.root / "test-gen").iterdir()), [])

    def test_invalid_second_output_not_published(self):
        result = self.publish("invalid")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("header", result.stderr)
        self.assertFalse((self.root / "test-gen").exists())

    def test_explicit_command_and_root_required(self):
        result = subprocess.run([sys.executable, str(PUBLISHER)], env=self.env,
                                capture_output=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.root.exists())

    def test_installation_root_refused(self):
        result = subprocess.run([sys.executable, str(PUBLISHER), "publish",
                                 "--root", str(self.home / "bin/generations"),
                                 "--generation", "test-gen", "--repo", str(self.repo),
                                 "--go", str(self.fake)], env=self.env,
                                capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("prohibited", result.stderr)
        self.assertFalse((self.home / "bin").exists())
        self.assertFalse((self.home / "build-log").exists())

    def test_symlink_output_not_published(self):
        result = self.publish("symlink")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.root / "test-gen").exists())


if __name__ == "__main__":
    unittest.main(verbosity=2)
