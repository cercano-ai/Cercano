"""Build-time payload inventory, not authenticity or dependency-closure proof.

Schema 1 excludes the inventory itself (no recursive hash). Paths are relative
 to the archive's top directory. Executable flags describe archived modes,
including Windows ZIP Unix attributes, not the build host's permissions.
"""
import hashlib
import json
import re
import stat
import sys
import tarfile
import zipfile
from pathlib import Path

NAME = "runtime-inventory.json"


def safe_path(path):
    if (not isinstance(path, str) or not path or "\\" in path or ":" in path
            or any(ord(c) < 32 for c in path)
            or any(p in ("", ".", "..") for p in path.split("/"))):
        raise ValueError(f"unsafe inventory path: {path!r}")
    return path


def encode(version, platform, files):
    """files maps safe relative paths to (bytes, archive mode)."""
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?", version):
        raise ValueError("invalid inventory version")
    if platform not in ("darwin-arm64", "linux-x64", "windows-x64"):
        raise ValueError("invalid inventory platform")
    members = []
    for path, (data, mode) in sorted(files.items()):
        safe_path(path)
        if path == NAME:
            raise ValueError("inventory cannot inventory itself")
        members.append(dict(path=path, sha256=hashlib.sha256(data).hexdigest(),
                            executable=bool(mode & 0o111)))
    return (json.dumps(dict(schema_version=1, version=version, platform=platform,
                            members=members), sort_keys=True, indent=2) + "\n").encode()


def produce(root, version, platform, modes):
    files = {}
    root = Path(root)
    for path, mode in modes.items():
        safe_path(path)
        target = root
        for part in path.split("/"):
            target = target / part
            if target.is_symlink():
                raise ValueError(f"symlink payload: {path}")
        if not stat.S_ISREG(target.stat().st_mode):
            raise ValueError(f"nonregular payload: {path}")
        files[path] = (target.read_bytes(), mode)
    target = root / NAME
    with target.open("xb") as handle:
        handle.write(encode(version, platform, files))
    target.chmod(0o644)


def validate_archive(archive, top, version, platform):
    """No extraction. Reject duplicate/unsafe/link entries and mismatched bytes.

    Exact payload layouts remain the caller's responsibility. Canonical JSON
    comparison also rejects duplicate JSON keys, extra fields and unsorted rows.
    """
    files = {}
    seen = set()
    def add(name, mode, data):
        safe_path(name)
        if name in seen:
            raise ValueError(f"duplicate archive member: {name}")
        seen.add(name)
        if name == top and data is None:
            return
        if not name.startswith(top + "/"):
            raise ValueError("invalid archive root")
        relative = name[len(top) + 1:]
        if data is not None:
            files[relative] = (data, mode)
    if zipfile.is_zipfile(archive):
        with zipfile.ZipFile(archive) as zf:
            for item in zf.infolist():
                mode = item.external_attr >> 16
                if not stat.S_ISREG(mode):
                    raise ValueError("nonregular ZIP member")
                add(item.filename, mode, zf.read(item))
    else:
        with tarfile.open(archive, "r:gz") as tf:
            for item in tf:
                if not (item.isdir() or item.isfile()):
                    raise ValueError("nonregular tar member")
                add(item.name, item.mode, None if item.isdir() else tf.extractfile(item).read())
    inventory, mode = files.pop(NAME, (None, 0))
    if mode & 0o111 or inventory != encode(version, platform, files):
        raise ValueError("runtime inventory mismatch (identity, membership, hash or mode)")


def payload_modes(platform):
    suffix = ".exe" if platform == "windows-x64" else ""
    return {"LICENSE": 0o644, "README.txt": 0o644,
            "bin/cercano" + suffix: 0o755, "bin/cercano-cli" + suffix: 0o755}


if __name__ == "__main__":
    action, path, version, platform, *extra = sys.argv[1:]
    if action == "produce":
        produce(path, version, platform, payload_modes(platform))
    elif action == "validate":
        validate_archive(path, extra[0], version, platform)
    else:
        raise SystemExit("expected produce or validate")
