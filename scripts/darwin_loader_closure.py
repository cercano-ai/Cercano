#!/usr/bin/env python3
"""Opt-in offline staged closure check from caller-supplied thin otool -l reports.

Does not invoke binaries/tools, discover dynamic loads, or attest report freshness.
"""
import argparse
import json
import os
import re
import sys
from pathlib import Path

SYSTEM = frozenset({
    '/usr/lib/libSystem.B.dylib', '/usr/lib/libc++.1.dylib',
    '/usr/lib/libobjc.A.dylib',
    *('/System/Library/Frameworks/' + name + '.framework/Versions/' + version + '/' + name
      for name, version in [('CoreFoundation', 'A'), ('Security', 'A'),
                            ('Accelerate', 'A'), ('Foundation', 'C'),
                            ('Metal', 'A'), ('MetalKit', 'A')]),
})
LOADS = {'LC_LOAD_DYLIB', 'LC_LOAD_WEAK_DYLIB', 'LC_REEXPORT_DYLIB',
         'LC_LOAD_UPWARD_DYLIB', 'LC_LAZY_LOAD_DYLIB'}
# Explicitly reviewed commands with no dependency/runpath semantics. Do not
# silently accept new LC_* names: review their loader effects before adding them.
NONDEPENDENCY = {
    'LC_SEGMENT', 'LC_SEGMENT_64', 'LC_SYMTAB', 'LC_DYSYMTAB',
    'LC_UUID', 'LC_CODE_SIGNATURE', 'LC_SEGMENT_SPLIT_INFO',
    'LC_FUNCTION_STARTS', 'LC_DATA_IN_CODE', 'LC_DYLIB_CODE_SIGN_DRS',
    'LC_LINKER_OPTIMIZATION_HINT', 'LC_DYLD_INFO', 'LC_DYLD_INFO_ONLY',
    'LC_DYLD_EXPORTS_TRIE', 'LC_DYLD_CHAINED_FIXUPS',
    'LC_VERSION_MIN_MACOSX', 'LC_VERSION_MIN_IPHONEOS',
    'LC_VERSION_MIN_TVOS', 'LC_VERSION_MIN_WATCHOS', 'LC_BUILD_VERSION',
    'LC_SOURCE_VERSION', 'LC_MAIN', 'LC_THREAD', 'LC_UNIXTHREAD',
    'LC_TWOLEVEL_HINTS', 'LC_ENCRYPTION_INFO', 'LC_ENCRYPTION_INFO_64',
}
STRING_COMMANDS = LOADS | {'LC_ID_DYLIB', 'LC_LOAD_DYLINKER', 'LC_RPATH'}


class Failure(ValueError):
    pass


def require(ok, message):
    if not ok:
        raise Failure(message)


def parse_report(text):
    """Parse one thin image's -l output, not -L (which includes its own ID)."""
    require(isinstance(text, str), 'report must be text')
    parts = re.split(r'^Load command (\d+)\s*$', text, flags=re.M)
    require(len(parts) > 1 and len(parts[0].strip().splitlines()) == 1
            and parts[0].strip().endswith(':'), 'expected one thin otool -l report')
    dependencies, rpaths = [], []
    for index in range(1, len(parts), 2):
        require(int(parts[index]) == (index - 1) // 2, 'nonsequential load commands')
        block = parts[index + 1]
        commands = re.findall(r'^\s*cmd\b([^\n]*)$', block, re.M)
        require(len(commands) == 1 and re.fullmatch(r'\s+LC_\w+\s*', commands[0]),
                'missing/ambiguous/malformed load command')
        cmd = commands[0].strip()
        require(cmd in STRING_COMMANDS | NONDEPENDENCY,
                f'unsupported load command: {cmd}; review loader semantics before accepting it')
        # Native otool leaves Section markers and dylib version fields
        # unindented. Recognize only their exact syntax in relevant commands;
        # all other unindented payload remains fail-closed.
        require(all(not line or line[0].isspace()
                    or (line == 'Section' and cmd in {'LC_SEGMENT', 'LC_SEGMENT_64'})
                    or (cmd in LOADS | {'LC_ID_DYLIB'} and re.fullmatch(
                        r'(?:current|compatibility) version [0-9]+\.[0-9]+\.[0-9]+', line))
                    for line in block.splitlines()),
                'unexpected report header or diagnostic')
        sizes = re.findall(r'^\s*cmdsize\b([^\n]*)$', block, re.M)
        require(len(sizes) == 1 and re.fullmatch(r'\s+[0-9]+\s*', sizes[0]),
                f'missing/ambiguous/malformed cmdsize: {cmd}')
        size = int(sizes[0])
        require(size >= 8 and size % 4 == 0, f'invalid cmdsize: {cmd}: {size}')
        # Validate the common load_command envelope, not every command payload.
        if cmd in STRING_COMMANDS:
            field = 'path' if cmd == 'LC_RPATH' else 'name'
            values = re.findall(r'^\s*' + field + r' (.+) \(offset \d+\)\s*$', block, re.M)
            require(len(values) == 1, f'missing/ambiguous {field}: {cmd}')
            value = values[0]
            if cmd in LOADS:
                dependencies.append(value)
            elif cmd == 'LC_RPATH':
                rpaths.append(value)
            elif cmd == 'LC_LOAD_DYLINKER':
                require(value == '/usr/lib/dyld', 'unsupported dynamic linker')
            # LC_ID_DYLIB is identity, never a dependency edge.
    return dependencies, rpaths


def inventory_members(root, executable, backends, names):
    """Shared staged member/alias validation for offline and fresh collection."""
    root = Path(root).resolve(strict=True)
    require(root.is_dir(), 'staged root must be a directory')
    require(isinstance(backends, list) and backends,
            'explicit nonempty backend roots required')
    def inside(path):
        # Check both lexical traversal and symlink resolution, including missing targets.
        require(Path(os.path.abspath(path)).is_relative_to(root), f'path escapes staged root: {path}')
        resolved = path.resolve()
        require(resolved.is_relative_to(root), f'symlink escapes staged root: {path}')
        return resolved

    def member(value):
        require(isinstance(value, str) and value and not Path(value).is_absolute()
                and all(p not in ('.', '..', '') for p in value.split('/')),
                f'invalid inventory member: {value!r}')
        return inside(root / value)

    images = {}
    for name in names:
        image = member(name)
        require(image.is_file(), f'missing reported image: {name}')
        require(image not in images.values(), f'ambiguous report alias: {name}')
        images[name] = image
    executable = member(executable)
    backends = [member(name) for name in backends]
    require(len(set(backends)) == len(backends), 'duplicate backend roots')
    require(executable in images.values(), 'missing report for executable')
    require(all(p in images.values() for p in backends), 'missing report for backend')
    return root, images, executable, backends


def validate(root, inventory):
    require(isinstance(inventory, dict) and set(inventory) == {'executable', 'backends', 'reports'},
            'expected executable, backends and reports inventory')
    require(isinstance(inventory['reports'], dict), 'reports must be an object')
    root, members, executable, backends = inventory_members(
        root, inventory['executable'], inventory['backends'], inventory['reports'])
    reports = {members[name]: parse_report(text) for name, text in inventory['reports'].items()}

    def inside(path):
        require(Path(os.path.abspath(path)).is_relative_to(root), f'path escapes staged root: {path}')
        resolved = path.resolve()
        require(resolved.is_relative_to(root), f'symlink escapes staged root: {path}')
        return resolved

    def expand(value, image):
        for token, base in [('@loader_path', image.parent),
                            ('@executable_path', executable.parent)]:
            if value == token:
                return inside(base)
            if value.startswith(token + '/'):
                return inside(base / value[len(token) + 1:])
        require(False, f'unsupported/external loader path: {value}')

    visited = set()
    reached = set()

    def walk(image, inherited, chain):
        require(image.is_file(), f'missing library/backend: {image}')
        require(image in reports, f'missing report for library/backend: {image}')
        deps, raw_paths = reports[image]
        paths = tuple(dict.fromkeys([expand(p, image) for p in raw_paths] + list(inherited)))
        state = (image, paths)
        if state in visited:
            return
        visited.add(state)
        reached.add(image)
        for dep in deps:
            if dep in SYSTEM:
                continue
            if dep.startswith('@rpath/'):
                candidates = {inside(p / dep[len('@rpath/'):]) for p in paths}
                existing = {p for p in candidates if p.is_file()}
                require(len(existing) == 1, f'unresolved/ambiguous dependency: {dep} in {image}')
                target = existing.pop()
            else:
                target = expand(dep, image)
            if target not in chain:
                walk(target, paths, chain | {image})

    walk(executable, (), set())
    # Explicit dlopen roots use the executable context, not guessed discovery paths.
    executable_paths = tuple(expand(p, executable) for p in reports[executable][1])
    for backend in backends:
        walk(backend, executable_paths, set())
    require(reached == set(reports), 'unreachable reports: inventory is not the declared closure')
    return sorted(str(p.relative_to(root)) for p in reached)


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result, f'duplicate JSON key: {key}')
        result[key] = value
    return result


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument('--staged-root', type=Path, required=True)
    parser.add_argument('--loader-reports', type=Path, required=True)
    args = parser.parse_args(argv)
    try:
        inventory = json.loads(args.loader_reports.read_text(), object_pairs_hook=unique_object)
        images = validate(args.staged_root, inventory)
    except (ValueError, OSError, RuntimeError) as error:
        print(f'FAILED: {error}', file=sys.stderr)
        return 1
    print(f'Offline supplied-report closure passed: {len(images)} staged images.')
    print('NOT verified: report freshness, dynamic discovery/resources, source identity, signing or runtime qualification.')
    return 0


if __name__ == '__main__':
    sys.exit(main())
