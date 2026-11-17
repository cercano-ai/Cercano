#!/usr/bin/env python3
"""Tests for the Linux package renderer. Fixtures are tiny inert files with a
valid x86-64 ELF64 header; nothing is downloaded, installed, executed or
published. The nfpm test runs only when `nfpm` is on PATH (it creates local
packages and installs nothing)."""

import gzip
import hashlib
import io
import os
import shutil
import struct
import subprocess
import tarfile
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import render_packages
from render_packages import runtime_inventory

VERSION = "1.2.3"
TOP = f"cercano-{VERSION}-linux-x64"
ARCHIVE_NAME = f"{TOP}.tar.gz"
MAINTAINER = "Test Maintainer <test@example.invalid>"
PACKAGES = ("cercano-agent", "cercano-cli", "cercano")


def fake_elf(tag, machine=62, ei_class=2, ei_data=1, e_type=2):
    header = bytearray(64)
    header[:4] = b"\x7fELF"
    header[4], header[5], header[6] = ei_class, ei_data, 1
    struct.pack_into("<HH", header, 16, e_type, machine)
    return bytes(header) + tag.encode()


def default_files():
    return {
        "LICENSE": (b"Apache License 2.0 (fixture)\n", 0o644),
        "README.txt": (b"readme\n", 0o644),
        "bin/cercano": (fake_elf("agent"), 0o755),
        "bin/cercano-cli": (fake_elf("cli"), 0o755),
    }


def write_archive(directory, files=None, extra=None, name=ARCHIVE_NAME, inventory=True):
    files = default_files() if files is None else files
    members = dict(files)
    if inventory:
        members[runtime_inventory.NAME] = (
            runtime_inventory.encode(VERSION, "linux-x64", files), 0o644)
    members.update(extra or {})
    path = Path(directory) / name
    with tarfile.open(path, "w:gz") as archive:
        for directory_name in (TOP, f"{TOP}/bin"):
            info = tarfile.TarInfo(directory_name)
            info.type, info.mode = tarfile.DIRTYPE, 0o755
            archive.addfile(info)
        for relative, (data, mode) in members.items():
            info = tarfile.TarInfo(f"{TOP}/{relative}")
            info.size, info.mode = len(data), mode
            archive.addfile(info, io.BytesIO(data))
    return path, hashlib.sha256(path.read_bytes()).hexdigest()


class RenderPackagesTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.dir = Path(self.tmp.name)
        self.output = self.dir / "packages"
        patcher = patch.dict(os.environ, {"SOURCE_DATE_EPOCH": "1700000000"})
        patcher.start()
        self.addCleanup(patcher.stop)

    def run_render(self, archive, digest, version=VERSION, maintainer=MAINTAINER, output=None):
        return render_packages.build([
            "--version", version, "--archive", str(archive), "--sha256", digest,
            "--maintainer", maintainer, "--output", str(output or self.output)])

    def assert_rejected(self, archive, digest, message, **kwargs):
        with self.assertRaisesRegex(render_packages.ValidationError, message):
            self.run_render(archive, digest, **kwargs)
        self.assertFalse(self.output.exists())
        self.assertEqual([p.name for p in self.dir.iterdir() if p.name.startswith(".cercano-linux-")], [])

    def config(self, package):
        return (self.output / f"{package}.yaml").read_text()

    def test_renders_a_config_and_payload_per_package(self):
        archive, digest = write_archive(self.dir)
        self.run_render(archive, digest)
        self.assertEqual(sorted(p.name for p in self.output.iterdir()),
                         sorted([*PACKAGES, *(f"{p}.yaml" for p in PACKAGES)]))
        for package, (exe, _, _) in render_packages.COMPONENTS.items():
            binary = self.output / package / exe
            self.assertEqual(binary.read_bytes(), default_files()[f"bin/{exe}"][0])
            self.assertEqual(binary.stat().st_mode & 0o777, 0o755)
            config = self.config(package)
            self.assertIn(f"name: {package}\n", config)
            self.assertIn("arch: amd64\n", config)
            self.assertIn(f'version: "{VERSION}"\n', config)
            self.assertIn("version_schema: none\n", config)
            self.assertIn(f'maintainer: "{MAINTAINER}"\n', config)
            self.assertIn(f'  - src: "{(self.output / package / exe).resolve()}"\n    dst: /usr/bin/{exe}\n', config)
            self.assertNotIn("depends", config)
        convenience = self.config("cercano")
        self.assertIn("arch: all\n", convenience)
        self.assertNotIn("/usr/bin/", convenience)
        self.assertIn(f'    depends: ["cercano-agent (>= {VERSION})", "cercano-cli (>= {VERSION})"]\n', convenience)
        self.assertIn(f'    depends: ["cercano-agent >= {VERSION}", "cercano-cli >= {VERSION}"]\n', convenience)
        self.assertEqual(convenience.count(f'    depends: ["cercano-agent>={VERSION}", "cercano-cli>={VERSION}"]\n'), 2)

    def test_config_sources_exist_and_deb_only_files_are_scoped(self):
        archive, digest = write_archive(self.dir)
        self.run_render(archive, digest)
        for package in PACKAGES:
            config = self.config(package)
            for line in config.splitlines():
                if line.startswith("  - src: "):
                    self.assertTrue(Path(line[len("  - src: "):].strip('"')).is_file(), line)
            for dst in ("copyright", "changelog.gz"):
                self.assertIn(f"    dst: /usr/share/doc/{package}/{dst}\n    file_info: {{mode: 0644}}\n    packager: deb\n", config)
            self.assertIn(f"    dst: /usr/share/lintian/overrides/{package}\n    file_info: {{mode: 0644}}\n    packager: deb\n", config)
            for packager in render_packages.NON_DEB:
                self.assertIn(f"    dst: /usr/share/licenses/{package}/LICENSE\n    file_info: {{mode: 0644}}\n    packager: {packager}\n", config)
                self.assertIn(f"  - dst: /usr/share/licenses/{package}\n    type: dir\n    file_info: {{mode: 0755}}\n    packager: {packager}\n", config)

    def test_docs_modes_and_description(self):
        archive, digest = write_archive(self.dir)
        self.run_render(archive, digest)
        payload = self.output / "cercano-agent"
        self.assertIn("License: Apache-2.0", (payload / "copyright").read_text())
        self.assertEqual((payload / "LICENSE").read_bytes(), default_files()["LICENSE"][0])
        changelog = gzip.decompress((payload / "changelog.gz").read_bytes()).decode()
        self.assertTrue(changelog.startswith(f"cercano-agent ({VERSION}) unstable; urgency=medium\n"))
        self.assertIn(f" -- {MAINTAINER}  Tue, 14 Nov 2023 22:13:20 +0000\n", changelog)
        self.assertIn("cercano-agent: statically-linked-binary [usr/bin/cercano]\n",
                      (payload / "lintian-overrides").read_text())
        self.assertEqual((self.output / "cercano" / "lintian-overrides").read_text(),
                         "cercano: empty-binary-package\n")
        for path in self.output.rglob("*"):
            expected = 0o755 if path.is_dir() or path.name in ("cercano", "cercano-cli") else 0o644
            self.assertEqual(path.stat().st_mode & 0o777, expected, path)
        text = render_packages.description(*render_packages.COMPONENTS["cercano-agent"][1:])
        self.assertTrue(text.startswith("Cercano agent, which runs models and tools for Cercano clients\n"))
        self.assertIn("\n\n", text)
        self.assertLessEqual(max(len(line) for line in text.splitlines()), 80)

    def test_render_is_reproducible(self):
        archive, digest = write_archive(self.dir)
        self.run_render(archive, digest)
        second = self.dir / "second"
        self.run_render(archive, digest, output=second)
        first_files = {p.relative_to(self.output): p.read_bytes() for p in self.output.rglob("*") if p.is_file()}
        second_files = {p.relative_to(second): p.read_bytes().replace(str(second.resolve()).encode(), str(self.output.resolve()).encode())
                        for p in second.rglob("*") if p.is_file()}
        self.assertEqual(first_files, second_files)

    def test_rejects_checksum_mismatch_before_parsing(self):
        archive, _ = write_archive(self.dir)
        with patch.object(render_packages, "read_archive") as read:
            self.assert_rejected(archive, "0" * 64, "sha256 mismatch")
            read.assert_not_called()

    def test_rejects_bad_arguments(self):
        archive, digest = write_archive(self.dir)
        self.assert_rejected(archive, digest, "stable X.Y.Z", version="v1.2.3")
        self.assert_rejected(archive, "xyz", "64 hexadecimal")
        self.assert_rejected(archive, digest, "Name <email>", maintainer="nobody")
        self.assert_rejected(archive, digest, "Name <email>", maintainer='Quote " Me <a@b.c>')
        self.output.mkdir()
        with self.assertRaisesRegex(render_packages.ValidationError, "already exists"):
            self.run_render(archive, digest)

    def test_rejects_wrong_archive_name(self):
        archive, digest = write_archive(self.dir, name="cercano-1.2.3-linux-arm64.tar.gz")
        self.assert_rejected(archive, digest, "must be named")

    def test_rejects_unexpected_and_missing_members(self):
        archive, digest = write_archive(self.dir, extra={"bin/extra": (b"x", 0o755)})
        self.assert_rejected(archive, digest, "unexpected member")
        files = default_files()
        del files["README.txt"]
        archive, digest = write_archive(self.dir, files=files, name=ARCHIVE_NAME)
        self.assert_rejected(archive, digest, "missing: README.txt")

    def test_rejects_inventory_mismatch(self):
        archive, digest = write_archive(self.dir, inventory=False,
                                        extra={runtime_inventory.NAME: (b"{}\n", 0o644)})
        self.assert_rejected(archive, digest, "runtime inventory mismatch")

    def test_rejects_non_x64_elf(self):
        for broken, message in ((fake_elf("a", machine=183), "e_machine 62"),
                                (fake_elf("a", ei_class=1), "ELFCLASS64"),
                                (fake_elf("a", e_type=1), "executable type"),
                                (b"MZ not an elf at all....", "not a Linux ELF")):
            files = default_files()
            files["bin/cercano"] = (broken, 0o755)
            archive, digest = write_archive(self.dir, files=files)
            self.assert_rejected(archive, digest, message)

    @unittest.skipUnless(shutil.which("nfpm"), "nfpm not on PATH")
    def test_nfpm_builds_every_format(self):
        archive, digest = write_archive(self.dir)
        self.run_render(archive, digest)
        dist = self.dir / "dist"
        dist.mkdir()
        for package in PACKAGES:
            for fmt in render_packages.FORMATS:
                subprocess.run(["nfpm", "package", "-f", str(self.output / f"{package}.yaml"),
                                "-p", fmt, "-t", str(dist)], check=True, capture_output=True)
        self.assertEqual(sorted(p.name for p in dist.iterdir()), sorted([
            f"cercano-agent_{VERSION}_amd64.deb", f"cercano-cli_{VERSION}_amd64.deb", f"cercano_{VERSION}_all.deb",
            f"cercano-agent-{VERSION}-1.x86_64.rpm", f"cercano-cli-{VERSION}-1.x86_64.rpm", f"cercano-{VERSION}-1.noarch.rpm",
            f"cercano-agent_{VERSION}_x86_64.apk", f"cercano-cli_{VERSION}_x86_64.apk", f"cercano_{VERSION}_noarch.apk",
            f"cercano-agent-{VERSION}-1-x86_64.pkg.tar.zst", f"cercano-cli-{VERSION}-1-x86_64.pkg.tar.zst",
            f"cercano-{VERSION}-1-any.pkg.tar.zst",
        ]))


if __name__ == "__main__":
    unittest.main()
