"""Temporary fake commands only: never invoke any application or credential API."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

from acceptance_env import build_acceptance_env
from acceptance_preparation import prepare_real_cloud_launch


class AcceptancePreparationTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.base = Path(self.temp.name).resolve()
        self.home = self.base / "home"
        self.home.mkdir(mode=0o700)
        self.root = self.base / "private"
        self.root.mkdir(mode=0o700)
        self.ledger = self.root / "usage.json"
        # Intentionally opaque: preparation must not interpret or rewrite it.
        self.ledger.write_bytes(b"existing aggregate ledger: halted; do not reset\n")
        self.ledger.chmod(0o600)
        self.script = self.base / "fake_command.py"
        self.script.write_text('''import json, os, subprocess, sys
result = {"role": sys.argv[1], "env": dict(os.environ)}
if sys.argv[1] == "host":
    # No env= override: demonstrate ordinary child inheritance from the host.
    child = subprocess.run([sys.executable, "-B", __file__, "worker"],
                           check=True, capture_output=True, text=True)
    result["worker"] = json.loads(child.stdout)
print(json.dumps(result))
''')
        self.args = dict(
            home=str(self.home), private_root=str(self.root),
            credential_policy="disabled", ledger_path=str(self.ledger),
            **{role + "_argv": [sys.executable, "-B", str(self.script), role]
               for role in ("launcher", "host", "cli")},
        )

    def test_one_allowlist_propagates_to_fake_commands_and_worker(self):
        original = self.ledger.stat()
        contents = self.ledger.read_bytes()
        hostile = {
            "HOME": "/hostile/home", "PATH": "/hostile/bin",
            "OPENAI_API_KEY": "must-not-propagate", "HTTP_PROXY": "hostile",
            "DYLD_INSERT_LIBRARIES": "/hostile/lib", "LD_PRELOAD": "/hostile/lib",
            "PYTHONPATH": "/hostile/python", "GODEBUG": "hostile",
            "CERCANO_ROUTING_LOG": "/hostile/log",
            "CERCANO_ACCEPTANCE_LEDGER": "/hostile/new-ledger",
            "CERCANO_DISABLE_CREDENTIALS": "0", "CERCANO_SEARCH_TRACE": "1",
        }
        with patch.dict(os.environ, hostile, clear=True):
            plan = prepare_real_cloud_launch(**self.args)
            expected = build_acceptance_env(**{k: self.args[k] for k in
                ("home", "private_root", "credential_policy", "ledger_path")})
            self.assertIs(plan.launcher.env, plan.host.env)
            self.assertIs(plan.host.env, plan.cli.env)
            self.assertEqual(dict(plan.host.env), expected)
            with self.assertRaises(TypeError):
                plan.host.env["OPENAI_API_KEY"] = "override"
            for role in ("launcher", "host", "cli"):
                command = getattr(plan, role)
                result = subprocess.run(command.argv, env=command.env, check=True,
                                        capture_output=True, text=True)
                observed = json.loads(result.stdout)
                self.assertEqual(observed["role"], role)
                # Runtime startup may add locale variables (Python and macOS
                # CoreFoundation); neither comes from the supplied allowlist.
                for record in ([observed, observed["worker"]] if role == "host" else [observed]):
                    env = record["env"]
                    extra = set(env) - set(expected)
                    self.assertLessEqual(extra, {"LC_CTYPE", "__CF_USER_TEXT_ENCODING"})
                    if "LC_CTYPE" in extra:
                        self.assertEqual(env.pop("LC_CTYPE"), "C.UTF-8")
                    if "__CF_USER_TEXT_ENCODING" in extra:
                        self.assertEqual(sys.platform, "darwin")
                        self.assertRegex(env.pop("__CF_USER_TEXT_ENCODING"),
                                         rf"^(?:{os.getuid()}|0x{os.getuid():X}):0x[0-9A-Fa-f]+:0x[0-9A-Fa-f]+$")
                    self.assertEqual(env, expected)
            self.assertEqual(dict(os.environ), hostile)
        after = self.ledger.stat()
        self.assertEqual((after.st_dev, after.st_ino, after.st_size, after.st_mtime_ns),
                         (original.st_dev, original.st_ino, original.st_size, original.st_mtime_ns))
        self.assertEqual(self.ledger.read_bytes(), contents)
        self.assertEqual(list(self.root.iterdir()), [self.ledger])
        self.assertEqual(list(self.home.iterdir()), [])

    def test_preparation_has_no_execution_or_content_access(self):
        with patch("subprocess.run", side_effect=AssertionError("no execution")), \
             patch("subprocess.Popen", side_effect=AssertionError("no execution")), \
             patch("builtins.open", side_effect=AssertionError("no content access")), \
             patch.object(Path, "open", side_effect=AssertionError("no content access")):
            prepare_real_cloud_launch(**self.args)

    def test_explicit_policy_tools_and_required_arguments(self):
        for key in self.args:
            args = dict(self.args)
            del args[key]
            with self.subTest(missing=key), self.assertRaises(TypeError):
                prepare_real_cloud_launch(**args)
        for policy in (None, "", "enabled"):
            with self.subTest(policy=policy), self.assertRaises(ValueError):
                prepare_real_cloud_launch(**dict(self.args, credential_policy=policy))
        tools = {"PATH": "/opt/explicit/bin:/usr/bin", "GOROOT": "/opt/explicit/go"}
        plan = prepare_real_cloud_launch(**dict(self.args, credential_policy="authorized-read-only"),
                                         tool_paths=tools)
        self.assertEqual(plan.host.env["CERCANO_DISABLE_CREDENTIALS"], "0")
        for key, value in tools.items():
            self.assertEqual(plan.cli.env[key], value)
        with self.assertRaises(ValueError):
            prepare_real_cloud_launch(**self.args, tool_paths={"OPENAI_API_KEY": "secret"})

    def test_refuses_missing_or_unsafe_ledger_without_replacement(self):
        self.ledger.unlink()
        with self.assertRaises(FileNotFoundError):
            prepare_real_cloud_launch(**self.args)
        self.assertFalse(self.ledger.exists())
        self.ledger.symlink_to(self.script)
        with self.assertRaises(ValueError):
            prepare_real_cloud_launch(**self.args)
        self.ledger.unlink()
        self.ledger.write_bytes(b"original")
        self.ledger.chmod(0o644)
        with self.assertRaises(ValueError):
            prepare_real_cloud_launch(**self.args)
        self.ledger.chmod(0o600)
        os.link(self.ledger, self.root / "alias")
        with self.assertRaises(ValueError):
            prepare_real_cloud_launch(**self.args)
        self.assertEqual(self.ledger.read_bytes(), b"original")

    def test_refuses_ambient_command_lookup_or_shell_strings(self):
        for argv in ("/bin/echo shell", [], ["cercano"], ["/bin/../bin/echo"],
                     ["/bin/echo", "bad\0arg"]):
            with self.subTest(argv=argv), self.assertRaises(ValueError):
                prepare_real_cloud_launch(**dict(self.args, host_argv=argv))


if __name__ == "__main__":
    unittest.main()
