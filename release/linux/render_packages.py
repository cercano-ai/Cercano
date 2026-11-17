#!/usr/bin/env python3
"""Render the non-publishing three-package Linux set (deb, rpm, apk, archlinux) from a verified linux-x64 tarball.

Output (a new directory; nothing outside it is touched):

  <output>/cercano-agent.yaml   nfpm config
  <output>/cercano-agent/       its payload: cercano, copyright, changelog.gz,
                                lintian-overrides, LICENSE
  <output>/cercano-cli.yaml     same, for cercano-cli
  <output>/cercano.yaml         dependencies only, no command
  <output>/cercano/             copyright, changelog.gz, lintian-overrides, LICENSE

Each component installs only its own executable: cercano-agent owns
`cercano`, cercano-cli owns `cercano-cli`, and `cercano` owns no command.
Build each package with `nfpm package -f <output>/<pkg>.yaml -p <format>`.

Trust boundary: checks the archive's SHA-256 against an independently trusted
value before parsing it, the exact release layout, the runtime inventory, and
an x86-64 ELF64 header on both executables. It does NOT verify signatures
(there are none) or the version the binaries report. It never downloads,
installs, builds packages, executes archive contents or publishes.
"""

import argparse
import gzip
import hashlib
import json
import os
import re
import shutil
import struct
import sys
import tarfile
import tempfile
import textwrap
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / "scripts"))
import runtime_inventory

HERE = Path(__file__).resolve().parent
PLATFORM = "linux-x64"
ARCH = "amd64"  # nfpm's name; it maps this to x86_64 for rpm, apk and archlinux
VERSION_PATTERN = re.compile(r"\A(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\Z")
SHA256_PATTERN = re.compile(r"\A[0-9a-fA-F]{64}\Z")
MAINTAINER_PATTERN = re.compile(r"\A[^<>\n\"\\]+ <[^<>@\s\"\\]+@[^<>@\s\"\\]+>\Z")
PLACEHOLDER = re.compile(r"@[A-Z0-9_]+@")
HOMEPAGE = "https://github.com/cercano-ai/Cercano"
FORMATS = ("deb", "rpm", "apk", "archlinux")
NON_DEB = ("rpm", "apk", "archlinux")

ABOUT = ("Cercano is a free and open agent and harness: use frontier providers through "
         "a subscription sign-in or API key, run open-weight models, or combine them, "
         "with routing, delegation and context management built in.")

# Package name -> (archive executable it owns, summary, description paragraphs).
COMPONENTS = {
    "cercano-agent": ("cercano", "Cercano agent, which runs models and tools for Cercano clients", [
        ABOUT,
        "This package installs the agent and its `cercano` command. Install "
        "cercano-cli (or the cercano package for both) to use it from a terminal.",
        "Experimental and unsigned. A local model runtime is not bundled. Credential "
        "storage needs a Secret Service or KWallet keyring, which headless systems "
        "often lack.",
    ]),
    "cercano-cli": ("cercano-cli", "Cercano terminal client", [
        ABOUT,
        "This package installs the terminal client, `cercano-cli`. It needs an agent: "
        "install cercano-agent (or the cercano package for both), or point it at a "
        "remote agent with --address.",
        "Experimental and unsigned.",
    ]),
}
CONVENIENCE = "cercano"
CONVENIENCE_SUMMARY = "agent and terminal client for Cercano, installs both"
CONVENIENCE_DESCRIPTION = [
    ABOUT,
    "This convenience package installs cercano-agent and cercano-cli. It owns no "
    "commands itself.",
    "Experimental and unsigned.",
]
# Versioned dependency syntax per format, for the convenience package.
DEPENDS_SYNTAX = {"deb": "{} (>= {})", "rpm": "{} >= {}", "apk": "{}>={}", "archlinux": "{}>={}"}
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
    for exe, _, _ in COMPONENTS.values():
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


def changelog_gz(package, values):
    text = render("changelog.in", dict(values, PACKAGE=package)).encode("utf-8")
    # mtime=0 and no file name: the same inputs give the same bytes.
    return gzip.compress(text, compresslevel=9, mtime=0)


def q(value):
    """A YAML double-quoted scalar (JSON string syntax is valid YAML)."""
    return json.dumps(value)


def description(summary, paragraphs):
    """Summary line, then paragraphs wrapped to lintian's 80 columns, blank-line separated."""
    return summary + "\n" + "\n\n".join(textwrap.fill(p, 79) for p in paragraphs)


def nfpm_config(package, arch, version, maintainer, summary, paragraphs, payload, entries, depends):
    """nfpm config text. `entries` are (staged name, destination, mode, packagers or None for all)."""
    lines = [
        f"name: {package}",
        f"arch: {arch}",
        "platform: linux",
        f"version: {q(version)}",
        # Exactly X.Y.Z: no semver parsing and no release suffix, so the deb
        # stays a native version.
        "version_schema: none",
        f"maintainer: {q(maintainer)}",
        f"description: {q(description(summary, paragraphs))}",
        "vendor: Cercano",
        f"homepage: {q(HOMEPAGE)}",
        "license: Apache-2.0",
        "section: devel",
        "priority: optional",
    ]
    if depends:
        lines.append("overrides:")
        for fmt, syntax in DEPENDS_SYNTAX.items():
            lines.append(f"  {fmt}:")
            lines.append("    depends: [" + ", ".join(q(syntax.format(d, version)) for d in depends) + "]")
    lines.append("contents:")
    # Explicit directories, or rpm leaves them behind on removal.
    dirs = [(f"/usr/share/doc/{package}", ("deb",)), (f"/usr/share/licenses/{package}", NON_DEB)]
    for directory, packagers in dirs:
        for packager in packagers:
            lines += [f"  - dst: {directory}", "    type: dir", "    file_info: {mode: 0755}",
                      f"    packager: {packager}"]
    for name, dst, mode, packagers in entries:
        for packager in packagers or (None,):
            lines += [f"  - src: {q(str(payload / name))}", f"    dst: {dst}",
                      f"    file_info: {{mode: 0{mode:o}}}"]
            if packager:
                lines.append(f"    packager: {packager}")
    lines += ["deb:", "  compression: xz", "rpm:", "  compression: xz", f"  summary: {q(summary)}", ""]
    return "\n".join(lines)


def stage(root, final, version, maintainer, date, files):
    """Write payloads and configs under `root`; config paths point at `final`."""
    common = {"VERSION": version, "MAINTAINER": maintainer, "DATE": date}
    packages = [(p, exe, summary, paragraphs, None) for p, (exe, summary, paragraphs) in COMPONENTS.items()]
    packages.append((CONVENIENCE, None, CONVENIENCE_SUMMARY, CONVENIENCE_DESCRIPTION, list(COMPONENTS)))
    for package, exe, summary, paragraphs, depends in packages:
        payload = root / package
        entries = []
        if exe:
            write(payload / exe, files[f"bin/{exe}"], 0o755)
            entries.append((exe, f"/usr/bin/{exe}", 0o755, None))
            overrides = [f"statically-linked-binary [usr/bin/{exe}]",
                         f"unstripped-binary-or-object [usr/bin/{exe}]"]
        else:
            overrides = ["empty-binary-package"]
        write(payload / "copyright", render("copyright.in", common), 0o644)
        write(payload / "changelog.gz", changelog_gz(package, common), 0o644)
        # Accepted on purpose, see README.md ("lintian").
        write(payload / "lintian-overrides", "".join(f"{package}: {tag}\n" for tag in overrides), 0o644)
        write(payload / "LICENSE", files["LICENSE"], 0o644)
        entries += [
            ("copyright", f"/usr/share/doc/{package}/copyright", 0o644, ("deb",)),
            ("changelog.gz", f"/usr/share/doc/{package}/changelog.gz", 0o644, ("deb",)),
            ("lintian-overrides", f"/usr/share/lintian/overrides/{package}", 0o644, ("deb",)),
            ("LICENSE", f"/usr/share/licenses/{package}/LICENSE", 0o644, NON_DEB),
        ]
        config = nfpm_config(package, ARCH if exe else "all", version, maintainer, summary,
                             paragraphs, final / package, entries, depends)
        write(root / f"{package}.yaml", config, 0o644)
    for path in [root, *root.rglob("*")]:
        if path.is_dir():
            path.chmod(0o755)


def build(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--version", required=True)
    parser.add_argument("--archive", required=True)
    parser.add_argument("--sha256", required=True)
    parser.add_argument("--maintainer", required=True, help="package Maintainer field, 'Name <email>'")
    parser.add_argument("--output", required=True, help="new directory for the configs and payloads")
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
    # SOURCE_DATE_EPOCH, when set, makes the changelog date reproducible; nfpm
    # reads it too, for the file times inside the packages.
    epoch = os.environ.get("SOURCE_DATE_EPOCH")
    try:
        moment = time.gmtime(int(epoch) if epoch else None)
    except ValueError as error:
        raise ValidationError(f"SOURCE_DATE_EPOCH must be an integer: {epoch!r}") from error
    # Debian changelog trailer date: RFC 2822 with a numeric zone.
    date = time.strftime("%a, %d %b %Y %H:%M:%S +0000", moment)
    staging = Path(tempfile.mkdtemp(prefix=".cercano-linux-", dir=output.parent))
    try:
        stage(staging, output.resolve(), version, maintainer, date, files)
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
    print(f"Rendered nfpm configs and payloads: {output}")
    print("Non-publishing fixture: unsigned, not release-ready. Build with "
          f"`nfpm package -f {output}/<package>.yaml -p <{'|'.join(FORMATS)}>`.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
