#!/usr/bin/env python3
"""Run only --version from inspected nupkg payloads in disposable user directories.

No Chocolatey install or agent startup. This is native Windows CI evidence only;
the package inspector must have succeeded before calling this helper.
"""
import argparse
import os
from pathlib import Path
import subprocess
import tempfile
import zipfile

import render_packages


def check(version, packages, package_version=None):
    version = render_packages.parse_version(version)
    package_version = render_packages.parse_package_version(package_version or version, version)
    with tempfile.TemporaryDirectory(prefix="cercano-package-smoke-") as temporary:
        root = Path(temporary)
        env = os.environ.copy()
        for key in ("HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "TEMP", "TMP"):
            directory = root / key
            directory.mkdir()
            env[key] = str(directory)
        run = root / "run"
        run.mkdir()
        for package_id, exe in render_packages.COMPONENTS.items():
            package = Path(packages) / f"{package_id}.{package_version}.nupkg"
            with zipfile.ZipFile(package) as archive:
                binary = run / exe
                binary.write_bytes(archive.read(f"tools/{exe}"))
            expected = f"{exe.removesuffix('.exe')} v{version}"
            result = subprocess.run([str(binary), "--version"], cwd=run, env=env,
                                    capture_output=True, text=True, check=True, timeout=30)
            if result.stdout.strip() != expected:
                raise ValueError(f"{exe} does not report the requested release version")
        for key in ("HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA"):
            if any((root / key).rglob("*")):
                raise ValueError("version smoke check wrote user state")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True)
    parser.add_argument("--package-version")
    parser.add_argument("--packages", required=True)
    args = parser.parse_args()
    check(args.version, args.packages, args.package_version)
