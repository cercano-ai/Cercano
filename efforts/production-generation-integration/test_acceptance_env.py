"""Hermetic environment-output tests: no application or subprocess execution."""
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from acceptance_env import build_acceptance_env


class AcceptanceEnvironmentTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.base = Path(self.temp.name).resolve()
        self.home = self.base / "ordinary-home"
        self.home.mkdir(mode=0o700)
        self.root = self.base / "private"
        self.root.mkdir(mode=0o700)
        self.args = dict(home=str(self.home), private_root=str(self.root),
                         credential_policy="disabled", ledger_path=str(self.root / "usage.json"))

    def test_exact_defaults_and_hostile_ambient_is_ignored(self):
        hostile = {"HOME": "/do-not-touch", "PATH": "/hostile", "OPENAI_API_KEY": "secret",
                   "CERCANO_DISABLE_CREDENTIALS": "0", "CERCANO_SEARCH_TRACE": "1",
                   "CERCANO_TUI_PERF": "1", "CERCANO_ROUTING_LOG": "/outside",
                   "CERCANO_UI_CONFIG": "/outside/ui.yaml", "GODEBUG": "hostile",
                   "CERCANO_ACCEPTANCE_LEDGER": "/outside/reset.json", "HTTP_PROXY": "hostile"}
        expected = {
            "HOME": str(self.home), "PATH": os.defpath, "LANG": "C", "TERM": "xterm-256color",
            "CERCANO_DISABLE_CREDENTIALS": "1", "CERCANO_TUI_PERF": "0",
            "CERCANO_SEARCH_TRACE": "0", "GOPROXY": "off", "GOSUMDB": "off", "GOTOOLCHAIN": "local",
        }
        paths = {
            "XDG_CONFIG_HOME": "config", "XDG_DATA_HOME": "data", "XDG_CACHE_HOME": "cache",
            "XDG_STATE_HOME": "state", "XDG_RUNTIME_DIR": "runtime", "TMPDIR": "tmp",
            "CERCANO_UI_CONFIG": "config/cercano/ui.yaml", "CERCANO_THEMES_DIR": "config/cercano/themes",
            "CERCANO_TUI_PERF_LOG": "logs/tui-perf.log", "CERCANO_COLORPROBE_LOG": "logs/colorprobe.log",
            "CERCANO_SEARCH_TRACE_LOG": "logs/search-trace.log", "CERCANO_SEARCH_SCRIPT_DIR": "search-scripts",
            "CERCANO_ANOMALY_LOG": "logs/stream-anomalies.jsonl", "CERCANO_ACCEPTANCE_LEDGER": "usage.json",
            "GOCACHE": "cache/go-build", "GOMODCACHE": "cache/go-mod", "GOPATH": "data/go",
        }
        expected.update({k: str(self.root / v) for k, v in paths.items()})
        with patch.dict(os.environ, hostile, clear=True):
            before = dict(os.environ)
            self.assertEqual(build_acceptance_env(**self.args), expected)
            self.assertEqual(dict(os.environ), before)
        self.assertEqual(list(self.root.iterdir()), [])
        self.assertEqual(list(self.home.iterdir()), [])

    def test_required_policy_and_ledger(self):
        for key in ("credential_policy", "ledger_path"):
            args = dict(self.args)
            del args[key]
            with self.assertRaises(TypeError):
                build_acceptance_env(**args)
        for policy in (None, "", "enabled", "read-only", True):
            with self.subTest(policy=policy), self.assertRaises(ValueError):
                build_acceptance_env(**dict(self.args, credential_policy=policy))
        env = build_acceptance_env(**dict(self.args, credential_policy="authorized-read-only"))
        self.assertEqual(env["HOME"], str(self.home))
        self.assertEqual(env["CERCANO_DISABLE_CREDENTIALS"], "0")

    def test_explicit_tools_only(self):
        tools = {"PATH": "/opt/tools/bin:/usr/bin:/bin", "GOROOT": "/opt/go"}
        env = build_acceptance_env(**self.args, tool_paths=tools)
        self.assertEqual({k: env[k] for k in tools}, tools)
        for tools in ({"OPENAI_API_KEY": "secret"}, {"GOCACHE": "/shared"}, {"PATH": ""},
                      {"PATH": "/bin:"}, {"PATH": "relative:/bin"}, {"GOROOT": "~/go"}):
            with self.subTest(tools=tools), self.assertRaises(ValueError):
                build_acceptance_env(**self.args, tool_paths=tools)

    def test_invalid_paths(self):
        cases = [("home", "relative"), ("home", "/"), ("home", "/fake/../home"),
                 ("home", "/fake\n/home"), ("private_root", str(self.home)),
                 ("private_root", str(self.home / "artifacts")),
                 ("ledger_path", str(self.home / "usage.json")),
                 ("ledger_path", str(self.root)), ("ledger_path", ""),
                 ("ledger_path", str(self.root / "logs")),
                 ("ledger_path", str(self.root / "config/cercano/ui.yaml")),
                 ("ledger_path", str(self.root / "config/cercano/ui.yaml/child")),
                 ("ledger_path", str(self.root) + "/../escape")]
        for key, value in cases:
            with self.subTest(key=key, value=value), self.assertRaises(ValueError):
                build_acceptance_env(**dict(self.args, **{key: value}))
        self.root.chmod(0o755)
        with self.assertRaises(ValueError):
            build_acceptance_env(**self.args)

    def test_symlinks_wrong_types_permissions_and_hardlinks(self):
        logs = self.root / "logs"
        logs.symlink_to(self.home, target_is_directory=True)
        with self.assertRaises(ValueError):
            build_acceptance_env(**self.args)
        logs.unlink()
        logs.write_text("not a directory")
        with self.assertRaises(ValueError):
            build_acceptance_env(**self.args)
        logs.unlink()
        logs.mkdir(mode=0o755)
        with self.assertRaises(ValueError):
            build_acceptance_env(**self.args)
        logs.chmod(0o700)
        ledger = self.root / "usage.json"
        ledger.write_bytes(b"existing ledger, never parse or reset")
        ledger.chmod(0o600)
        before = ledger.stat()
        build_acceptance_env(**self.args)
        self.assertEqual(ledger.read_bytes(), b"existing ledger, never parse or reset")
        self.assertEqual(ledger.stat().st_mtime_ns, before.st_mtime_ns)
        os.link(ledger, self.root / "ledger-alias")
        with self.assertRaises(ValueError):
            build_acceptance_env(**self.args)

    def test_no_home_inspection_or_file_content_access(self):
        # A nonexistent supplied HOME is accepted lexically; only the owned root
        # is inspected. Instrument both stat entry points to enforce that bound.
        original_stat, original_lstat = Path.stat, Path.lstat
        def checked(method):
            def call(path, *args, **kwargs):
                self.assertFalse(path == self.home or self.home in path.parents)
                return method(path, *args, **kwargs)
            return call
        with patch.object(Path, "stat", checked(original_stat)), \
             patch.object(Path, "lstat", checked(original_lstat)), \
             patch("builtins.open", side_effect=AssertionError("no content reads")):
            build_acceptance_env(**self.args)
        args = dict(self.args, home=str(self.base / "nonexistent-home"))
        build_acceptance_env(**args)

    def test_canonical_root_required(self):
        alias = self.base / "alias"
        alias.symlink_to(self.root, target_is_directory=True)
        with self.assertRaises(ValueError):
            build_acceptance_env(**dict(self.args, private_root=str(alias)))


if __name__ == "__main__":
    unittest.main()
