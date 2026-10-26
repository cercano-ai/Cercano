#!/usr/bin/env python3
"""Tests for the Claude Code cloud-token hook. Each test runs against a
throwaway HOME; nothing outside it is read or written."""

import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

HOOK = Path(__file__).with_name("report_cloud_tokens.py")

SCHEMA = """CREATE TABLE cloud_usage (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME NOT NULL,
    cloud_input_tokens INTEGER NOT NULL DEFAULT 0,
    cloud_output_tokens INTEGER NOT NULL DEFAULT 0,
    cloud_provider TEXT NOT NULL DEFAULT '',
    cloud_model TEXT NOT NULL DEFAULT ''
)"""


class HookTest(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.root = Path(self._tmp.name)
        self.home = self.root / "home"
        self.home.mkdir()
        self.transcript = self.root / "transcript.jsonl"
        entry = {"type": "assistant", "message": {"usage": {"input_tokens": 7, "output_tokens": 3}}}
        self.transcript.write_text(json.dumps(entry) + "\n")

    def tearDown(self):
        self._tmp.cleanup()

    def profile_dir(self, name):
        return self.home / ".config" / "cercano" / "profiles" / name

    def make_db(self, name):
        directory = self.profile_dir(name)
        directory.mkdir(parents=True)
        db = directory / "telemetry.db"
        conn = sqlite3.connect(db)
        conn.execute(SCHEMA)
        conn.commit()
        conn.close()
        return db

    def run_hook(self, profile):
        env = {k: v for k, v in os.environ.items() if k != "CERCANO_PROFILE"}
        env["HOME"] = str(self.home)
        if profile is not None:
            env["CERCANO_PROFILE"] = profile
        payload = json.dumps({"transcript_path": str(self.transcript), "session_id": "s1"})
        return subprocess.run([sys.executable, str(HOOK)], input=payload, env=env,
                              capture_output=True, text=True, timeout=30)

    def rows(self, db):
        conn = sqlite3.connect(db)
        try:
            return conn.execute("SELECT cloud_input_tokens, cloud_output_tokens FROM cloud_usage").fetchall()
        finally:
            conn.close()

    def all_files(self):
        return sorted(str(p.relative_to(self.home)) for p in self.home.rglob("*"))

    def test_unset_profile_uses_default(self):
        db = self.make_db("default")
        result = self.run_hook(None)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.rows(db), [(7, 3)])
        self.assertTrue((self.profile_dir("default") / "hook_state.json").exists())

    def test_named_profile_uses_its_directory(self):
        work = self.make_db("work")
        default = self.make_db("default")
        result = self.run_hook("work")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.rows(work), [(7, 3)])
        self.assertEqual(self.rows(default), [])
        self.assertTrue((self.profile_dir("work") / "hook_state.json").exists())
        self.assertFalse((self.profile_dir("default") / "hook_state.json").exists())

    def test_second_run_records_only_the_delta(self):
        db = self.make_db("work")
        self.run_hook("work")
        result = self.run_hook("work")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.rows(db), [(7, 3)])

    def test_invalid_profile_writes_nothing(self):
        for bad in ["../escape", "Work", "a/b", "-x", "work\n", "a" * 33]:
            with self.subTest(profile=bad):
                result = self.run_hook(bad)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(result.stderr, "")
                self.assertEqual(self.all_files(), [])

    def test_missing_database_is_not_created(self):
        result = self.run_hook("work")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse((self.profile_dir("work") / "telemetry.db").exists())


if __name__ == "__main__":
    unittest.main()
