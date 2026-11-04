#!/usr/bin/env python3
"""Opt-in fresh Darwin staged-image collection and loader closure check.

Requires a caller-owned stable stage; not safe against hostile concurrent writes.
Never executes staged payloads. Reports are ephemeral, not provenance evidence.
"""
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys

import darwin_loader_closure as closure

OTOOL = '/usr/bin/otool'
TIMEOUT = 10
MAX_OUTPUT = 4 * 1024 * 1024
ARCHITECTURES = {'arm64': 'ARM64', 'x86_64': 'X86_64'}


def host_tool(command):
    """Only the fixed host inspector is invoked, with no staged environment/code."""
    try:
        result = subprocess.run(command, capture_output=True, timeout=TIMEOUT,
                                env={'PATH': '/usr/bin:/bin', 'LC_ALL': 'C'})
    except subprocess.TimeoutExpired as error:
        raise closure.Failure(f'host tool timed out after {TIMEOUT}s: {command[1]}') from error
    except OSError as error:
        raise closure.Failure(f'cannot run host otool: {error}') from error
    closure.require(result.returncode == 0,
                    f'host otool failed ({result.returncode}): {result.stderr[:500]!r}')
    closure.require(not result.stderr, f'host otool diagnostic: {result.stderr[:500]!r}')
    closure.require(len(result.stdout) <= MAX_OUTPUT, 'host otool output too large')
    try:
        return result.stdout.decode('utf-8', errors='strict')
    except UnicodeError as error:
        raise closure.Failure('host otool output is not UTF-8') from error


def digest(path):
    value = hashlib.sha256()
    with path.open('rb') as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b''):
            value.update(block)
    return value.digest()


def identity(text, architecture, executable):
    # otool -hv emits one pathname, one table title, one column line and one row
    # for a thin image. Reject multiple slices, diagnostics, unknown CPUs/types.
    lines = text.strip().splitlines()
    closure.require(len(lines) == 4 and lines[0].endswith(':')
                    and lines[1].strip() == 'Mach header'
                    and lines[2].split() == ['magic', 'cputype', 'cpusubtype', 'caps',
                                             'filetype', 'ncmds', 'sizeofcmds', 'flags'],
                    'expected one thin Mach-O header from otool -hv')
    fields = lines[3].split()
    closure.require(len(fields) >= 7 and fields[0] == 'MH_MAGIC_64'
                    and fields[1] == ARCHITECTURES[architecture],
                    'unsupported Mach-O identity/architecture')
    closure.require(fields[4] in ({'EXECUTE'} if executable else {'DYLIB', 'BUNDLE'}),
                    'unsupported Mach-O file type for inventory role')
    closure.require(re.fullmatch(r'[0-9]+', fields[5]) and int(fields[5]) > 0
                    and re.fullmatch(r'[0-9]+', fields[6]), 'invalid Mach-O header counts')


def collect(root, inventory, architecture, run_tool=host_tool):
    """Collect in memory, check closure, recheck every image before returning.

    run_tool is a test seam, not a CLI option to select arbitrary tooling.
    No persistent hash/report schema or source-identity claim is introduced.
    """
    closure.require(architecture in ARCHITECTURES, 'unsupported architecture')
    closure.require(isinstance(inventory, dict)
                    and set(inventory) == {'executable', 'backends', 'images'},
                    'expected executable, backends and images inventory')
    closure.require(isinstance(inventory['images'], list) and inventory['images'],
                    'explicit nonempty image inventory required')
    root, members, executable, backends = closure.inventory_members(
        root, inventory['executable'], inventory['backends'], inventory['images'])
    # Snapshot all members first, so changes to not-yet-inspected images also fail.
    before = {name: digest(path) for name, path in members.items()}
    reports = {}
    for name, path in members.items():
        with path.open('rb') as stream:
            closure.require(stream.read(4) == b'\xcf\xfa\xed\xfe',
                            f'unsupported/fat/non-Mach-O image: {name}')
        header = run_tool([OTOOL, '-hv', str(path)])
        identity(header, architecture, path == executable)
        reports[name] = run_tool([OTOOL, '-l', str(path)])
        closure.require(digest(path) == before[name], f'image changed during collection: {name}')
    supplied = {'executable': inventory['executable'], 'backends': inventory['backends'],
                'reports': reports}
    images = closure.validate(root, supplied)
    # Re-resolve aliases as well as hashing all bytes after closure validation.
    _, final_members, final_executable, final_backends = closure.inventory_members(
        root, inventory['executable'], inventory['backends'], inventory['images'])
    closure.require(final_members == members and final_executable == executable
                    and final_backends == backends, 'image paths changed during collection')
    for name, path in final_members.items():
        closure.require(digest(path) == before[name], f'image changed by end of collection: {name}')
    return images


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument('--staged-root', type=Path, required=True)
    parser.add_argument('--image-inventory', type=Path, required=True)
    parser.add_argument('--architecture', choices=sorted(ARCHITECTURES), required=True)
    args = parser.parse_args(argv)
    try:
        inventory = json.loads(args.image_inventory.read_text(), object_pairs_hook=closure.unique_object)
        images = collect(args.staged_root, inventory, args.architecture)
    except (ValueError, OSError, RuntimeError) as error:
        print(f'FAILED: {error}', file=sys.stderr)
        return 1
    print(f'Fresh staged-image supplied-inventory closure passed: {len(images)} images.')
    print('Requires caller-owned stable stage; NOT hostile-concurrency safe.')
    print('NOT verified: dynamic discovery/resources, source provenance, signing or runtime qualification.')
    return 0


if __name__ == '__main__':
    sys.exit(main())
