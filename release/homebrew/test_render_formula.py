#!/usr/bin/env python3
"""Tests for the formula renderer. Fixtures are tiny inert files; nothing is
downloaded, installed, executed or published."""

import hashlib
import io
import tarfile
import tempfile
import unittest
from unittest.mock import patch
import subprocess
import shutil
from pathlib import Path

import render_formula

VERSION = "1.2.3"
TOP = f"cercano-{VERSION}-darwin-arm64"
ARCHIVE_NAME = f"{TOP}.tar.gz"


def member(name, mode=0o644, kind=tarfile.REGTYPE, data=b"inert", link=""):
    info = tarfile.TarInfo(name)
    info.type = kind
    info.mode = mode
    info.linkname = link
    if kind == tarfile.REGTYPE:
        info.size = len(data)
    return info, io.BytesIO(data)


def default_members():
    entries = [
        member(TOP, mode=0o755, kind=tarfile.DIRTYPE),
        member(f"{TOP}/bin", mode=0o755, kind=tarfile.DIRTYPE),
        member(f"{TOP}/bin/cercano", mode=0o755),
        member(f"{TOP}/bin/cercano-cli", mode=0o755),
        member(f"{TOP}/LICENSE"),
        member(f"{TOP}/README.txt"),
    ]

    files = {info.name[len(TOP)+1:]: (payload.getvalue(), info.mode)
             for info, payload in entries if info.isfile()}
    entries.append(member(f"{TOP}/runtime-inventory.json",
                          data=render_formula.runtime_inventory.encode(VERSION, "darwin-arm64", files)))
    return entries


def write_archive(directory, members=None, name=ARCHIVE_NAME):
    path = Path(directory) / name
    with tarfile.open(path, "w:gz") as archive:
        for info, payload in (default_members() if members is None else members):
            archive.addfile(info, payload if info.type == tarfile.REGTYPE else None)
    return path, hashlib.sha256(path.read_bytes()).hexdigest()


class RenderFormulaTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.dir = Path(self.tmp.name)
        self.output = self.dir / "cercano.rb"
        self.template = Path(__file__).with_name("cercano.rb.in")

    def run_render(self, archive, digest, version=VERSION, output=None, template=None):
        return render_formula.build([
            "--version", version,
            "--archive", str(archive),
            "--sha256", digest,
            "--output", str(output or self.output),
            "--template", str(template or self.template),
        ])

    def assert_rejected(self, fragment, *args, **kwargs):
        with self.assertRaises(render_formula.ValidationError) as caught:
            self.run_render(*args, **kwargs)
        self.assertIn(fragment, str(caught.exception))
        self.assertFalse(self.output.exists(), "a rejected release must not write a formula")

    def test_renders_valid_archive(self):
        archive, digest = write_archive(self.dir)
        self.run_render(archive, digest)
        text = self.output.read_text()
        text = self.output.with_name("cercano-agent.rb").read_text()
        self.assertIn(f'url "https://github.com/cercano-ai/Cercano/releases/download/v{VERSION}/{ARCHIVE_NAME}"', text)
        self.assertIn(f'version "{VERSION}"', text)
        self.assertIn(f'sha256 "{digest}"', text)
        self.assertNotIn("@", text.split("class Cercano")[1])

    @unittest.skipUnless(shutil.which("ruby"), "Ruby unavailable")
    def test_formula_ownership_dsl(self):
        subprocess.run(["ruby", str(Path(__file__).with_name("test_formula.rb"))], check=True, capture_output=True)

    def test_all_outputs_have_coordinated_metadata_and_ruby_syntax(self):
        archive, digest = write_archive(self.dir)
        self.run_render(archive, digest)
        for path in self.dir.glob("*.rb"):
            self.assertIn(f'version "{VERSION}"', path.read_text())
            self.assertIn(f'sha256 "{digest}"', path.read_text())
            if shutil.which("ruby"):
                subprocess.run(["ruby", "-c", str(path)], check=True, capture_output=True)
        self.assertEqual(len(list(self.dir.glob("*.rb"))), 3)

    def test_existing_component_or_dangling_symlink_blocks_entire_set(self):
        archive, digest = write_archive(self.dir)
        sibling = self.dir / "cercano-cli.rb"
        for symlink in (False, True):
            if symlink:
                sibling.symlink_to("missing")
            else:
                sibling.write_text("keep")
            with self.assertRaises(render_formula.ValidationError):
                self.run_render(archive, digest)
            self.assertFalse(self.output.exists())
            self.assertFalse((self.dir / "cercano-agent.rb").exists())
            sibling.unlink()

    def test_partial_write_failure_rolls_back_new_files(self):
        archive, digest = write_archive(self.dir)
        original = render_formula.os.open
        def failing(path, *args):
            if Path(path).name == "cercano-cli.rb":
                raise PermissionError("fixture failure")
            return original(path, *args)
        with patch.object(render_formula.os, "open", side_effect=failing):
            with self.assertRaisesRegex(render_formula.ValidationError, "could not write"):
                self.run_render(archive, digest)
        self.assertEqual(list(self.dir.glob("*.rb")), [])

    def test_rollback_preserves_replaced_output(self):
        archive, digest = write_archive(self.dir)
        original = render_formula.os.open
        sentinel = self.dir / "sentinel"
        sentinel.write_text("competitor replacement")

        def failing(path, *args):
            if Path(path).name == "cercano-cli.rb":
                self.output.unlink()
                sentinel.rename(self.output)
                raise PermissionError("original third-create failure")
            return original(path, *args)

        with patch.object(render_formula.os, "open", side_effect=failing):
            with self.assertRaisesRegex(render_formula.ValidationError,
                                        "original third-create failure") as caught:
                self.run_render(archive, digest)
        self.assertIsInstance(caught.exception.__cause__, PermissionError)
        self.assertEqual(self.output.read_text(), "competitor replacement")
        self.assertFalse((self.dir / "cercano-agent.rb").exists())
        self.assertFalse((self.dir / "cercano-cli.rb").exists())

    def test_rollback_continues_after_disappeared_output(self):
        archive, digest = write_archive(self.dir)
        original = render_formula.os.open

        def failing(path, *args):
            if Path(path).name == "cercano-cli.rb":
                # The last-created file is visited first during rollback.
                (self.dir / "cercano-agent.rb").unlink()
                raise PermissionError("original third-create failure")
            return original(path, *args)

        with patch.object(render_formula.os, "open", side_effect=failing):
            with self.assertRaisesRegex(render_formula.ValidationError,
                                        "original third-create failure") as caught:
                self.run_render(archive, digest)
        self.assertIsInstance(caught.exception.__cause__, PermissionError)
        self.assertEqual(list(self.dir.glob("*.rb")), [])

    def test_cleanup_error_does_not_mask_failure_or_stop_independent_cleanup(self):
        archive, digest = write_archive(self.dir)
        original_open = render_formula.os.open
        original_unlink = Path.unlink
        sibling = self.dir / "cercano-agent.rb"

        def failing_open(path, *args):
            if Path(path).name == "cercano-cli.rb":
                raise PermissionError("original third-create failure")
            return original_open(path, *args)

        def failing_unlink(path, *args, **kwargs):
            if path == sibling:
                raise PermissionError("fixture cleanup failure")
            return original_unlink(path, *args, **kwargs)

        with patch.object(render_formula.os, "open", side_effect=failing_open), \
                patch.object(Path, "unlink", failing_unlink):
            with self.assertRaisesRegex(render_formula.ValidationError,
                                        "original third-create failure") as caught:
                self.run_render(archive, digest)
        self.assertIsInstance(caught.exception.__cause__, PermissionError)
        self.assertIn("fixture cleanup failure", str(caught.exception))
        self.assertFalse(self.output.exists())
        self.assertTrue(sibling.exists())
        self.assertFalse((self.dir / "cercano-cli.rb").exists())

    def test_invalid_last_template_writes_nothing(self):
        archive, digest = write_archive(self.dir)
        for name in ("cercano", "cercano-agent", "cercano-cli"):
            source = self.template.with_name(name + ".rb.in")
            (self.dir / source.name).write_text(source.read_text())
        (self.dir / "cercano-cli.rb.in").write_text("invalid")
        with self.assertRaises(render_formula.ValidationError):
            self.run_render(archive, digest, template=self.dir / "cercano.rb.in")
        self.assertEqual(list(self.dir.glob("*.rb")), [])

    def test_concurrent_output_is_preserved_and_ours_rolled_back(self):
        archive, digest = write_archive(self.dir)
        original = render_formula.os.open
        target = self.dir / "cercano-agent.rb"
        def racing(path, *args):
            if Path(path) == target:
                target.write_text("concurrent owner")
            return original(path, *args)
        with patch.object(render_formula.os, "open", side_effect=racing):
            with self.assertRaises(render_formula.ValidationError):
                self.run_render(archive, digest)
        self.assertFalse(self.output.exists())
        self.assertEqual(target.read_text(), "concurrent owner")
        self.assertFalse((self.dir / "cercano-cli.rb").exists())

    def test_rejects_missing_or_tampered_inventory(self):
        for missing in (True, False):
            entries = [entry for entry in default_members()
                       if not entry[0].name.endswith("runtime-inventory.json")]
            if not missing:
                entries.append(member(f"{TOP}/runtime-inventory.json", data=b"{}"))
            archive, digest = write_archive(self.dir, entries)
            with self.assertRaises(render_formula.ValidationError):
                self.run_render(archive, digest)
            self.assertEqual(list(self.dir.glob("*.rb")), [])

    def test_rejects_digest_mismatch(self):
        archive, _ = write_archive(self.dir)
        self.assert_rejected("sha256 mismatch", archive, "0" * 64)

    def test_rejects_malformed_inputs(self):
        archive, digest = write_archive(self.dir)
        for version in ("v1.2.3", "1.2", "1.2.3-rc1", "01.2.3", "1.2.3.4"):
            with self.subTest(version=version):
                self.assert_rejected("version must be stable", archive, digest, version=version)
        for value in ("ABC" + "0" * 61, "0" * 63, ""):
            with self.subTest(sha=value):
                self.assert_rejected("64 lowercase hexadecimal", archive, value)

    def test_rejects_archive_name_and_version_mismatch(self):
        archive, digest = write_archive(self.dir, name="cercano-9.9.9-darwin-arm64.tar.gz")
        self.assert_rejected("archive must be named", archive, digest)

    def test_rejects_unsigned_rehearsal_archive_layout(self):
        # The unsigned build script emits a different top-level directory.
        name = f"cercano-{VERSION}-darwin-arm64-unsigned"
        members = [
            member(name, mode=0o755, kind=tarfile.DIRTYPE),
            member(f"{name}/bin", mode=0o755, kind=tarfile.DIRTYPE),
            member(f"{name}/bin/cercano", mode=0o755),
            member(f"{name}/bin/cercano-cli", mode=0o755),
        ]
        archive, digest = write_archive(self.dir, members)
        self.assert_rejected("unexpected directory", archive, digest)

    def test_rejects_incomplete_archive(self):
        members = [entry for entry in default_members() if not entry[0].name.endswith("cercano-cli")]
        archive, digest = write_archive(self.dir, members)
        self.assert_rejected("missing", archive, digest)

    def test_rejects_non_executable_binary(self):
        members = [entry for entry in default_members() if not entry[0].name.endswith("bin/cercano")]
        members.append(member(f"{TOP}/bin/cercano", mode=0o644))
        archive, digest = write_archive(self.dir, members)
        self.assert_rejected("not executable", archive, digest)

    def test_rejects_hostile_members(self):
        cases = {
            "symlink": [member(f"{TOP}/bin/evil", kind=tarfile.SYMTYPE, link="/etc/passwd")],
            "hardlink": [member(f"{TOP}/bin/evil", kind=tarfile.LNKTYPE, link=f"{TOP}/bin/cercano")],
            "fifo": [member(f"{TOP}/bin/pipe", kind=tarfile.FIFOTYPE)],
            "traversal": [member(f"{TOP}/../escape", mode=0o644)],
            "absolute": [member("/etc/passwd", mode=0o644)],
            "extra executable": [member(f"{TOP}/bin/helper", mode=0o755)],
            "extra file": [member(f"{TOP}/secrets.env", mode=0o644)],
            "second tree": [member("other", mode=0o755, kind=tarfile.DIRTYPE)],
        }
        for label, extra in cases.items():
            with self.subTest(case=label):
                archive, digest = write_archive(self.dir, default_members() + extra,
                                                name=ARCHIVE_NAME)
                with self.assertRaises(render_formula.ValidationError):
                    self.run_render(archive, digest)
                self.assertFalse(self.output.exists())

    def test_rejects_duplicate_member(self):
        archive, digest = write_archive(self.dir, default_members() + [member(f"{TOP}/LICENSE")])
        self.assert_rejected("duplicate member", archive, digest)

    def test_refuses_to_overwrite_existing_output(self):
        archive, digest = write_archive(self.dir)
        self.output.write_text("original formula")
        with self.assertRaises(render_formula.ValidationError):
            self.run_render(archive, digest)
        self.assertEqual(self.output.read_text(), "original formula")

    def test_rejects_template_without_exact_placeholders(self):
        archive, digest = write_archive(self.dir)
        for label, text in {
            "missing": 'url "https://example.invalid"\nversion "@VERSION@"\nsha256 "@SHA256@"',
            "duplicate": 'url "@RELEASE_URL@"\nurl "@RELEASE_URL@"\nversion "@VERSION@"\nsha256 "@SHA256@"',
            "unknown": 'url "@RELEASE_URL@"\nversion "@VERSION@"\nsha256 "@SHA256@"\nrevision "@REVISION@"',
        }.items():
            with self.subTest(case=label):
                template = self.dir / f"{label}.rb.in"
                template.write_text(text)
                output = self.output
                with self.assertRaises(render_formula.ValidationError):
                    self.run_render(archive, digest, output=output, template=template)
                self.assertFalse(output.exists())


if __name__ == "__main__":
    unittest.main()
