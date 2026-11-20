#!/usr/bin/env python3
"""Render, locally pack and inspect the release set; never install or publish.

The caller supplies the exact build archive and recorded digest. Successful
output is atomic: sources under rendered/, three inspected packages and their
SHA-256 sidecars under packages/. Native choco must be on PATH (never skipped).
"""
import argparse
import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import xml.etree.ElementTree as ET
import zipfile

import render_packages

PACKAGE_IDS = (*render_packages.COMPONENTS, render_packages.CONVENIENCE)
ValidationError = render_packages.ValidationError


def inspect_package(package, rendered, package_id, version):
    """Compare packed metadata and every tools byte with verified sources."""
    source = rendered / package_id
    expected = ET.parse(source / f"{package_id}.nuspec").getroot().find("{*}metadata")
    with zipfile.ZipFile(package) as archive:
        names = archive.namelist()
        if len(names) != len(set(names)) or any("\\" in n or ".." in n.split("/") for n in names):
            raise ValidationError("duplicate or unsafe package member")
        actual = ET.fromstring(archive.read(f"{package_id}.nuspec")).find("{*}metadata")
        if actual is None:
            raise ValidationError("missing package metadata")
        for field in expected:
            name = field.tag.split("}")[-1]
            if name == "dependencies":
                continue
            values = actual.findall(f"{{*}}{name}")
            if len(values) != 1 or (values[0].text or "").strip() != (field.text or "").strip():
                raise ValidationError(f"metadata mismatch: {package_id}/{name}")
        dependencies = actual.findall("{*}dependencies")
        if package_id == render_packages.CONVENIENCE:
            if len(dependencies) != 1 or len(dependencies[0]) != 2:
                raise ValidationError("convenience dependencies mismatch")
            got = {}
            for dep in dependencies[0]:
                if dep.tag.split("}")[-1] != "dependency" or dep.get("id") in got:
                    raise ValidationError("unexpected dependency structure")
                got[dep.get("id")] = dep.get("version")
            # NuGet may spell a bare minimum as its equivalent range.
            if set(got) != set(render_packages.COMPONENTS) or any(
                    v not in (version, f"[{version}, )", f"[{version},)") for v in got.values()):
                raise ValidationError("convenience dependencies mismatch")
        elif dependencies:
            raise ValidationError("component must have no dependencies")
        expected_tools = {p.relative_to(source).as_posix(): p.read_bytes()
                          for p in (source / "tools").rglob("*") if p.is_file()}
        actual_tools = {n for n in names if n.startswith("tools/") and not n.endswith("/")}
        if actual_tools != set(expected_tools):
            raise ValidationError("packed tools manifest mismatch")
        for name, data in expected_tools.items():
            if archive.read(name) != data:
                raise ValidationError(f"packed payload mismatch: {name}")
        # License bytes are already compared with rendered sources above. Those
        # sources come from the digest-verified release ZIP, not this tooling ref.
        for name in names:
            if name in expected_tools or name.endswith("/"):
                continue
            if (name in (f"{package_id}.nuspec", "[Content_Types].xml", "_rels/.rels")
                    or (name.startswith("package/services/metadata/core-properties/")
                        and name.endswith(".psmdcp"))):
                continue
            raise ValidationError(f"unexpected package member: {name}")


def build(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True)
    parser.add_argument("--archive", required=True)
    parser.add_argument("--sha256", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args(argv)
    version = render_packages.parse_version(args.version)
    choco = shutil.which("choco")
    if not choco:
        raise ValidationError("choco is required; packing cannot be skipped")
    output = Path(args.output).resolve()
    if output.exists():
        raise ValidationError(f"output already exists: {output}")
    output.parent.mkdir(parents=True, exist_ok=True)
    staging = Path(tempfile.mkdtemp(prefix=".cercano-pack-", dir=output.parent))
    try:
        rendered = render_packages.build([
            "--version", version, "--archive", str(Path(args.archive).resolve()),
            "--sha256", args.sha256, "--output", str(staging / "rendered")])
        packages = staging / "packages"
        packages.mkdir()
        for package_id in PACKAGE_IDS:
            subprocess.run([choco, "pack", str(rendered / package_id / f"{package_id}.nuspec"),
                            "--out", str(packages), "--limit-output", "--no-progress"],
                           check=True)
        expected = {f"{p}.{version}.nupkg" for p in PACKAGE_IDS}
        if {p.name for p in packages.iterdir()} != expected:
            raise ValidationError("expected exactly three packed packages")
        for package_id in PACKAGE_IDS:
            package = packages / f"{package_id}.{version}.nupkg"
            inspect_package(package, rendered, package_id, version)
        # No checksum sidecar is written until every package passes inspection.
        for package in sorted(packages.iterdir()):
            digest = hashlib.sha256(package.read_bytes()).hexdigest()
            package.with_name(package.name + ".sha256").write_text(
                f"{digest}  {package.name}\n", encoding="ascii")
        os.rename(staging, output)
    finally:
        if staging.exists():
            shutil.rmtree(staging)
    return output


def main(argv=None):
    try:
        output = build(argv)
    except (ValidationError, subprocess.CalledProcessError, OSError, ValueError,
            zipfile.BadZipFile, KeyError, ET.ParseError) as error:
        print(f"Error: {error}", file=sys.stderr)
        return 1
    print(f"Packed and inspected three packages: {output / 'packages'} (not published)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
