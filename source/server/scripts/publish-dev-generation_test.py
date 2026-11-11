#!/usr/bin/env python3
"""Deterministic fixture-only publisher tests. Never invokes launcher/live app.
Run: python3 source/server/scripts/publish-dev-generation_test.py
Only fake Go outputs are validated, not executed. All homes/repos/roots are temp.
"""
import errno
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
if sys.argv[1] == "env":
    if os.environ.get("FAIL_FINAL_ENV") and Path(os.environ["FAIL_FINAL_ENV"]).exists():
        print("SECRET_TOOL_STDERR", file=sys.stderr)
        sys.exit(42)
    import json
    print(json.dumps({"CGO_ENABLED": os.environ.get("CGO_ENABLED", "0"),
                      "GOARCH": os.environ.get("GOARCH", "native"),
                      "GOMOD": str(Path.cwd() / "go.mod"),
                      "GOGCCFLAGS": "-O2 -ffile-prefix-map=/tmp/go-build%d=/tmp/go-build" % os.getpid()}))
    if os.environ.get("ENV_LOG"):
        with Path(os.environ["ENV_LOG"]).open("a") as log:
            log.write(str(Path.cwd()) + "\\n")
    sys.exit(0)
if sys.argv[1:3] == ["mod", "edit"]:
    import subprocess
    sys.exit(subprocess.call([REAL_GO, *sys.argv[1:]]))
if sys.argv[1:] == ["version"]:
    print("go version fixture")
    sys.exit(0)
assert sys.argv[1] == "build", sys.argv
if os.environ.get("FAIL_FINAL_ENV"):
    Path(os.environ["FAIL_FINAL_ENV"]).touch()
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
        self.fake.write_text(self.fake.read_text().replace('REAL_GO', repr(shutil.which('go'))))
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

    def fingerprint(self, cwd=None):
        return subprocess.run([sys.executable, str(PUBLISHER), "fingerprint",
                               "--repo", str(self.repo), "--go", str(self.fake)],
                              env=self.env, cwd=cwd, text=True, capture_output=True)

    def test_fingerprint_independent_of_caller_module(self):
        unrelated = self.base / "unrelated project"
        unrelated.mkdir()
        (unrelated / "go.mod").write_text("module unrelated\n")
        self.env["ENV_LOG"] = str(self.home / "env-log")
        first = self.fingerprint(cwd=self.repo / "source/server")
        second = self.fingerprint(cwd=unrelated)
        self.assertEqual(first.returncode, 0, first.stderr)
        self.assertEqual(second.returncode, 0, second.stderr)
        self.assertEqual(first.stdout, second.stdout)
        self.assertEqual((self.home / "env-log").read_text().splitlines(),
                         [str(self.repo / "source/server")] * 2)
        (self.repo / "source/server/go.mod").write_text("module changed\n")
        changed = self.fingerprint(cwd=unrelated)
        self.assertEqual(changed.returncode, 0, changed.stderr)
        self.assertNotEqual(first.stdout, changed.stdout)
        self.env["GOARCH"] = "arm64"
        changed_toolchain = self.fingerprint(cwd=unrelated)
        self.assertEqual(changed_toolchain.returncode, 0, changed_toolchain.stderr)
        self.assertNotEqual(changed.stdout, changed_toolchain.stdout)

    def test_native_go_module_diagnostic(self):
        # Diagnostic only: no compilation, SDK hashing, credentials, or app launch.
        go = shutil.which("go")
        if not go:
            self.skipTest("Go unavailable")
        import importlib.util
        from unittest.mock import patch
        spec = importlib.util.spec_from_file_location("publisher", PUBLISHER)
        publisher = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(publisher)
        env = {**self.env, "CGO_ENABLED": "0", "GOENV": "off",
               "GOWORK": "off", "GOTOOLCHAIN": "local"}
        modules = [self.repo / "source/server", self.repo / "source/clients/cli"]
        diagnostics = [json.loads(subprocess.check_output(
            [go, "env", "-json"], env=env, cwd=module, text=True))
            for module in modules]
        self.assertNotEqual(diagnostics[0]["GOMOD"], diagnostics[1]["GOMOD"])
        # Keep this a cheap native diagnostic test, not a full toolchain hash.
        identities = []
        original_cwd = Path.cwd()
        try:
            with patch.object(publisher, "tree_identity", return_value="fixture-goroot"), \
                    patch.object(publisher, "tree_batches", return_value=[]):
                for module in modules:
                    os.chdir(module)
                    identities.append(publisher.effective_identity(go, env, modules[0]))
        finally:
            os.chdir(original_cwd)
        self.assertEqual(identities[0], identities[1])
        self.assertNotIn("GOMOD", identities[0]["go_env"])

    def test_native_cgo_default_and_explicit_values(self):
        import importlib.util
        spec = importlib.util.spec_from_file_location("publisher", PUBLISHER)
        publisher = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(publisher)
        from unittest.mock import patch
        with patch.dict(os.environ, self.env, clear=True):
            self.assertNotIn("CGO_ENABLED", publisher.build_environment()[0])
            for value in ("0", "1"):
                os.environ["CGO_ENABLED"] = value
                self.assertEqual(publisher.build_environment()[0]["CGO_ENABLED"], value)

    def test_absolute_and_quoted_replacements_refused(self):
        module = self.repo / "source/server/go.mod"
        for target in (str(self.repo / "source/server"), '"' + str(self.repo / "source/server") + '"', '"../../../../outside space"'):
            module.write_text("module fixture\nreplace example.com/local => " + target + "\n")
            result = self.fingerprint()
            self.assertNotEqual(result.returncode, 0, target)

    def test_quoted_relative_replacement_stays_in_snapshot(self):
        target = self.repo / "source/local module"
        target.mkdir()
        (target / "go.mod").write_text("module example.com/local\n")
        module = self.repo / "source/server/go.mod"
        module.write_text('module fixture\nreplace example.com/local => "../local module"\n')
        original = module.read_bytes()
        result = self.publish()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(module.read_bytes(), original)

    def test_ignored_build_and_embed_inputs_not_excluded(self):
        (self.repo / ".gitignore").write_text("dist/\n__pycache__/\n")
        first = self.fingerprint().stdout
        for directory in ("dist", "__pycache__"):
            path = self.repo / "source/server" / directory
            path.mkdir()
            (path / "embedded.txt").write_text("input")
            second = self.fingerprint()
            self.assertEqual(second.returncode, 0, second.stderr)
            self.assertNotEqual(first, second.stdout)
            first = second.stdout

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

    def current(self):
        return subprocess.run([sys.executable, str(PUBLISHER), "publish-current",
                               "--root", str(self.root), "--repo", str(self.repo),
                               "--go", str(self.fake)], env=self.env, text=True,
                              capture_output=True)

    def test_legacy_launcher_collection_probe(self):
        log = self.home / "env-log"
        self.env["ENV_LOG"] = str(log)
        fingerprint = self.fingerprint().stdout.strip()
        self.assertEqual(self.publish(extra=["--fingerprint", fingerprint, "--reuse"]).returncode, 0)
        self.assertEqual(len(log.read_text().splitlines()), 4)
        log.write_text("")
        fingerprint = self.fingerprint().stdout.strip()
        self.assertEqual(self.publish(extra=["--fingerprint", fingerprint, "--reuse"]).returncode, 0)
        self.assertEqual(len(log.read_text().splitlines()), 2)

    def test_current_collection_counts_and_protocol(self):
        log = self.home / "env-log"
        self.env["ENV_LOG"] = str(log)
        first = self.current()
        self.assertEqual(first.returncode, 0, first.stderr)
        self.assertEqual(len(log.read_text().splitlines()), 3)
        destination = Path(first.stdout.strip())
        manifest = json.loads((destination / "manifest.json").read_text())
        self.assertEqual(first.stdout, str(self.root / manifest["fingerprint"]) + "\n")
        builds = (self.home / "build-log").read_bytes()
        log.write_text("")
        second = self.current()
        self.assertEqual(second.returncode, 0, second.stderr)
        self.assertEqual(second.stdout, first.stdout)
        self.assertEqual(len(log.read_text().splitlines()), 1)
        self.assertEqual(builds, (self.home / "build-log").read_bytes())
        (self.repo / "source/server/new.go").write_text("new input")
        log.write_text("")
        changed = self.current()
        self.assertEqual(changed.returncode, 0, changed.stderr)
        self.assertNotEqual(changed.stdout, first.stdout)
        self.assertEqual(len(log.read_text().splitlines()), 3)
        binary = Path(changed.stdout.strip()) / "cercano"
        binary.chmod(0o755)
        binary.write_bytes(b"corrupt")
        self.assertNotEqual(self.current().returncode, 0)

    def test_current_snapshot_and_final_checks(self):
        import argparse
        import importlib.util
        import io
        from contextlib import redirect_stdout
        from unittest.mock import patch
        spec = importlib.util.spec_from_file_location("publisher", PUBLISHER)
        publisher = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(publisher)
        args = argparse.Namespace(repo=str(self.repo), go=str(self.fake), sign=False,
                                  root=str(self.root), generation=None,
                                  fingerprint=None, reuse=True)
        for failure_call in (2, 3):
            for unavailable in (False, True):
                with self.subTest(call=failure_call, unavailable=unavailable):
                    original = publisher.inputs
                    calls = 0

                    def collect(*a, **kw):
                        nonlocal calls
                        calls += 1
                        result = original(*a, **kw)
                        if calls == failure_call:
                            if unavailable:
                                raise ValueError("fixture collection failure")
                            return ("changed", result[1], result[2])
                        return result

                    stdout = io.StringIO()
                    with patch.dict(os.environ, self.env, clear=True), \
                            patch.object(publisher, "inputs", side_effect=collect), \
                            redirect_stdout(stdout):
                        with self.assertRaises(ValueError):
                            publisher.publish(args, fingerprint_generation=True)
                    self.assertEqual(calls, failure_call)
                    self.assertEqual(stdout.getvalue(), "")
                    self.assertEqual(list(self.root.iterdir()), [])

    def test_scan_failure_refuses_each_collection_and_preserves_generations(self):
        import argparse
        import importlib.util
        import io
        from contextlib import redirect_stdout
        from unittest.mock import patch
        spec = importlib.util.spec_from_file_location("publisher", PUBLISHER)
        publisher = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(publisher)
        self.assertEqual(self.publish().returncode, 0)

        def published_state():
            return {str(path.relative_to(self.root)): (path.read_bytes(), path.stat().st_mode)
                    for path in self.root.rglob("*") if path.is_file()}

        prior = published_state()
        goroot = self.base / "toolchain"
        (goroot / "nested").mkdir(parents=True)
        (goroot / "nested/input").write_bytes(b"toolchain fixture")
        source = self.repo / "source/server"
        scandir = os.scandir
        check_output = publisher.subprocess.check_output
        original_inputs = publisher.inputs

        def output(command, **kwargs):
            result = check_output(command, **kwargs)
            if command[1:] == ["env", "-json"]:
                result = json.dumps({**json.loads(result), "GOROOT": str(goroot)})
            return result

        for tree, failed_path in (("source", source), ("toolchain", goroot / "nested")):
            for failure_call in (1, 2, 3):
                for code in (errno.EIO, errno.EACCES):
                    with self.subTest(tree=tree, collection=failure_call, errno=code):
                        calls = 0
                        error = OSError(code, "PRIVATE scan diagnostic", str(failed_path))
                        args = argparse.Namespace(repo=str(self.repo), go=str(self.fake),
                            sign=False, root=str(self.root), generation=f"failed-{tree}-{failure_call}-{code}",
                            fingerprint=None, reuse=False)

                        def scan(path):
                            if calls == failure_call and not isinstance(path, int) and Path(path) == failed_path:
                                raise error
                            return scandir(path)

                        def collect(*a, **kw):
                            nonlocal calls
                            calls += 1
                            return original_inputs(*a, **kw)

                        stdout = io.StringIO()
                        with patch.dict(os.environ, self.env, clear=True), \
                                patch.object(publisher, "inputs", side_effect=collect), \
                                patch.object(publisher.subprocess, "check_output", side_effect=output), \
                                patch.object(os, "scandir", side_effect=scan), \
                                patch.object(publisher, "rename_exclusive") as rename, \
                                redirect_stdout(stdout):
                            with self.assertRaises((OSError, ValueError)) as raised:
                                publisher.publish(args)
                        rename.assert_not_called()
                        if failure_call == 3:
                            self.assertIn("unable to collect final", str(raised.exception))
                            self.assertNotIn("PRIVATE", str(raised.exception))
                        else:
                            self.assertIs(raised.exception, error)
                        self.assertEqual(calls, failure_call)
                        self.assertEqual(stdout.getvalue(), "")
                        self.assertEqual(list(self.root.glob(".building-*")), [])
                        self.assertEqual(sorted(p.name for p in self.root.iterdir()), ["test-gen"])
                        self.assertEqual(published_state(), prior)

    @unittest.skipUnless(sys.platform == "darwin", "Darwin signing barrier fixture")
    def test_fresh_toolchain_checks_mutation_refusal_and_temporal_barriers(self):
        import argparse
        import importlib.util
        import io
        from contextlib import redirect_stdout
        from unittest.mock import patch
        spec = importlib.util.spec_from_file_location("barrier_publisher", PUBLISHER)
        p = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(p)
        self.assertEqual(self.publish().returncode, 0)
        prior = {str(f.relative_to(self.root)): f.read_bytes()
                 for f in self.root.rglob("*") if f.is_file()}
        goroot = self.base / "toolchain"
        goroot.mkdir()
        tool = goroot / "input"
        self.signing_tools()
        env = self.env
        original_inputs, original_output = p.inputs, p.subprocess.check_output
        original_run, original_validate = p.subprocess.run, p.validate
        for mutate_at in (None, 2, 3):
            with self.subTest(mutate_at=mutate_at):
                tool.write_bytes(b"old-toolchain")
                stamp = tool.stat()
                calls = 0
                events = []
                args = argparse.Namespace(repo=str(self.repo), go=str(self.fake), sign=True,
                    root=str(self.root), generation=f"barrier-{mutate_at}",
                    fingerprint=None, reuse=False)
                def output(command, **kwargs):
                    result = original_output(command, **kwargs)
                    if command[1:] == ["env", "-json"]:
                        result = json.dumps({**json.loads(result), "GOROOT": str(goroot)})
                    return result
                def collect(*a, **kw):
                    nonlocal calls
                    calls += 1
                    events.append(f"collect-{calls}-start")
                    if calls == mutate_at:
                        tool.write_bytes(b"new-toolchain")
                        os.utime(tool, ns=(stamp.st_atime_ns, stamp.st_mtime_ns))
                        self.assertEqual(tool.stat().st_size, stamp.st_size)
                        self.assertEqual(tool.stat().st_mtime_ns, stamp.st_mtime_ns)
                    result = original_inputs(*a, **kw)
                    events.append(f"collect-{calls}-end")
                    return result
                def run(command, **kwargs):
                    result = original_run(command, **kwargs)
                    if len(command) > 1 and command[1] == "build":
                        events.append("build")
                    elif len(command) > 1 and Path(command[1]).name == "codesign-if-available.sh":
                        events.append("sign")
                    elif command[:2] == ["codesign", "--verify"]:
                        events.append("verify")
                    return result
                def validate(path):
                    result = original_validate(path)
                    events.append("validate")
                    return result
                with patch.dict(os.environ, env, clear=True), \
                        patch.object(p, "inputs", side_effect=collect), \
                        patch.object(p.subprocess, "check_output", side_effect=output), \
                        patch.object(p.subprocess, "run", side_effect=run), \
                        patch.object(p, "validate", side_effect=validate), \
                        patch.object(p, "rename_exclusive", wraps=p.rename_exclusive) as rename, \
                        redirect_stdout(io.StringIO()):
                    if mutate_at is None:
                        p.publish(args)
                        rename.assert_called_once()
                    else:
                        with self.assertRaisesRegex(ValueError, "changed"):
                            p.publish(args)
                        rename.assert_not_called()
                self.assertEqual(calls, 2 if mutate_at == 2 else 3)
                snapshot_end = events.index("collect-2-end")
                if mutate_at == 2:
                    self.assertNotIn("build", events)
                else:
                    final_start = events.index("collect-3-start")
                    self.assertEqual(events[snapshot_end + 1:final_start],
                        ["build", "sign", "verify", "validate",
                         "build", "sign", "verify", "validate", "validate", "validate"])
                self.assertEqual(list(self.root.glob(".building-*")), [])
                self.assertEqual({str(f.relative_to(self.root)): f.read_bytes()
                    for f in (self.root / "test-gen").rglob("*") if f.is_file()}, prior)
                if mutate_at:
                    self.assertFalse((self.root / args.generation).exists())

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
        self.assertIn('added (1): ["source/server/new.go"]', result.stderr)
        self.assertNotIn("mutation", result.stderr)

    def test_final_collection_failure_does_not_forward_tool_stderr(self):
        self.env["FAIL_FINAL_ENV"] = str(self.home / "fail-env-after-build")
        result = self.publish()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("unable to collect final source/build inputs (CalledProcessError)", result.stderr)
        self.assertIn("no input diff available", result.stderr)
        self.assertNotIn("SECRET_TOOL_STDERR", result.stdout + result.stderr)
        self.assertFalse((self.root / "test-gen").exists())

    def test_final_input_diagnostics(self):
        import argparse
        import importlib.util
        from unittest.mock import patch
        spec = importlib.util.spec_from_file_location("publisher", PUBLISHER)
        publisher = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(publisher)
        args = argparse.Namespace(repo=str(self.repo), go=str(self.fake), sign=False,
                                  root=str(self.root), generation="test-gen",
                                  fingerprint=None, reuse=False)
        source = self.repo / "source/server/input.txt"
        secret = "SECRET_CONTENT_ENV_IDENTITY_DISCOVERY"

        for change in ("added", "removed", "content", "mode", "metadata",
                       "unchanged", "mtime", "failure", "missing", "tool-error", "symlink"):
            with self.subTest(change=change):
                source.write_text("original")
                source.chmod(0o644)
                calls = 0
                original_inputs = publisher.inputs

                def collect(*a, **kw):
                    nonlocal calls
                    calls += 1
                    if calls == 3 and change in {"failure", "missing", "tool-error"}:
                        kw["inventory"]["source/partial.txt"] = (b"partial", 0)
                        if change == "missing":
                            raise FileNotFoundError(secret)
                        if change == "tool-error":
                            raise subprocess.CalledProcessError(1, [secret], output=secret, stderr=secret)
                        raise ValueError(secret)
                    result = original_inputs(*a, **kw)
                    if calls == 3 and change == "metadata":
                        metadata = dict(result[1], version=secret,
                                        flags={"GOARCH": secret},
                                        signing={secret: secret})
                        return "different", metadata, result[2]
                    return result

                builds = 0
                original_run = publisher.subprocess.run

                def build(command, **kwargs):
                    nonlocal builds
                    if command[1] != "build":
                        return original_run(command, **kwargs)
                    builds += 1
                    output = Path(command[command.index("-o") + 1])
                    output.write_bytes(b"\x7fELF" + b"fixture" * 16)
                    output.chmod(0o755)
                    if builds == 2:
                        if change == "added":
                            (source.parent / "added.txt").write_text(secret)
                        elif change == "removed":
                            source.unlink()
                        elif change == "content":
                            source.write_text(secret)
                        elif change == "mode":
                            source.chmod(0o755)
                        elif change == "mtime":
                            os.utime(source, (1, 1))
                        elif change == "symlink":
                            source.unlink()
                            source.symlink_to(source.parent / "go.mod")

                with patch.dict(os.environ, self.env, clear=True), \
                        patch.object(publisher, "inputs", side_effect=collect), \
                        patch.object(publisher.subprocess, "run", side_effect=build), \
                        patch.object(publisher, "rename_exclusive") as rename:
                    if change in {"unchanged", "mtime"}:
                        publisher.publish(args)
                        rename.assert_called_once()
                    else:
                        with self.assertRaises(ValueError) as caught:
                            publisher.publish(args)
                        message = str(caught.exception)
                        self.assertNotIn(secret, message)
                        self.assertNotIn(str(self.repo), message)
                        rename.assert_not_called()
                        if change in {"failure", "missing", "tool-error", "symlink"}:
                            self.assertIn("unable to collect final", message)
                            error_type = {"missing": "FileNotFoundError",
                                          "tool-error": "CalledProcessError"}.get(change, "ValueError")
                            self.assertIn(error_type, message)
                            self.assertIn("no input diff available", message)
                            self.assertNotIn("partial.txt", message)
                        elif change == "metadata":
                            self.assertIn('metadata keys (3): ["flags", "signing", "version"]', message)
                            self.assertIn("changed (0)", message)
                        else:
                            category = change if change in {"added", "removed"} else "changed"
                            name = "added.txt" if change == "added" else "input.txt"
                            self.assertIn(f'{category} (1): ["source/server/{name}"]', message)
                self.assertEqual(calls, 3)
                self.assertEqual(builds, 2)
                self.assertFalse((self.root / "test-gen").exists())
                self.assertEqual(list(self.root.iterdir()), [])
                (source.parent / "added.txt").unlink(missing_ok=True)
                if source.is_symlink():
                    source.unlink()

    def test_inventory_matches_legacy_fingerprint_and_ignores_mtime(self):
        import argparse
        import importlib.util
        from unittest.mock import patch
        spec = importlib.util.spec_from_file_location("publisher", PUBLISHER)
        publisher = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(publisher)
        args = argparse.Namespace(repo=str(self.repo), go=str(self.fake), sign=False)
        with patch.dict(os.environ, self.env, clear=True):
            before = {}
            fingerprint, metadata, _ = publisher.inputs(args, inventory=before)
            legacy = hashlib.sha256(json.dumps(metadata, sort_keys=True).encode())
            for path in sorted((self.repo / "source").rglob("*")):
                if path.is_file():
                    legacy.update(str(path.relative_to(self.repo)).encode() + b"\0" +
                                  hashlib.sha256(path.read_bytes()).digest())
                    legacy.update(str(path.stat().st_mode & 0o111).encode() + b"\0")
            self.assertEqual(fingerprint, legacy.hexdigest())
            self.assertEqual(len(publisher.inputs(args)), 3)
            for path in (self.repo / "source").rglob("*"):
                os.utime(path, (1, 1))
            after = {}
            final, final_metadata, _ = publisher.inputs(args, inventory=after)
            self.assertEqual(fingerprint, final)
            self.assertEqual(before, after)
            self.assertEqual(publisher.input_changes(before, after, metadata, final_metadata),
                             "added (0): []; removed (0): []; changed (0): []; metadata keys (0): []")

    def test_diagnostics_bounded_sorted_escaped_and_redacted(self):
        import importlib.util
        spec = importlib.util.spec_from_file_location("publisher", PUBLISHER)
        publisher = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(publisher)
        secret = "SECRET_VALUE_NOT_A_PATH"
        paths = [f"source/{index:03d}-" + "x" * 300 for index in range(25)]
        before = {"source/removed.txt": (secret, 1), "source/changed.txt": (secret, 0)}
        after = {path: (secret, 1) for path in reversed(paths)}
        after["source/changed.txt"] = (secret, 1)
        after['source/line\n".txt'] = (secret, 0)
        message = publisher.input_changes(before, after, {"effective": {secret: secret}},
                                          {"effective": {"other": secret}})
        self.assertNotIn(secret, message)
        self.assertNotIn("\n", message)
        self.assertIn("added (26)", message)
        self.assertIn("+16 more", message)
        self.assertNotIn("source/010-", message)
        self.assertLess(message.index("source/000-"), message.index("source/009-"))
        self.assertLess(len(message), 2200)
        self.assertEqual(message, publisher.input_changes(before, dict(reversed(list(after.items()))),
                                                         {"effective": {secret: secret}},
                                                         {"effective": {"other": secret}}))
        escaped = publisher.input_changes({}, {'source/line\n".txt': (secret, 0)}, {}, {})
        self.assertIn('source/line\\n\\".txt', escaped)
        many = {f"source/{index:03d}": (secret, 0) for index in range(25)}
        self.assertIn("removed (25):", publisher.input_changes(many, {}, {}, {}))
        changed = publisher.input_changes(many, {key: (secret, 1) for key in many}, {}, {})
        self.assertIn("changed (25):", changed)
        self.assertIn("+15 more", changed)
        removed = publisher.input_changes(many, {}, {}, {})
        self.assertIn("+15 more", removed)
        self.assertNotIn("source/010", removed + changed)

    def test_signing_not_applicable_never_discovers(self):
        import importlib.util
        from unittest.mock import patch
        spec = importlib.util.spec_from_file_location("publisher", PUBLISHER)
        publisher = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(publisher)
        with patch.object(publisher.subprocess, "check_output", side_effect=AssertionError("unexpected discovery")):
            with patch.object(publisher.sys, "platform", "linux"):
                self.assertEqual(publisher.signing_identity(self.repo, True, {}),
                                 {"policy": "not-applicable"})
            with patch.object(publisher.sys, "platform", "darwin"):
                self.assertEqual(publisher.signing_identity(self.repo, False, {}),
                                 {"policy": "not-applicable"})

    def signing_tools(self):
        helper = self.repo / "source/server/scripts/codesign-if-available.sh"
        helper.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(PUBLISHER.with_name(helper.name), helper)
        tools = self.base / "tools"
        tools.mkdir()
        self.env["SIGN_LOG"] = str(self.home / "sign-log")
        self.env["CERT"] = "A" * 40
        self.env["IDENTITY_NAME"] = "Developer ID Application: Fixture"
        for name, code in {
            "security": '''import os
print('  1) %s "%s"' % (os.environ["CERT"], os.environ["IDENTITY_NAME"]) if os.environ["CERT"] else "0 valid identities found")''',
            "codesign": 'import os,sys,json\nwith open(os.environ["SIGN_LOG"], "a") as f: f.write(json.dumps(sys.argv[1:])+"\\n")\nsys.exit(7 if os.environ.get("FAIL_PHASE") in sys.argv[1:] else 0)',
        }.items():
            tool = tools / name
            tool.write_text(f"#!{sys.executable}\n" + code + "\n")
            tool.chmod(0o755)
        self.env["PATH"] = str(tools) + os.pathsep + self.env["PATH"]

    def signing_calls(self):
        log = Path(self.env["SIGN_LOG"])
        return [json.loads(line) for line in log.read_text().splitlines()] if log.exists() else []

    def test_existing_build_helper_policy(self):
        self.signing_tools()
        helper = self.repo / "source/server/scripts/codesign-if-available.sh"
        output = self.base / "fixture binary"
        output.write_bytes(b"fixture never execute")
        def sign():
            return subprocess.run(["/bin/bash", str(helper), str(output), "cercano"],
                                  env=self.env, capture_output=True, text=True)
        self.assertEqual(sign().returncode, 0)
        self.assertEqual(self.signing_calls()[-1][2], self.env["IDENTITY_NAME"])
        self.env["CERCANO_CODESIGN_ID"] = "Explicit Override"
        self.assertEqual(sign().returncode, 0)
        self.assertEqual(self.signing_calls()[-1][2], "Explicit Override")
        before = self.signing_calls()
        self.env["CERCANO_CODESIGN_ID"] = "none"
        self.assertEqual(sign().returncode, 0)
        del self.env["CERCANO_CODESIGN_ID"]
        self.env["CERT"] = ""
        self.assertEqual(sign().returncode, 0)
        self.assertEqual(self.signing_calls(), before)

    @unittest.skipUnless(sys.platform == "darwin", "Darwin signing")
    def test_signing_stable_identity_and_verification(self):
        self.signing_tools()
        self.env["CERCANO_CODESIGN_ID"] = self.env["IDENTITY_NAME"]
        result = self.publish(extra=["--sign"])
        self.assertEqual(result.returncode, 0, result.stderr)
        calls = self.signing_calls()
        self.assertEqual(len(calls), 4)
        for i, name in enumerate(("cercano", "cercano-cli")):
            self.assertEqual(calls[i*2][:-1], ["--force", "--sign", "A"*40, "--identifier", name])
            self.assertEqual(calls[i*2+1], ["--verify", "--strict", calls[i*2][-1]])
        manifest = (self.root / "test-gen/manifest.json").read_text()
        self.assertNotIn(self.env["IDENTITY_NAME"], manifest)
        self.assertNotIn("A"*40, manifest)
        # Matching reuse does no signing; certificate renewal with the SAME
        # display name must not silently reuse the prior signed generation.
        self.assertEqual(self.publish(extra=["--sign", "--reuse"]).returncode, 0)
        self.assertEqual(calls, self.signing_calls())
        self.env["CERT"] = "B" * 40
        self.assertNotEqual(self.publish(extra=["--sign", "--reuse"]).returncode, 0)

    @unittest.skipUnless(sys.platform == "darwin", "Darwin signing")
    def test_signing_discovery_and_failures(self):
        self.signing_tools()
        for phase in ("--sign", "--verify"):
            self.env["FAIL_PHASE"] = phase
            result = self.publish(extra=["--sign"])
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse((self.root / "test-gen").exists())
        del self.env["FAIL_PHASE"]
        result = self.publish(extra=["--sign"])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.signing_calls()[0][2], "A"*40)
        helper = self.repo / "source/server/scripts/codesign-if-available.sh"
        helper.write_text(helper.read_text() + "\n# changed signing policy\n")
        self.assertNotEqual(self.publish(extra=["--sign", "--reuse"]).returncode, 0)

    @unittest.skipUnless(sys.platform == "darwin", "Darwin signing")
    def test_signing_none_and_no_identity_are_noops(self):
        self.signing_tools()
        self.env["CERCANO_CODESIGN_ID"] = "none"
        # none must bypass discovery entirely.
        (self.base / "tools/security").write_text("#!/bin/sh\nexit 99\n")
        result = self.publish(extra=["--sign"])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.signing_calls(), [])
        self.root = self.base / "no identity generations"
        self.env["FAKE_ROOT"] = str(self.root)
        del self.env["CERCANO_CODESIGN_ID"]
        (self.base / "tools/security").write_text("#!/bin/sh\necho '0 valid identities found'\n")
        result = self.publish(extra=["--sign"])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.signing_calls(), [])
        manifest = json.loads((self.root / "test-gen/manifest.json").read_text())
        self.assertFalse(manifest["build_inputs"]["signing"]["enabled"])

    @unittest.skipUnless(sys.platform == "darwin", "Darwin signing")
    def test_signing_hash_override_bypasses_discovery(self):
        self.signing_tools()
        self.env["CERCANO_CODESIGN_ID"] = "B" * 40
        (self.base / "tools/security").write_text("#!/bin/sh\nexit 99\n")
        result = self.publish(extra=["--sign"])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.signing_calls()[0][2], "B"*40)
        self.env["CERCANO_CODESIGN_ID"] = "none"
        self.assertNotEqual(self.publish(extra=["--sign", "--reuse"]).returncode, 0)

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
        self.signing_tools()
        self.env["CERCANO_CODESIGN_ID"] = "B" * 40
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


class FingerprintTraversalTests(unittest.TestCase):
    def setUp(self):
        import importlib.util
        spec = importlib.util.spec_from_file_location("publisher", PUBLISHER)
        self.publisher = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.publisher)
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)

    @staticmethod
    def legacy_tree(root):
        # Frozen pre-optimization oracle: including absolute symlink text and
        # Path (component-wise), rather than string, global ordering.
        root = Path(root)
        digest = hashlib.sha256()
        paths = sorted(root.rglob("*")) if root.is_dir() else [root]
        for path in paths:
            if path.is_file():
                digest.update(str(path.relative_to(root) if path != root else path).encode())
                digest.update(hashlib.sha256(path.read_bytes()).digest())
            elif path.is_symlink():
                digest.update(str(path).encode() + os.readlink(path).encode())
        return digest.hexdigest()

    def assert_tree_equal(self, root):
        self.assertEqual(self.publisher.tree_identity(root), self.legacy_tree(root))

    def test_empty_unicode_component_order_and_large_file(self):
        from unittest.mock import patch
        self.assert_tree_equal(self.root)
        (self.root / "empty directory").mkdir()
        (self.root / "empty").touch()
        (self.root / "a").mkdir()
        for name in ("a/z", "a.txt", "é", "e\u0301", "Z", ".hidden"):
            (self.root / name).write_bytes(name.encode())
        (self.root / "large").write_bytes(b"0123456789" * 300000)
        self.assertEqual(sorted(self.publisher.descendant_paths(self.root)),
                         sorted(self.root.rglob("*")))
        expected = self.legacy_tree(self.root)
        with patch.object(Path, "read_bytes", side_effect=AssertionError("whole-file read")):
            self.assertEqual(self.publisher.tree_identity(self.root), expected)
        self.assert_tree_equal(self.root / "empty")
        self.assert_tree_equal(self.root / "missing")

    def test_symlink_branches_root_identity_and_nonregular(self):
        (self.root / "file").write_bytes(b"original")
        (self.root / "dir").mkdir()
        (self.root / "dir/nested").write_bytes(b"nested")
        (self.root / "file-link").symlink_to("file")
        (self.root / "dir-link").symlink_to("dir", target_is_directory=True)
        (self.root / "broken").symlink_to("absent")
        (self.root / "loop").symlink_to("loop")
        os.mkfifo(self.root / "fifo")
        self.assert_tree_equal(self.root)
        for name in ("file", "file-link", "dir-link", "broken", "loop", "fifo"):
            self.assert_tree_equal(self.root / name)
        before = self.publisher.tree_identity(self.root / "file-link")
        (self.root / "file").write_bytes(b"changed")
        self.assertNotEqual(before, self.publisher.tree_identity(self.root / "file-link"))
        (self.root / "other-link").symlink_to("file")
        self.assertNotEqual(self.publisher.tree_identity(self.root / "file-link"),
                            self.publisher.tree_identity(self.root / "other-link"))

    def test_read_errors_propagate_and_reads_are_bounded(self):
        from unittest.mock import MagicMock, patch
        path = self.root / "file"
        path.touch()
        stream = MagicMock()
        stream.__enter__.return_value = stream
        stream.read.side_effect = [b"bytes", OSError("read failed")]
        with patch.object(Path, "open", return_value=stream):
            with self.assertRaisesRegex(OSError, "read failed"):
                self.publisher.tree_identity(self.root)
        self.assertEqual([call.args for call in stream.read.call_args_list],
                         [(1024 * 1024,), (1024 * 1024,)])

    def test_directory_scan_errors_propagate(self):
        from unittest.mock import patch
        nested = self.root / "directory"
        nested.mkdir()
        (nested / "input").write_bytes(b"must not disappear")
        scandir = os.scandir
        for failed_path in (self.root, nested):
            for code in (errno.EIO, errno.EACCES):
                with self.subTest(path=failed_path, errno=code):
                    error = OSError(code, "scan failed", str(failed_path))

                    def scan(path):
                        if Path(path) == failed_path:
                            raise error
                        return scandir(path)

                    with patch.object(os, "scandir", side_effect=scan):
                        with self.assertRaises(OSError) as raised:
                            self.publisher.tree_identity(self.root)
                    self.assertIs(raised.exception, error)

    def test_parallel_files_fifo_queue_bound_and_join(self):
        from concurrent.futures import ThreadPoolExecutor
        from unittest.mock import patch
        import threading
        p = self.publisher
        for number in range(80):
            (self.root / f"{number:03}").write_bytes(str(number).encode())
        expected = self.legacy_tree(self.root)
        lock = threading.Lock()
        release = threading.Event()
        three_readers = threading.Event()
        six_pending = threading.Event()
        active = peak = pending = peak_pending = 0
        original = p.file_digest
        result = []
        errors = []

        class ConsumedFuture:
            def __init__(self, future):
                self.future = future
            def result(self):
                nonlocal pending
                try:
                    return self.future.result()
                finally:
                    with lock:
                        pending -= 1

        class Pool(ThreadPoolExecutor):
            def submit(self, *args, **kwargs):
                nonlocal pending, peak_pending
                with lock:
                    pending += 1
                    peak_pending = max(peak_pending, pending)
                    if pending == p.HASH_PENDING_BATCHES:
                        six_pending.set()
                return ConsumedFuture(super().submit(*args, **kwargs))

        def read(path):
            nonlocal active, peak
            with lock:
                active += 1
                peak = max(peak, active)
                if active == 3:
                    three_readers.set()
            try:
                if not release.wait(10):
                    raise AssertionError("reader release timed out")
                return original(path)
            finally:
                with lock:
                    active -= 1

        def collect():
            try:
                result.append(p.tree_identity(self.root))
            except BaseException as error:
                errors.append(error)

        with patch.object(p, "ThreadPoolExecutor", Pool), \
                patch.object(p, "HASH_BATCH_SIZE", 2), \
                patch.object(p, "file_digest", side_effect=read):
            thread = threading.Thread(target=collect)
            thread.start()
            try:
                self.assertTrue(three_readers.wait(10))
                self.assertTrue(six_pending.wait(10))
                self.assertTrue(thread.is_alive())
                self.assertEqual(peak_pending, 6)
                self.assertEqual(peak, 3)
            finally:
                release.set()
                thread.join(10)
            self.assertFalse(thread.is_alive())
        self.assertEqual(errors, [])
        self.assertEqual(result, [expected])
        self.assertEqual(active, 0)
        self.assertEqual(pending, 0)
        self.assertLessEqual(peak_pending, 6)

    def test_error_selection_is_path_order_and_workers_join_before_raise(self):
        from unittest.mock import patch
        import threading
        p = self.publisher
        for name in ("a", "b", "c", "d", "e", "f"):
            (self.root / name).touch()
        later_failed = threading.Event()
        last_finished = threading.Event()
        first = OSError(errno.EIO, "first logical path")
        second = OSError(errno.EACCES, "later logical path")
        def read(path):
            if path.name == "a":
                if not later_failed.wait(10):
                    raise AssertionError("later failure timed out")
                raise first
            if path.name == "b":
                later_failed.set()
                raise second
            last_finished.set()
            return hashlib.sha256(b"").digest()
        with patch.object(p, "HASH_BATCH_SIZE", 1), \
                patch.object(p, "file_digest", side_effect=read):
            with self.assertRaises(OSError) as raised:
                p.tree_identity(self.root)
        self.assertIs(raised.exception, first)
        self.assertTrue(last_finished.is_set())
        self.assertFalse(any(t.name.startswith("ThreadPoolExecutor")
                             for t in threading.enumerate()))

    def test_batches_records_and_fresh_aliases_have_bounded_content_memory(self):
        from unittest.mock import patch
        p = self.publisher
        large = self.root / "large"
        large.write_bytes(b"x" * (p.HASH_BATCH_BYTES + 1))
        (self.root / "alias").symlink_to("large")
        for number in range(150):
            (self.root / f"tiny-{number:03}").write_bytes(b"x")
        batches = p.tree_batches(self.root)
        self.assertTrue(all(len(batch) <= p.HASH_BATCH_SIZE for batch in batches))
        self.assertIn([large], batches)
        self.assertIn([self.root / "alias"], batches)
        for batch in batches:
            records = p.tree_batch(self.root, batch)
            self.assertEqual(len(records), len(batch))
            self.assertTrue(all(len(record) < 128 for record in records))
        original = p.file_digest
        with patch.object(p, "file_digest", wraps=original) as read:
            before = p.tree_identity(self.root)
            stamp = large.stat()
            large.write_bytes(b"y" * stamp.st_size)
            os.utime(large, ns=(stamp.st_atime_ns, stamp.st_mtime_ns))
            after = p.tree_identity(self.root)
        self.assertNotEqual(before, after)
        self.assertEqual(after, self.legacy_tree(self.root))
        self.assertEqual(sum(call.args[0] == large for call in read.call_args_list), 2)
        self.assertEqual(sum(call.args[0] == self.root / "alias"
                             for call in read.call_args_list), 2)

    def test_all_toolchain_roots_share_one_file_worker_budget(self):
        from concurrent.futures import ThreadPoolExecutor
        from unittest.mock import patch
        import threading
        p = self.publisher
        for name in ("go", "sdk", "native"):
            directory = self.root / name
            directory.mkdir()
            for number in range(8):
                (directory / str(number)).write_bytes(name.encode())
        for name in ("cc", "c++"):
            (self.root / name).write_bytes(name.encode())
        original = p.file_digest
        lock = threading.Lock()
        release, three = threading.Event(), threading.Event()
        active = peak = 0
        pools, observations, result, errors = [], [], [], []
        class Pool(ThreadPoolExecutor):
            def __init__(self, max_workers):
                pools.append(max_workers)
                super().__init__(max_workers=max_workers)
        def read(path):
            nonlocal active, peak
            with lock:
                active += 1
                peak = max(active, peak)
                observations.append(path)
                if active == 3:
                    three.set()
            try:
                if not release.wait(10):
                    raise AssertionError("reader release timed out")
                return original(path)
            finally:
                with lock:
                    active -= 1
        def output(command, **kwargs):
            if command[1:] == ["env", "-json"]:
                return json.dumps({"GOROOT": str(self.root / "go"), "CGO_ENABLED": "1",
                                   "CC": "cc", "CXX": "c++"})
            if command[0] == "/usr/bin/xcrun":
                return str(self.root / "sdk") if command[1] == "--show-sdk-path" else str(self.root / "native/bin/clang")
            return "compiler version"
        def collect():
            try:
                result.append(p.effective_identity("go", {}, self.root))
            except BaseException as error:
                errors.append(error)
        with patch.object(p.sys, "platform", "darwin"), \
                patch.object(p.subprocess, "check_output", side_effect=output), \
                patch.object(p.shutil, "which", side_effect=lambda tool, **kw: str(self.root / tool)), \
                patch.object(p, "ThreadPoolExecutor", Pool), \
                patch.object(p, "HASH_BATCH_SIZE", 1), \
                patch.object(p, "file_digest", side_effect=read):
            thread = threading.Thread(target=collect)
            thread.start()
            try:
                self.assertTrue(three.wait(10))
                self.assertEqual(active, 3)
            finally:
                release.set()
                thread.join(10)
            self.assertFalse(thread.is_alive())
        self.assertFalse(errors, errors)
        self.assertEqual(pools, [3])
        self.assertEqual((peak, active), (3, 0))
        self.assertEqual(len(observations), 26)
        self.assertEqual(result[0]["goroot_sha256"], self.legacy_tree(self.root / "go"))
        self.assertEqual(result[0]["sdk"]["sha256"], self.legacy_tree(self.root / "sdk"))
        self.assertEqual(result[0]["native_toolchain"], self.legacy_tree(self.root / "native"))

    def test_bounded_identity_order_metadata_and_failure(self):
        from unittest.mock import patch
        import threading
        import time
        p = self.publisher
        active = peak = 0
        lock = threading.Lock()
        calls = []
        def scan(root):
            nonlocal active, peak
            with lock:
                calls.append(str(root))
                active += 1
                peak = max(peak, active)
            time.sleep(0.01)
            with lock:
                active -= 1
            return []
        def tree(root, pool, paths):
            paths.result()
            return "hash:" + str(root)
        def output(command, **kwargs):
            if command[1:] == ["env", "-json"]:
                return json.dumps({"GOROOT": "go-root", "CGO_ENABLED": "1",
                                   "CC": "cc", "CXX": "c++"})
            if command[0] == "/usr/bin/xcrun":
                return str(self.root) if command[1] == "--show-sdk-path" else str(self.root / "bin/clang")
            return "compiler version"
        with patch.object(p.sys, "platform", "darwin"), \
                patch.object(p.subprocess, "check_output", side_effect=output), \
                patch.object(p.shutil, "which", side_effect=lambda tool, **kw: str(self.root / tool)), \
                patch.object(p, "tree_batches", side_effect=scan), \
                patch.object(p, "tree_identity", side_effect=tree):
            result = p.effective_identity("go", {}, self.root)
            self.assertEqual(peak, 3)
            self.assertCountEqual(calls, ["go-root", str(self.root / "cc"),
                                        str(self.root / "c++"), str(self.root), str(self.root.resolve())])
            self.assertEqual(result["goroot_sha256"], "hash:go-root")
            self.assertEqual(result["CC"]["sha256"], "hash:" + str(self.root / "cc"))
            self.assertEqual(result["sdk"], {"path": str(self.root), "sha256": "hash:" + str(self.root)})
            with patch.object(p, "tree_identity", side_effect=OSError("failed tree")):
                with self.assertRaisesRegex(OSError, "failed tree"):
                    p.effective_identity("go", {}, self.root)


class SourceTraversalTests(unittest.TestCase):
    setUp = PublicationTests.setUp
    tearDown = PublicationTests.tearDown
    git = PublicationTests.git

    def test_source_legacy_digest_inventory_snapshot_and_mode(self):
        import argparse
        import importlib.util
        from unittest.mock import patch
        spec = importlib.util.spec_from_file_location("publisher", PUBLISHER)
        p = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(p)
        source = self.repo / "source"
        for name in ("empty", "é", "a.txt", "a/z", ".git/excluded"):
            path = source / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(name.encode() if name != "empty" else b"")
        args = argparse.Namespace(repo=str(self.repo), go=str(self.fake), sign=False)
        snapshot = self.base / "snapshot"
        old_inventory, new_inventory = {}, {}
        with patch.dict(os.environ, self.env, clear=True):
            with patch.object(p, "descendant_paths", side_effect=lambda root: root.rglob("*")):
                before = p.inputs(args, inventory=old_inventory)
            after = p.inputs(args, snapshot, inventory=new_inventory)
            self.assertEqual(before, after)
            self.assertEqual(old_inventory, new_inventory)
            for name, (identity, mode) in new_inventory.items():
                copied = snapshot / name
                self.assertEqual(hashlib.sha256(copied.read_bytes()).digest(), identity)
                self.assertEqual(copied.stat().st_mode & 0o111, mode)
            (source / "empty").chmod(0o755)
            changed = {}
            self.assertNotEqual(after[0], p.inputs(args, inventory=changed)[0])
            self.assertEqual(changed["source/empty"][1], 0o111)
            for name in ("symlink", "fifo"):
                path = source / name
                if name == "symlink":
                    path.symlink_to("empty")
                else:
                    os.mkfifo(path)
                try:
                    with self.assertRaisesRegex(ValueError, "unsupported nonregular source"):
                        p.inputs(args)
                finally:
                    path.unlink()


if __name__ == "__main__":
    unittest.main(verbosity=2)
