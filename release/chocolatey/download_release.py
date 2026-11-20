#!/usr/bin/env python3
"""Download a public Windows release ZIP; trust only the caller's independent digest."""
import argparse
import hashlib
import os
from pathlib import Path
import sys
import tempfile
import time
import urllib.request

import render_packages

# Reuse the tap's public-only HTTPS redirect policy, without API credentials.
sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "homebrew"))
from update_tap import PublicRedirect, public_url, Failure, MAX_ARCHIVE


def download(version, expected, directory):
    version = render_packages.parse_version(version)
    expected = render_packages.parse_digest(expected)
    name = f"cercano-{version}-windows-x64.zip"
    url = render_packages.URL_TEMPLATE.format(version=version, name=name)
    public_url(url)
    directory = Path(directory)
    directory.mkdir(parents=True, exist_ok=True)
    target = directory / name
    if target.exists():
        raise Failure("download target already exists")
    digest, size = hashlib.sha256(), 0
    deadline = time.monotonic() + 300
    fd, temporary = tempfile.mkstemp(prefix=".download-", dir=directory)
    try:
        with os.fdopen(fd, "wb") as out:
            with urllib.request.build_opener(PublicRedirect()).open(url, timeout=30) as response:
                public_url(response.geturl())
                if response.status != 200:
                    raise Failure("public download must return HTTP 200")
                while chunk := response.read1(64 * 1024):
                    size += len(chunk)
                    if size > MAX_ARCHIVE or time.monotonic() > deadline:
                        raise Failure("download exceeds size or time limit")
                    digest.update(chunk)
                    out.write(chunk)
        if not size or digest.hexdigest() != expected:
            raise Failure("archive sha256 mismatch or empty download")
        os.rename(temporary, target)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)
    return target


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True)
    parser.add_argument("--sha256", required=True)
    parser.add_argument("--directory", required=True)
    args = parser.parse_args()
    try:
        print(download(args.version, args.sha256, args.directory))
    except (Failure, render_packages.ValidationError, OSError, ValueError) as error:
        print(f"Error: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
