#!/usr/bin/env python3
"""Tests for the Chocolatey package renderer. Fixtures are tiny inert files
with a valid x64 PE header; nothing is downloaded, installed, executed or
published. `choco pack` runs only when choco is on PATH (it creates a local
.nupkg and installs nothing)."""

import hashlib
import shutil
import struct
import subprocess
import tempfile
import unittest
import zipfile
from pathlib import Path
from unittest.mock import patch

import render_packages
from render_packages import runtime_inventory

VERSION = "1.2.3"
TOP = f"cercano-{VERSION}-windows-x64"
ARCHIVE_NAME = f"{TOP}.zip"


def fake_pe(tag, machine=0x8664):
    dos = bytearray(64)
    dos[:2] = b"MZ"
    struct.pack_into("<I", dos, 60, 64)
    header = bytearray(26)
    header[:4] = b"PE\0\0"
    struct.pack_into("<H", header, 4, machine)
    struct.pack_into("<H", header, 22, 0x0022)  # executable, large-address-aware
    struct.pack_into("<H", header, 24, 0x20b)   # PE32+
    return bytes(dos + header) + tag.encode()


def default_files():
    return {
        "LICENSE": (b"Apache License 2.0 (fixture)\n", 0o644),
        "README.txt": (b"readme\n", 0o644),
        "bin/cercano.exe": (fake_pe("agent"), 0o755),
        "bin/cercano-cli.exe": (fake_pe("cli"), 0o755),
    }


def write_archive(directory, files=None, extra=None, name=ARCHIVE_NAME, inventory=True):
    files = default_files() if files is None else files
    path = Path(directory) / name
    with zipfile.ZipFile(path, "w") as archive:
        members = dict(files)
        if inventory:
            members[runtime_inventory.NAME] = (
                runtime_inventory.encode(VERSION, "windows-x64", files), 0o644)
        members.update(extra or {})
        for relative, (data, mode) in members.items():
            info = zipfile.ZipInfo(f"{TOP}/{relative}", date_time=(1980, 1, 1, 0, 0, 0))
            info.create_system = 3
            info.external_attr = (0o100000 | mode) << 16
            archive.writestr(info, data)
    return path, hashlib.sha256(path.read_bytes()).hexdigest()


class RenderPackagesTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.dir = Path(self.tmp.name)
        self.output = self.dir / "packages"

    def run_render(self, archive, digest, version=VERSION, output=None):
        return render_packages.build([
            "--version", version, "--archive", str(archive),
            "--sha256", digest, "--output", str(output or self.output)])

    def assert_rejected(self, archive, digest, pattern, **kwargs):
        with self.assertRaisesRegex(render_packages.ValidationError, pattern):
            self.run_render(archive, digest, **kwargs)
        self.assertFalse(self.output.exists())
        leftovers = [p.name for p in self.dir.iterdir() if p.name.startswith(".cercano-choco-")]
        self.assertEqual(leftovers, [])

    def test_renders_three_packages_with_exclusive_command_ownership(self):
        archive, digest = write_archive(self.dir)
        self.run_render(archive, digest)
        files = sorted(p.relative_to(self.output).as_posix()
                       for p in self.output.rglob("*") if p.is_file())
        tools = ["LICENSE.txt", "VERIFICATION.txt", "cercano-guard.ps1",
                 "chocolateyBeforeModify.ps1", "chocolateyInstall.ps1", "chocolateyUninstall.ps1"]
        expected = sorted(
            ["cercano/cercano.nuspec",
             "cercano-agent/cercano-agent.nuspec", "cercano-agent/tools/cercano.exe",
             "cercano-cli/cercano-cli.nuspec", "cercano-cli/tools/cercano-cli.exe"]
            + [f"cercano-agent/tools/{t}" for t in tools]
            + [f"cercano-cli/tools/{t}" for t in tools])
        self.assertEqual(files, expected)
        # Exactly one executable per component: one shim each, none for `cercano`.
        exes = sorted(p.relative_to(self.output).as_posix() for p in self.output.rglob("*.exe"))
        self.assertEqual(exes, ["cercano-agent/tools/cercano.exe", "cercano-cli/tools/cercano-cli.exe"])

    def test_payload_bytes_and_verification_match_the_archive(self):
        archive, digest = write_archive(self.dir)
        self.run_render(archive, digest)
        for package_id, exe in render_packages.COMPONENTS.items():
            tools = self.output / package_id / "tools"
            data = (tools / exe).read_bytes()
            self.assertEqual(data, default_files()[f"bin/{exe}"][0])
            verification = (tools / "VERIFICATION.txt").read_text()
            self.assertIn(digest, verification)
            self.assertIn(hashlib.sha256(data).hexdigest(), verification)
            self.assertIn(f"/releases/download/v{VERSION}/{ARCHIVE_NAME}", verification)
            self.assertEqual((tools / "LICENSE.txt").read_bytes(), default_files()["LICENSE"][0])

    def test_install_and_uninstall_refuse_and_before_modify_only_warns(self):
        archive, digest = write_archive(self.dir)
        self.run_render(archive, digest)
        for package_id, exe in render_packages.COMPONENTS.items():
            tools = self.output / package_id / "tools"
            install = (tools / "chocolateyInstall.ps1").read_text()
            uninstall = (tools / "chocolateyUninstall.ps1").read_text()
            before = (tools / "chocolateyBeforeModify.ps1").read_text()
            call = f"-PackageId '{package_id}' -Executables @('{exe}')"
            for refusing in (install, uninstall):
                self.assertIn(call, refusing)
                self.assertNotIn("-WarnOnly", refusing)
            self.assertIn(call + " -WarnOnly", before)
            guard = (tools / "cercano-guard.ps1").read_text()
            for text in (install, uninstall, before, guard):
                # No process control, launches or user-data access from package scripts.
                for forbidden in ("Stop-Process", "taskkill", "Start-Process", "Remove-Item",
                                  ".Kill(", "Invoke-Item", "USERPROFILE", "APPDATA"):
                    self.assertNotIn(forbidden, text)

    def test_rendered_text_is_ascii_crlf_without_placeholders(self):
        archive, digest = write_archive(self.dir)
        self.run_render(archive, digest)
        for path in self.output.rglob("*"):
            if path.suffix in (".nuspec", ".ps1", ".txt") and path.name != "LICENSE.txt":
                data = path.read_bytes()
                data.decode("ascii")
                self.assertNotIn(b"\n", data.replace(b"\r\n", b""), path.name)
                self.assertIsNone(render_packages.PLACEHOLDER.search(data.decode()), path.name)

    def test_convenience_package_depends_on_both_and_owns_nothing(self):
        archive, digest = write_archive(self.dir)
        self.run_render(archive, digest)
        nuspec = (self.output / "cercano" / "cercano.nuspec").read_text()
        self.assertIn(f'<dependency id="cercano-agent" version="{VERSION}" />', nuspec)
        self.assertIn(f'<dependency id="cercano-cli" version="{VERSION}" />', nuspec)
        self.assertIn("<files />", nuspec)
        self.assertEqual([p.name for p in (self.output / "cercano").iterdir()], ["cercano.nuspec"])
        # Components must not depend on each other: a remote-only CLI stays possible.
        for package_id in render_packages.COMPONENTS:
            text = (self.output / package_id / f"{package_id}.nuspec").read_text()
            self.assertNotIn("<dependencies>", text)

    def test_rejects_checksum_mismatch(self):
        archive, _ = write_archive(self.dir)
        self.assert_rejected(archive, "0" * 64, "sha256 mismatch")

    def test_rejects_wrong_archive_name(self):
        archive, digest = write_archive(self.dir, name="cercano-1.2.3-windows-arm64.zip")
        self.assert_rejected(archive, digest, "must be named")

    def test_rejects_unexpected_member(self):
        archive, digest = write_archive(self.dir, extra={"bin/llama-server.exe": (fake_pe("x"), 0o755)})
        self.assert_rejected(archive, digest, "unexpected member")

    def test_rejects_missing_executable(self):
        files = default_files()
        del files["bin/cercano-cli.exe"]
        archive, digest = write_archive(self.dir, files=files)
        self.assert_rejected(archive, digest, "missing")

    def test_rejects_inventory_mismatch(self):
        archive, digest = write_archive(self.dir, inventory=False, extra={
            runtime_inventory.NAME: (runtime_inventory.encode(VERSION, "windows-x64", {}), 0o644)})
        self.assert_rejected(archive, digest, "inventory")

    def test_rejects_non_x64_executable(self):
        files = default_files()
        files["bin/cercano.exe"] = (fake_pe("arm", machine=0xAA64), 0o755)
        archive, digest = write_archive(self.dir, files=files)
        self.assert_rejected(archive, digest, "x64 PE32")

    def test_rejects_non_pe_executable(self):
        files = default_files()
        files["bin/cercano-cli.exe"] = (b"#!/bin/sh\n", 0o755)
        archive, digest = write_archive(self.dir, files=files)
        self.assert_rejected(archive, digest, "not a Windows PE")

    def test_rejects_unstable_version_and_bad_digest(self):
        archive, digest = write_archive(self.dir)
        self.assert_rejected(archive, digest, "stable X.Y.Z", version="1.2.3-rc1")
        self.assert_rejected(archive, digest[:-1], "64 hexadecimal")

    def test_accepts_uppercase_digest_from_get_filehash(self):
        archive, digest = write_archive(self.dir)
        self.run_render(archive, digest.upper())
        self.assertIn(digest, (self.output / "cercano-agent" / "tools" / "VERIFICATION.txt").read_text())

    def test_non_ascii_template_is_a_validation_error(self):
        archive, digest = write_archive(self.dir)
        with patch.object(render_packages, "HERE", self.dir):
            for name in Path(render_packages.__file__).parent.glob("*.in"):
                shutil.copy(name, self.dir / name.name)
            (self.dir / "guard.ps1").write_text("# café\n", encoding="utf-8")
            self.assert_rejected(archive, digest, "must be ASCII")

    def test_refuses_existing_output(self):
        archive, digest = write_archive(self.dir)
        self.output.mkdir()
        marker = self.output / "keep.txt"
        marker.write_text("mine")
        with self.assertRaisesRegex(render_packages.ValidationError, "already exists"):
            self.run_render(archive, digest)
        self.assertEqual(marker.read_text(), "mine")

    @unittest.skipUnless(shutil.which("choco"), "choco not on PATH")
    def test_choco_pack_produces_expected_nupkgs(self):
        archive, digest = write_archive(self.dir)
        self.run_render(archive, digest)
        out = self.dir / "nupkg"
        out.mkdir()
        for package_id in ("cercano-agent", "cercano-cli", "cercano"):
            nuspec = self.output / package_id / f"{package_id}.nuspec"
            result = subprocess.run(["choco", "pack", str(nuspec), "--out", str(out),
                                     "--limit-output", "--no-progress"],
                                    capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            with zipfile.ZipFile(out / f"{package_id}.{VERSION}.nupkg") as nupkg:
                names = {n for n in nupkg.namelist() if n.startswith("tools/")}
            if package_id == "cercano":
                self.assertEqual(names, set())
            else:
                exe = render_packages.COMPONENTS[package_id]
                self.assertIn(f"tools/{exe}", names)
                self.assertEqual({n for n in names if n.endswith(".exe")}, {f"tools/{exe}"})


@unittest.skipUnless(shutil.which("powershell"), "Windows PowerShell not available")
class GuardScriptTest(unittest.TestCase):
    def test_guard_script(self):
        script = Path(__file__).with_name("test_guard.ps1")
        result = subprocess.run(["powershell", "-NoProfile", "-ExecutionPolicy", "Bypass",
                                 "-File", str(script)], capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)


if __name__ == "__main__":
    unittest.main()
