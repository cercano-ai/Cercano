#!/usr/bin/env python3
"""Prepare an isolated full-set review commit; native acceptance remains gated."""

import argparse
import os
import tempfile
import formula_set as formulas
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


def stage_reviewed_set(tap_path, desired, version, digest):
    """Build an unreferenced commit using a private index; never touch user work.

    The returned commit can be reviewed then explicitly fast-forwarded by the
    operator. Object creation is not publication, and does not advance a ref.
    """
    def git(*args, data=None, env=None):
        result = subprocess.run(['git', '-C', str(tap_path), *args], input=data,
                                capture_output=True, env=env)
        if result.returncode:
            raise PromotionError('Local Git operation refused: ' + args[0])
        return result.stdout.decode().strip()

    head = git('rev-parse', '--verify', 'HEAD')
    existing = {}
    for name in formulas.NAMES:
        path = 'Formula/' + name
        entry = git('ls-tree', head, '--', path)
        if entry:
            if not entry.startswith('100644 blob '):
                raise PromotionError('Formula is not an ordinary file')
            # Preserve the final newline, unlike metadata commands.
            blob = git('rev-parse', head + ':' + path)
            text = subprocess.check_output(['git', '-C', str(tap_path), 'cat-file', 'blob', blob])
            existing[name] = text.decode()
    if formulas.decision(existing, desired, version, digest) == 'unchanged':
        return head, False, ['Exact set already committed; no changes.']
    with tempfile.TemporaryDirectory(prefix='cercano-tap-index-') as temporary:
        env = dict(os.environ, GIT_INDEX_FILE=str(Path(temporary) / 'index'))
        git('read-tree', head, env=env)
        for name in formulas.NAMES:
            blob = git('hash-object', '-w', '--stdin', data=desired[name].encode())
            git('update-index', '--add', '--cacheinfo', '100644', blob,
                'Formula/' + name, env=env)
        tree = git('write-tree', env=env)
        commit = git('commit-tree', tree, '-p', head,
                     data=('chore: publish Cercano ' + version + '\n').encode())
    return commit, True, ['Unreferenced review commit; checkout, index and refs unchanged.']


def promote(formula_path, tap_path, expected_version, skip_download=False, archive=None):
    try:
        formulas.require_acceptance()
    except render_formula.ValidationError as error:
        raise PromotionError('Homebrew promotion is disabled: native/runtime acceptance pending') from error
    if skip_download or archive is None:
        raise PromotionError('Verified local and public archives are required')
    # Import here to avoid the legacy field-helper import cycle.
    import update_tap
    formula = Path(formula_path)
    metadata = formulas.fields(formula.read_text())
    version = expected_version or metadata['version']
    try:
        desired = update_tap.prepare(version, metadata['sha256'], Path(archive), formula)
        update_tap.published(version, Path(archive).name, metadata['sha256'])
    except update_tap.Failure as error:
        raise PromotionError(str(error)) from None
    return stage_reviewed_set(tap_path, desired, version, metadata['sha256'])


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("formula", help="rendered cercano.rb from render_formula.py")
    parser.add_argument("tap", help="local checkout of cercano-ai/homebrew-cercano")
    parser.add_argument("--version", help="expected version, as a cross-check")
    parser.add_argument("--archive", type=Path, help="verified coordinated local archive")
    parser.add_argument("--skip-download", action="store_true",
                        help="offline; records the artifact as UNVERIFIED instead of checking it")
    args = parser.parse_args(argv)

    try:
        destination, replaced, notes = promote(
            args.formula, args.tap, args.version, args.skip_download, args.archive)
    except (PromotionError, render_formula.ValidationError, OSError) as error:
        print(f"REFUSED: {error}", file=sys.stderr)
        print("The tap checkout was not modified.", file=sys.stderr)
        return 1

    for note in notes:
        print(note)
    print(f"{'Updated' if replaced else 'Created'} {destination}")
    print("No branch was advanced or pushed. Review the returned commit before explicit fast-forward publication.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
