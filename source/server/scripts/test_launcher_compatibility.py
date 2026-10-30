"""Disposable metadata admission tests; never access live state or inventory."""
import importlib.util
from pathlib import Path
import sqlite3
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('launcher', Path(__file__).with_name('cercano-launcher.py'))
launcher = importlib.util.module_from_spec(spec)
spec.loader.exec_module(launcher)


def logical(path):
    conn = sqlite3.connect(path.as_uri() + '?mode=ro', uri=True)
    try:
        return (conn.execute('pragma user_version').fetchall(), list(conn.iterdump()))
    finally:
        conn.close()


class CompatibilityTests(unittest.TestCase):
    def test_metadata_only_in_wal(self):
        for version, contract, expected in ((2, 'cercano-development-owned-v2', True),
                                            (99, 'unknown', ValueError)):
            for missing_shm in (False, True):
                with self.subTest(version=version, missing_shm=missing_shm), tempfile.TemporaryDirectory() as root:
                    db = Path(root).resolve() / 'fixture.db'
                    conn = sqlite3.connect(db)
                    conn.execute('create table application_data(value text)')
                    conn.execute("insert into application_data values ('preserved')")
                    conn.commit()
                    conn.close()
                    subprocess.run([sys.executable, '-c', '''import os,sqlite3,sys
c=sqlite3.connect(sys.argv[1])
c.execute('pragma journal_mode=wal')
c.execute('create table coexistence_metadata(singleton integer, contract text, schema_version integer)')
c.execute('insert into coexistence_metadata values(1,?,?)', (sys.argv[2], int(sys.argv[3])))
c.execute('pragma user_version='+sys.argv[3])
c.commit()
os._exit(0)
''', str(db), contract, str(version)], check=True)
                    # Immutable is used only as a test oracle: metadata is NOT in main DB.
                    conn = sqlite3.connect(db.as_uri()+'?immutable=1', uri=True)
                    self.assertEqual(conn.execute('pragma user_version').fetchone()[0], 0)
                    conn.close()
                    before = logical(db)
                    if missing_shm:
                        Path(str(db)+'-shm').unlink(missing_ok=True)  # disposable, no open connections
                    statements = []
                    connect = sqlite3.connect
                    def traced(database, **kwargs):
                        self.assertEqual(database, db.as_uri()+'?mode=ro')
                        c = connect(database, **kwargs)
                        c.set_trace_callback(statements.append)
                        return c
                    with patch.object(launcher.sqlite3, 'connect', traced), patch.object(
                            launcher.tempfile, 'TemporaryDirectory', side_effect=AssertionError('inspection must not copy')):
                        if expected is ValueError:
                            with self.assertRaises(ValueError):
                                launcher.compatible_store(db)
                        else:
                            self.assertIs(launcher.compatible_store(db), expected)
                    self.assertEqual(logical(db), before)
                    self.assertFalse(any(word in sql.lower() for sql in statements
                                         for word in ('integrity_check', 'checkpoint', 'insert', 'update', 'create')))
                    self.assertIn('BEGIN', statements)

    def test_unknown_and_incompatible_metadata(self):
        for statement in (
                'pragma user_version=99',
                'delete from coexistence_metadata',
                "update coexistence_metadata set contract='unknown'",
                'update coexistence_metadata set singleton=0',
                'insert into coexistence_metadata select * from coexistence_metadata',
                'drop table coexistence_metadata'):
            with self.subTest(statement=statement), tempfile.TemporaryDirectory() as root:
                db = Path(root).resolve() / 'fixture.db'
                conn = sqlite3.connect(db)
                conn.executescript("create table coexistence_metadata(singleton,contract,schema_version);"
                                   "insert into coexistence_metadata values(1,'cercano-development-owned-v2',2);"
                                   "pragma user_version=2;")
                conn.execute(statement)
                conn.commit()
                conn.close()
                before = logical(db)
                with self.assertRaises(ValueError):
                    launcher.compatible_store(db)
                self.assertEqual(logical(db), before)

    def test_source_boundary(self):
        import inspect
        source = inspect.getsource(launcher.compatible_store)
        for forbidden in ('immutable', 'integrity_check', 'wal_checkpoint', 'copyfile', 'chmod', 'unlink'):
            self.assertNotIn(forbidden, source)
        self.assertIn('integrity_check', inspect.getsource(launcher.confirmed_upgrade))


if __name__ == '__main__':
    unittest.main()
