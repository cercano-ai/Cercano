#!/usr/bin/env python3
"""Render the non-publishing three-package Debian set from a verified linux-x64 tarball.

Output (a new directory; nothing outside it is touched):

  <output>/cercano-agent/DEBIAN/control
  <output>/cercano-agent/usr/bin/cercano
  <output>/cercano-agent/usr/share/doc/cercano-agent/{copyright, changelog.gz}
  <output>/cercano-agent/usr/share/lintian/overrides/cercano-agent
  <output>/cercano-cli/...   same, with usr/bin/cercano-cli
  <output>/cercano/...       dependencies only: control, docs, overrides

Each component installs only its own executable: cercano-agent owns
`cercano`, cercano-cli owns `cercano-cli`, and `cercano` owns no command.
Build each tree with `dpkg-deb --root-owner-group --build <tree>`.

Trust boundary: checks the archive's SHA-256 against an independently trusted
value before parsing it, the exact release layout, the runtime inventory, and
an x86-64 ELF64 header on both executables. It does NOT verify signatures
(there are none) or the version the binaries report. It never downloads,
installs, builds .deb files, executes archive contents or publishes.

Render on a Linux filesystem: file modes are part of the package.
"""

import argparse
import gzip
import hashlib
import os
import re
import shutil
import struct
import sys
import tarfile
import tempfile
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / "scripts"))
import runtime_inventory

HERE = Path(__file__).resolve().parent
PLATFORM = "linux-x64"
VERSION_PATTERN = re.compile(r"\A(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\Z")
SHA256_PATTERN = re.compile(r"\A[0-9a-fA-F]{64}\Z")
MAINTAINER_PATTERN = re.compile(r"\A[^<>\n]+ <[^<>@\s]+@[^<>@\s]+>\Z")
PLACEHOLDER = re.compile(r"@[A-Z0-9_]+@")

# Package name -> the one archive executable it owns, installed to /usr/bin.
COMPONENTS = {
    "cercano-agent": "cercano",
    "cercano-cli": "cercano-cli",
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
    return value.lower()


def parse_maintainer(value):
    if not MAINTAINER_PATTERN.match(value):
        raise ValidationError(f"maintainer must look like 'Name <email>': {value!r}")
    return value


def hash_file(path):
    digest = hashlib.sha256()
    with open(path, "rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def require_x64_elf(name, data):
    """The Linux release builder's rule (scripts/build-linux-release.py verify_elf)."""
    if len(data) < 20 or data[:4] != b"\x7fELF":
        raise ValidationError(f"{name}: not a Linux ELF executable")
    if data[4] != 2:
        raise ValidationError(f"{name}: expected 64-bit ELF (ELFCLASS64), got class {data[4]}")
    if data[5] != 1:
        raise ValidationError(f"{name}: expected little-endian ELF, got EI_DATA {data[5]}")
    e_type, e_machine = struct.unpack_from("<HH", data, 16)
    if e_machine != 62:
        raise ValidationError(f"{name}: expected x86-64 ELF (e_machine 62), got {e_machine}")
    if e_type not in (2, 3):
        raise ValidationError(f"{name}: expected an ELF executable type, got e_type {e_type}")


def read_archive(path, version):
    """Return {relative path: bytes} for the exact release layout, no extraction to disk."""
    top = f"cercano-{version}-{PLATFORM}"
    if path.name != f"{top}.tar.gz":
        raise ValidationError(f"archive must be named {top}.tar.gz, got {path.name}")
    files = {}
    try:
        with tarfile.open(path, "r:gz") as archive:
            for item in archive:
                name = item.name.rstrip("/")
                if item.isdir():
                    if name not in (top, f"{top}/bin"):
                        raise ValidationError(f"unexpected directory: {item.name!r}")
                    continue
                if not item.isfile():
                    raise ValidationError(f"unexpected non-regular member: {item.name!r}")
                if not name.startswith(top + "/"):
                    raise ValidationError(f"unexpected member: {item.name!r}")
                relative = name[len(top) + 1:]
                if relative not in ARCHIVE_FILES:
                    raise ValidationError(f"unexpected member: {item.name!r}")
                if relative in files:
                    raise ValidationError(f"duplicate member: {item.name}")
                files[relative] = archive.extractfile(item).read()
    except (tarfile.TarError, OSError, EOFError) as error:
        raise ValidationError(f"not a readable .tar.gz archive: {path.name}: {error}") from error
    missing = sorted(set(ARCHIVE_FILES) - set(files))
    if missing:
        raise ValidationError(f"archive is missing: {', '.join(missing)}")
    try:
        runtime_inventory.validate_archive(path, top, version, PLATFORM)
    except ValueError as error:
        raise ValidationError(str(error)) from error
    for exe in COMPONENTS.values():
        require_x64_elf(exe, files[f"bin/{exe}"])
    return files


def render(template_name, values):
    text = (HERE / template_name).read_text(encoding="utf-8")
    for key, value in values.items():
        text = text.replace(f"@{key}@", value)
    leftover = sorted(set(PLACEHOLDER.findall(text)))
    if leftover:
        raise ValidationError(f"{template_name}: unresolved placeholders {', '.join(leftover)}")
    return text


def write(path, data, mode):
    path.parent.mkdir(parents=True, exist_ok=True)
    if isinstance(data, str):
        data = data.replace("\r\n", "\n").encode("utf-8")
    path.write_bytes(data)
    path.chmod(mode)


def installed_size_kib(root):
    """Debian's Installed-Size: payload bytes (DEBIAN/ excluded), in KiB, rounded up."""
    total = 0
    for path in root.rglob("*"):
        if path.is_file() and "DEBIAN" not in path.relative_to(root).parts:
            total += path.stat().st_size
    return str(max(1, -(-total // 1024)))


def write_control(root, template_name, values):
    control = render(template_name, dict(values, INSTALLED_SIZE=installed_size_kib(root)))
    write(root / "DEBIAN" / "control", control, 0o644)


def changelog_gz(package, values):
    text = render("changelog.in", dict(values, PACKAGE=package)).encode("utf-8")
    # mtime=0 and no file name: the same inputs give the same bytes.
    return gzip.compress(text, compresslevel=9, mtime=0)


def write_docs(tree, package, values, overrides):
    doc = tree / "usr" / "share" / "doc" / package
    write(doc / "copyright", render("copyright.in", values), 0o644)
    write(doc / "changelog.gz", changelog_gz(package, values), 0o644)
    if overrides:
        # Accepted on purpose, see README.md ("lintian").
        lines = "".join(f"{package}: {tag}\n" for tag in overrides)
        write(tree / "usr" / "share" / "lintian" / "overrides" / package, lines, 0o644)


def stage(root, version, maintainer, date, files):
    common = {"VERSION": version, "MAINTAINER": maintainer, "DATE": date}
    for package, exe in COMPONENTS.items():
        tree = root / package
        write(tree / "usr" / "bin" / exe, files[f"bin/{exe}"], 0o755)
        write_docs(tree, package, common, [f"statically-linked-binary [usr/bin/{exe}]",
                                           f"unstripped-binary-or-object [usr/bin/{exe}]"])
        write_control(tree, f"{package}.control.in", common)
    tree = root / CONVENIENCE
    write_docs(tree, CONVENIENCE, common, ["empty-binary-package"])
    write_control(tree, f"{CONVENIENCE}.control.in", common)
    for path in [root, *root.rglob("*")]:
        if path.is_dir():
            path.chmod(0o755)


def build(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--version", required=True)
    parser.add_argument("--archive", required=True)
    parser.add_argument("--sha256", required=True)
    parser.add_argument("--maintainer", required=True, help="Debian Maintainer field, 'Name <email>'")
    parser.add_argument("--output", required=True, help="new directory for the three package trees")
    args = parser.parse_args(argv)

    version = parse_version(args.version)
    expected = parse_digest(args.sha256)
    maintainer = parse_maintainer(args.maintainer)
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
    # SOURCE_DATE_EPOCH, when set, makes the changelog date reproducible.
    epoch = os.environ.get("SOURCE_DATE_EPOCH")
    try:
        moment = time.gmtime(int(epoch) if epoch else None)
    except ValueError as error:
        raise ValidationError(f"SOURCE_DATE_EPOCH must be an integer: {epoch!r}") from error
    # Debian changelog trailer date: RFC 2822 with a numeric zone.
    date = time.strftime("%a, %d %b %Y %H:%M:%S +0000", moment)
    staging = Path(tempfile.mkdtemp(prefix=".cercano-deb-", dir=output.parent))
    try:
        stage(staging, version, maintainer, date, files)
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
    print(f"Rendered package trees: {output}")
    print("Non-publishing fixture: unsigned, not release-ready. "
          "Build with `dpkg-deb --root-owner-group --build <tree>`.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
