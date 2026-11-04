#!/usr/bin/env python3
"""Legacy local tap promotion: disabled for the incomplete Homebrew split.

promote() refuses even offline; a single formula is not a coherent release set.
Validation helpers remain for future review. See README.md.
"""

import argparse
import hashlib
import ipaddress
import re
import shutil
import subprocess
import sys
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import render_formula  # noqa: E402

DOWNLOAD_TIMEOUT = 300
MAX_ARCHIVE_BYTES = 512 * 1024 * 1024


class PromotionError(Exception):
    """Promotion was refused; the tap checkout is left unchanged."""


def read_formula_fields(text):
    fields = {}
    for key in ("url", "version", "sha256"):
        match = re.search(rf'^\s*{key}\s+"([^"]+)"\s*$', text, re.M)
        if not match:
            raise PromotionError(f"formula has no {key} field")
        fields[key] = match.group(1)
    leftover = re.findall(r"@[A-Z0-9_]+@", text)
    if leftover:
        raise PromotionError(
            f"formula still contains placeholders: {', '.join(sorted(set(leftover)))}")
    return fields


def is_loopback(url):
    host = urllib.parse.urlparse(url).hostname or ""
    try:
        return ipaddress.ip_address(host).is_loopback
    except ValueError:
        return host == "localhost"


def verify_published_artifact(url, expected_sha256):
    """Download the artifact from its published URL and check its digest.

    Plain HTTP is permitted only for loopback, so tests can exercise the real
    download path; a published release URL must always be HTTPS.
    """
    if not url.startswith("https://") and not (url.startswith("http://") and is_loopback(url)):
        raise PromotionError(f"refusing a non-HTTPS release URL: {url}")
    digest = hashlib.sha256()
    total = 0
    try:
        with urllib.request.urlopen(url, timeout=DOWNLOAD_TIMEOUT) as response:
            if getattr(response, "status", 200) != 200:
                raise PromotionError(f"release artifact is not available: HTTP {response.status}")
            while chunk := response.read(1024 * 1024):
                total += len(chunk)
                if total > MAX_ARCHIVE_BYTES:
                    raise PromotionError("release artifact is implausibly large; refusing")
                digest.update(chunk)
    except urllib.error.HTTPError as error:
        raise PromotionError(
            f"release artifact is not available at {url}: HTTP {error.code}") from error
    except urllib.error.URLError as error:
        raise PromotionError(f"cannot reach the release artifact: {error.reason}") from error
    if total == 0:
        raise PromotionError("release artifact is empty")
    actual = digest.hexdigest()
    if actual != expected_sha256:
        raise PromotionError(
            f"published artifact does not match the formula: formula claims {expected_sha256}, "
            f"download is {actual}")
    return total


def check_ruby_syntax(path):
    if shutil.which("ruby") is None:
        return "ruby not installed; formula syntax was NOT checked"
    result = subprocess.run(["ruby", "-c", str(path)], capture_output=True, text=True)
    if result.returncode != 0:
        raise PromotionError(f"formula is not valid Ruby:\n{result.stderr.strip()}")
    return None


def promote(formula_path, tap_path, expected_version, skip_download=False):
    raise PromotionError("Homebrew split promotion is disabled pending coordinated publication and native runtime/migration acceptance")


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("formula", help="rendered cercano.rb from render_formula.py")
    parser.add_argument("tap", help="local checkout of cercano-ai/homebrew-cercano")
    parser.add_argument("--version", help="expected version, as a cross-check")
    parser.add_argument("--skip-download", action="store_true",
                        help="offline; records the artifact as UNVERIFIED instead of checking it")
    args = parser.parse_args(argv)

    try:
        destination, replaced, notes = promote(
            args.formula, args.tap, args.version, args.skip_download)
    except (PromotionError, render_formula.ValidationError, OSError) as error:
        print(f"REFUSED: {error}", file=sys.stderr)
        print("The tap checkout was not modified.", file=sys.stderr)
        return 1

    for note in notes:
        print(note)
    print(f"{'Updated' if replaced else 'Created'} {destination}")
    print("Nothing was committed or pushed. Review the diff, then publish deliberately.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
