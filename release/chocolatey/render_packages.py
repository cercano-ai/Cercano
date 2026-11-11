#!/usr/bin/env python3
"""Render the non-publishing three-package Chocolatey set from a verified windows-x64 ZIP.

Output (a new directory; nothing outside it is touched):

  <output>/cercano-agent/cercano-agent.nuspec
  <output>/cercano-agent/tools/{cercano.exe, LICENSE.txt, VERIFICATION.txt,
                                chocolateyInstall.ps1, chocolateyBeforeModify.ps1,
                                chocolateyUninstall.ps1, cercano-guard.ps1}
  <output>/cercano-cli/...   same, with cercano-cli.exe
  <output>/cercano/cercano.nuspec   dependencies only, no files

Each component embeds only its own executable, so Chocolatey creates exactly
one shim per component: cercano-agent owns `cercano`, cercano-cli owns
`cercano-cli`, and `cercano` owns no command.

Trust boundary: checks the archive's SHA-256 against an independently trusted
value before parsing it, the exact release layout, the runtime inventory, and
an x64 PE32+ header on both executables. It does NOT verify signatures (there
are none) or the version the binaries report. It never downloads, installs,
packs, executes archive contents or publishes.
"""

import argparse
import hashlib
import os
import re
import shutil
import sys
import tempfile
import zipfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / "scripts"))
import pe_check
import runtime_inventory

HERE = Path(__file__).resolve().parent
PLATFORM = "windows-x64"
URL_TEMPLATE = "https://github.com/cercano-ai/Cercano/releases/download/v{version}/{name}"
VERSION_PATTERN = re.compile(r"\A(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\Z")
SHA256_PATTERN = re.compile(r"\A[0-9a-fA-F]{64}\Z")
PLACEHOLDER = re.compile(r"@[A-Z0-9_]+@")

# Package id -> the one archive executable it owns.
COMPONENTS = {
    "cercano-agent": "cercano.exe",
    "cercano-cli": "cercano-cli.exe",
}
CONVENIENCE = "cercano"
# The builder's own layout, so a payload change cannot drift from this check.
ARCHIVE_FILES = tuple(runtime_inventory.payload_modes(PLATFORM)) + (runtime_inventory.NAME,)


class ValidationError(Exception):
    """A release input failed validation; nothing is written."""


def parse_version(value):
    if not VERSION_PATTERN.match(value):
        raise ValidationError(
            f"version must be stable X.Y.Z without a leading v or leading zeros: {value!r}")
    return value


def parse_digest(value):
    if not SHA256_PATTERN.match(value):
        raise ValidationError("sha256 must be 64 hexadecimal characters")
    # Get-FileHash prints uppercase; compare in one case.
    return value.lower()


def sha256(data):
    return hashlib.sha256(data).hexdigest()


def hash_file(path):
    digest = hashlib.sha256()
    with open(path, "rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def require_x64_pe(name, data):
    """The release builder's rule (scripts/pe_check.py), as a ValidationError."""
    problem = pe_check.x64_pe_problem(data)
    if problem:
        raise ValidationError(f"{name}: {problem}")


def read_archive(path, version):
    """Return {relative path: bytes} for the exact release layout, no extraction to disk."""
    top = f"cercano-{version}-{PLATFORM}"
    if path.name != f"{top}.zip":
        raise ValidationError(f"archive must be named {top}.zip, got {path.name}")
    if not zipfile.is_zipfile(path):
        raise ValidationError(f"not a ZIP archive: {path.name}")
    files = {}
    with zipfile.ZipFile(path) as archive:
        for item in archive.infolist():
            name = item.filename
            if not name.startswith(top + "/"):
                raise ValidationError(f"unexpected member: {name!r}")
            relative = name[len(top) + 1:]
            if relative not in ARCHIVE_FILES:
                raise ValidationError(f"unexpected member: {name!r}")
            if relative in files:
                raise ValidationError(f"duplicate member: {name}")
            files[relative] = archive.read(item)
    missing = sorted(set(ARCHIVE_FILES) - set(files))
    if missing:
        raise ValidationError(f"archive is missing: {', '.join(missing)}")
    try:
        runtime_inventory.validate_archive(path, top, version, PLATFORM)
    except ValueError as error:
        raise ValidationError(str(error)) from error
    for exe in COMPONENTS.values():
        require_x64_pe(exe, files[f"bin/{exe}"])
    return files


def read_template(name):
    text = (HERE / name).read_text(encoding="utf-8-sig")
    if not text.isascii():
        raise ValidationError(f"{name}: templates must be ASCII (Windows PowerShell 5.1 reads BOM-less files as ANSI)")
    return text


def render(template_name, values):
    text = read_template(template_name)
    for key, value in values.items():
        text = text.replace(f"@{key}@", value)
    leftover = sorted(set(PLACEHOLDER.findall(text)))
    if leftover:
        raise ValidationError(f"{template_name}: unresolved placeholders {', '.join(leftover)}")
    return text


def write_text(path, text):
    # ASCII-only by test; CRLF suits Windows PowerShell 5.1 and Notepad.
    path.write_bytes(text.replace("\r\n", "\n").replace("\n", "\r\n").encode("ascii"))


def stage(root, version, archive_name, digest, files):
    common = {
        "VERSION": version,
        "ARCHIVE_NAME": archive_name,
        "RELEASE_URL": URL_TEMPLATE.format(version=version, name=archive_name),
        "SHA256": digest,
    }
    guard = read_template("guard.ps1")
    for package_id, exe in COMPONENTS.items():
        values = dict(common, PACKAGE_ID=package_id, EXECUTABLE=exe,
                      EXECUTABLE_SHA256=sha256(files[f"bin/{exe}"]))
        tools = root / package_id / "tools"
        tools.mkdir(parents=True)
        write_text(root / package_id / f"{package_id}.nuspec",
                   render(f"{package_id}.nuspec.in", values))
        (tools / exe).write_bytes(files[f"bin/{exe}"])
        (tools / "LICENSE.txt").write_bytes(files["LICENSE"])
        write_text(tools / "VERIFICATION.txt", render("VERIFICATION.txt.in", values))
        write_text(tools / "chocolateyInstall.ps1", render("chocolateyInstall.ps1.in", values))
        write_text(tools / "chocolateyBeforeModify.ps1",
                   render("chocolateyBeforeModify.ps1.in", values))
        write_text(tools / "chocolateyUninstall.ps1", render("chocolateyUninstall.ps1.in", values))
        write_text(tools / "cercano-guard.ps1", guard)
    (root / CONVENIENCE).mkdir()
    write_text(root / CONVENIENCE / f"{CONVENIENCE}.nuspec",
               render(f"{CONVENIENCE}.nuspec.in", common))


def build(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--version", required=True)
    parser.add_argument("--archive", required=True)
    parser.add_argument("--sha256", required=True)
    parser.add_argument("--output", required=True, help="new directory for the three package sources")
    args = parser.parse_args(argv)

    version = parse_version(args.version)
    expected = parse_digest(args.sha256)
    archive = Path(args.archive)
    if not archive.is_file():
        raise ValidationError(f"archive is not a regular file: {archive}")
    output = Path(args.output)
    if os.path.lexists(output):
        raise ValidationError(f"output already exists: {output}")
    if not output.parent.is_dir():
        raise ValidationError(f"output parent does not exist: {output.parent}")

    # Hash before parsing, so an unexpected archive is rejected unopened.
    actual = hash_file(archive)
    if actual != expected:
        raise ValidationError(f"archive sha256 mismatch: expected {expected}, got {actual}")
    files = read_archive(archive, version)

    # Stage beside the output, then rename: a failure leaves no partial set.
    staging = Path(tempfile.mkdtemp(prefix=".cercano-choco-", dir=output.parent))
    try:
        stage(staging, version, archive.name, expected, files)
        os.rename(staging, output)
    except BaseException:
        shutil.rmtree(staging, ignore_errors=True)
        raise
    return output


def main(argv=None):
    try:
        output = build(argv)
    except ValidationError as error:
        print(f"Error: {error}", file=sys.stderr)
        return 1
    print(f"Rendered package sources: {output}")
    print("Non-publishing fixture: unsigned, not release-ready. Pack with `choco pack <nuspec>`.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
