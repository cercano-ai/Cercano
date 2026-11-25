#!/usr/bin/env python3
"""Prepare an offline three-formula review plan; never modify or publish a tap."""
import argparse
import hashlib
import json
import re
from pathlib import Path
import sys
import tarfile

import render_formula as renderer
from update_tap import Failure, require
from promote_to_tap import PromotionError, read_formula_fields

NAMES = ('cercano.rb', 'cercano-agent.rb', 'cercano-cli.rb')


def decision(existing, desired, version, digest):
    """Offline per-file comparison; not the coordinated publication decision."""
    if existing is None:
        return 'update'
    fields = read_formula_fields(existing)
    old = tuple(map(int, renderer.parse_version(fields['version']).split('.')))
    new = tuple(map(int, version.split('.')))
    require(old <= new, 'Tap already has a newer version; refusing downgrade')
    if old == new:
        require(fields['sha256'] == digest, 'Same version has different checksum; refusing replacement')
        require(existing == desired, 'Same version has different formula; refusing replacement')
        return 'unchanged'
    return 'update'


def sha(content):
    return hashlib.sha256(content).hexdigest()


def prepare(version, digest, archive, formulas, tap, tap_base_sha=None):
    """Validate the whole set before returning any plan. Inputs must be private snapshots.

    Existing text is recorded by hash, not trusted as executable Ruby. Nothing is
    executed, downloaded, installed, written into the tap, committed or pushed.
    """
    require(tap_base_sha is None or re.fullmatch(r'[0-9a-f]{40}', tap_base_sha),
            'Tap base must be a full lowercase commit SHA')
    renderer.parse_version(version)
    renderer.parse_digest(digest)
    require(renderer.hash_archive(archive) == digest, 'Archive checksum mismatch')
    renderer.validate_archive(archive, version)
    require(formulas.is_dir() and not formulas.is_symlink(), 'Formula directory required')
    require(tap.is_dir() and not tap.is_symlink(), 'Tap snapshot directory required')
    directory = tap / 'Formula'
    require(directory.is_dir() and not directory.is_symlink(), 'Tap Formula directory required')
    entries = []
    for name in NAMES:
        source = formulas / name
        require(source.is_file() and not source.is_symlink(), f'Missing or symlink formula: {name}')
        expected = renderer.render(Path(__file__).with_name(name + '.in').read_bytes().decode('utf-8'),
                                   version, archive.name, digest)
        desired_bytes = expected.encode('utf-8')
        source_bytes = source.read_bytes()
        source_bytes.decode('utf-8')  # Strict decoding; never normalize newlines.
        require(source_bytes == desired_bytes, f'Formula differs from trusted template: {name}')
        target = directory / name
        require(not target.is_symlink(), f'Symlink tap target: {name}')
        require(not target.exists() or target.is_file(), f'Non-file tap target: {name}')
        existing_bytes = target.read_bytes() if target.exists() else None
        existing = existing_bytes.decode('utf-8') if existing_bytes is not None else None
        action = decision(existing, expected, version, digest)
        entries.append({'path': 'Formula/' + name, 'action': action,
                        'before_sha256': sha(existing_bytes) if existing_bytes is not None else None,
                        'after_sha256': sha(desired_bytes), 'content': expected})
    return {'schema': 1, 'status': 'review-only-publication-blocked',
            'version': version, 'archive_sha256': digest, 'files': entries,
            'tap_base_sha': tap_base_sha,
            'migration_policy': 'explicit-operator-only; no automatic unlink/overwrite/remove',
            'requirements': [
                'Review all three formulas as one set; dependencies must be available together.',
                'Future authorized publication must use one commit and a compare-and-swap of the reviewed base; never sequential Contents API writes.',
                'Revalidate all inputs and tap base at application time; this plan grants no publication authority.',
                'Complete applicable historical-monolith rehearsal, human/runtime and signed-package acceptance.'
            ]}


class SafeParser(argparse.ArgumentParser):
    def error(self, message):
        self.exit(2, 'Preparation refused: invalid command line.\n')


def main(argv=None):
    parser = SafeParser(description=__doc__)
    parser.add_argument('--version', required=True)
    parser.add_argument('--sha256', required=True)
    parser.add_argument('--archive', type=Path, required=True)
    parser.add_argument('--formulas', type=Path, required=True)
    parser.add_argument('--tap', type=Path, required=True, help='private local tap snapshot (read only)')
    parser.add_argument('--tap-base-sha', help='reviewed snapshot commit (recorded, not authenticated)')
    args = parser.parse_args(argv)
    try:
        plan = prepare(args.version, args.sha256, args.archive, args.formulas, args.tap,
                       args.tap_base_sha)
    except (Failure, PromotionError, renderer.ValidationError, OSError, ValueError, tarfile.TarError):
        print('Preparation refused: invalid or unreadable inputs.', file=sys.stderr)
        return 1
    print(json.dumps(plan, indent=2, sort_keys=True))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
