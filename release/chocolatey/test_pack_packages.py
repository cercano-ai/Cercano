"""Portable packing boundary tests: simulated choco output, no executables run.

The existing renderer suite separately exercises real choco when available.
These tests do not constitute native packing evidence.
"""
import hashlib
import json
import os
import xml.etree.ElementTree as ET
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch
import zipfile

import check_packages
import pack_packages
from test_render_packages import VERSION, default_files, write_archive


class PackPackagesTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        files = default_files()
        files["LICENSE"] = (b"License from the older independently verified release\n", 0o644)
        self.archive, self.digest = write_archive(self.root, files=files)
        self.output = self.root / "output"
        self.args = ["--version", VERSION, "--archive", str(self.archive),
                     "--sha256", self.digest, "--output", str(self.output)]
        self.calls = []
        self.mutate = lambda package_id, members: members

    def fake_pack(self, command, check):
        self.assertTrue(check)
        self.assertEqual(command[1], "pack")
        self.calls.append(command)
        nuspec = Path(command[2])
        source = nuspec.parent
        members = {p.relative_to(source).as_posix(): p.read_bytes()
                   for p in source.rglob("*") if p.is_file()}
        members["[Content_Types].xml"] = b"fixture NuGet bookkeeping"
        members = self.mutate(source.name, members)
        package_version = ET.parse(nuspec).getroot().find("{*}metadata/{*}version").text
        dest = Path(command[command.index("--out") + 1]) / f"{source.name}.{package_version}.nupkg"
        with zipfile.ZipFile(dest, "w") as archive:
            for name, data in members.items():
                archive.writestr(name, data)

    def build(self):
        with patch.object(pack_packages.shutil, "which", return_value="choco"), \
                patch.object(pack_packages.subprocess, "run", side_effect=self.fake_pack):
            return pack_packages.build(self.args)

    def assert_refused(self, pattern):
        with self.assertRaisesRegex(pack_packages.ValidationError, pattern):
            self.build()
        self.assertFalse(self.output.exists())
        self.assertEqual(list(self.root.glob(".cercano-pack-*")), [])

    def test_full_boundary_renders_packs_inspects_then_checksums_all_three(self):
        self.build()
        self.assertEqual(len(self.calls), 3)
        packages = self.output / "packages"
        self.assertEqual(len(list(packages.iterdir())), 6)
        for package_id in pack_packages.PACKAGE_IDS:
            path = packages / f"{package_id}.{VERSION}.nupkg"
            self.assertEqual(path.with_name(path.name + ".sha256").read_text(),
                             f"{hashlib.sha256(path.read_bytes()).hexdigest()}  {path.name}\n")

    def test_package_only_revision_preserves_application_url_and_binary_version(self):
        revision = VERSION + ".1"
        self.args += ["--package-version", revision]
        output_file = self.root / "github-output"
        with patch.dict(os.environ, {"GITHUB_OUTPUT": str(output_file)}):
            self.build()
        manifest = json.loads(output_file.read_text().removeprefix("package_digests="))
        self.assertEqual(set(manifest), {f"{p}.{revision}.nupkg" for p in pack_packages.PACKAGE_IDS})
        for name, digest in manifest.items():
            self.assertEqual(hashlib.sha256((self.output / "packages" / name).read_bytes()).hexdigest(), digest)
        rendered = self.output / "rendered"
        for component in ("cercano-agent", "cercano-cli"):
            source = (rendered / component / (component + ".nuspec")).read_text()
            self.assertIn(f"<version>{revision}</version>", source)
            self.assertIn(f"/tag/v{VERSION}</releaseNotes>", source)
            verification = (rendered / component / "tools/VERIFICATION.txt").read_text()
            self.assertIn(f"/download/v{VERSION}/", verification)
            self.assertNotIn(f"/download/v{revision}/", verification)
        convenience = (rendered / "cercano/cercano.nuspec").read_text()
        self.assertIn(f'version="{revision}"', convenience)
        def run(command, **kwargs):
            return subprocess.CompletedProcess(command, 0, Path(command[0]).stem + " v" + VERSION, "")
        with patch.object(check_packages.subprocess, "run", side_effect=run):
            check_packages.check(VERSION, self.output / "packages", revision)

    def test_package_revision_validation_and_no_outputs_on_failed_inspection(self):
        for invalid in ("1.2.4", VERSION + ".0", VERSION + ".01", VERSION + ".1.2"):
            with self.assertRaises(pack_packages.ValidationError):
                pack_packages.render_packages.parse_package_version(invalid, VERSION)
        output_file = self.root / "github-output"
        self.mutate = lambda package_id, members: dict(members, **{"bad.txt": b"unexpected"})
        with patch.dict(os.environ, {"GITHUB_OUTPUT": str(output_file)}):
            self.assert_refused("unexpected package member")
        self.assertFalse(output_file.exists())

    def test_native_checker_reads_actual_packages_and_uses_isolated_version_only(self):
        self.build()
        calls = []

        def run(command, **kwargs):
            calls.append(command)
            self.assertEqual(command[1:], ["--version"])
            self.assertTrue(Path(command[0]).is_file())
            self.assertEqual(Path(command[0]).parent, Path(kwargs["cwd"]))
            self.assertNotEqual(kwargs["env"]["HOME"], str(Path.home()))
            self.assertEqual(kwargs["timeout"], 30)
            return subprocess.CompletedProcess(command, 0, Path(command[0]).stem + " v" + VERSION, "")

        with patch.object(check_packages.subprocess, "run", side_effect=run):
            check_packages.check(VERSION, self.output / "packages")
        self.assertEqual(len(calls), 2)
        with patch.object(check_packages.subprocess, "run", return_value=
                          subprocess.CompletedProcess([], 0, "cercano 9.9.9", "")):
            with self.assertRaisesRegex(ValueError, "requested release version"):
                check_packages.check(VERSION, self.output / "packages")

    def test_missing_choco_is_failure_not_skip(self):
        with patch.object(pack_packages.shutil, "which", return_value=None):
            with self.assertRaisesRegex(pack_packages.ValidationError, "choco is required"):
                pack_packages.build(self.args)
        self.assertFalse(self.output.exists())

    def test_checksum_mismatch_never_packs(self):
        self.args[self.args.index(self.digest)] = "0" * 64
        self.assert_refused("sha256 mismatch")
        self.assertEqual(self.calls, [])

    def test_pack_nonzero_leaves_no_output(self):
        with patch.object(pack_packages.shutil, "which", return_value="choco"), \
                patch.object(pack_packages.subprocess, "run",
                             side_effect=subprocess.CalledProcessError(1, "choco")):
            with self.assertRaises(subprocess.CalledProcessError):
                pack_packages.build(self.args)
        self.assertFalse(self.output.exists())
        self.assertEqual(list(self.root.glob(".cercano-pack-*")), [])

    def test_packed_payload_license_metadata_and_dependencies_fail_closed(self):
        cases = [("tools/cercano.exe", b"changed", "payload"),
                 ("tools/LICENSE.txt", b"changed", "payload"),
                 ("tools/unexpected.exe", b"changed", "manifest"),
                 ("outside.exe", b"changed", "unexpected")]
        for name, data, pattern in cases:
            with self.subTest(name=name):
                def mutate(package_id, members):
                    if package_id == "cercano-agent":
                        members[name] = data
                    return members
                self.mutate = mutate
                self.assert_refused(pattern)
        for old, new, pattern in [(b"Copyright Cercloud", b"Wrong copyright", "metadata"),
                                  (b"<version>1.2.3</version>", b"<version>9.9.9</version>", "metadata"),
                                  (b'id="cercano-cli"', b'id="wrong-cli"', "dependencies")]:
            with self.subTest(old=old):
                def mutate(package_id, members):
                    name = f"{package_id}.nuspec"
                    members[name] = members[name].replace(old, new)
                    return members
                self.mutate = mutate
                self.assert_refused(pattern)

    def test_existing_output_is_preserved(self):
        self.output.mkdir()
        marker = self.output / "keep"
        marker.write_text("untouched")
        with self.assertRaisesRegex(pack_packages.ValidationError, "already exists"):
            self.build()
        self.assertEqual(marker.read_text(), "untouched")


if __name__ == "__main__":
    unittest.main()
