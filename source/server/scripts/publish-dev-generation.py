#!/usr/bin/env python3
"""Build-only, opt-in immutable development generations; never a launcher.

Usage:
  python3 source/server/scripts/publish-dev-generation.py publish \
      --root '/absolute/dedicated dev generations' --generation review-001

Requires Python 3.9+ and Go. Builds source/server ./cmd/cercano/ and
source/clients/cli . into ONE temporary sibling directory under --root, validates
both outputs without executing them, writes manifest.json, then atomically
renames with no replacement (macOS/Linux only; other platforms fail closed).
--go accepts an absolute build-tool path (also useful for fixture tests).
--sign explicitly opts into the existing conditional signing helper; otherwise
no Keychain/signing tools are accessed. VERSION overrides the Git-derived version.

Use a dedicated, caller-owned root, not an installation directory. ~/bin and
.cercano-libexec are prohibited. Published generations are read-only by convention,
not tamper-proof: owners can change permissions. No published generation is ever
deleted, overwritten, selected as current, or launched by this command. Failed
unpublished staging directories are removed. There is no runtime PATH lookup,
launcher replacement, process cleanup, service, or automatic retention policy.

Validation is structural (native executable header, executable mode, hashes), not
a native launch/Keychain/CLI-agent compatibility or runtime isolation acceptance
test. Explicit binary paths alone do NOT isolate live config, data, or listeners.
Do not infer that running these binaries against user data is safe. This command
neither runs the app nor configures a safe runtime. Normal launcher is unchanged.
"""

import argparse
import ctypes
import errno
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import sys
import tempfile
import uuid


TARGETS = (("cercano", "source/server", "./cmd/cercano/"),
           ("cercano-cli", "source/clients/cli", "."))


def rename_exclusive(source, destination):
    """An existence check + os.rename is NOT sufficient (empty-dir race)."""
    libc = ctypes.CDLL(None, use_errno=True)
    if sys.platform == "darwin":
        rename = libc.renamex_np
        rename.argtypes = [ctypes.c_char_p, ctypes.c_char_p, ctypes.c_uint]
        args = (os.fsencode(source), os.fsencode(destination), 4)  # RENAME_EXCL
    elif sys.platform.startswith("linux") and hasattr(libc, "renameat2"):
        rename = libc.renameat2
        rename.argtypes = [ctypes.c_int, ctypes.c_char_p, ctypes.c_int,
                           ctypes.c_char_p, ctypes.c_uint]
        args = (-100, os.fsencode(source), -100, os.fsencode(destination), 1)
    else:
        raise RuntimeError("atomic no-replace directory rename unavailable")
    rename.restype = ctypes.c_int
    if rename(*args) != 0:
        error = ctypes.get_errno()
        if error in (errno.EEXIST, errno.ENOTEMPTY):
            raise FileExistsError(f"generation already exists: {destination}")
        raise OSError(error, os.strerror(error), str(destination))


def git(repo, *args):
    return subprocess.check_output(["git", "-C", str(repo), *args], text=True).strip()


def validate(path):
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or not info.st_mode & 0o111 or info.st_size < 64:
        raise ValueError(f"not a nonempty regular executable: {path}")
    # Thin/fat Mach-O and ELF. No --version or other app invocation, even in tests.
    with path.open("rb") as stream:
        magic = stream.read(4)
    if magic not in (b"\x7fELF", b"\xfe\xed\xfa\xce", b"\xce\xfa\xed\xfe",
                     b"\xfe\xed\xfa\xcf", b"\xcf\xfa\xed\xfe",
                     b"\xca\xfe\xba\xbe", b"\xbe\xba\xfe\xca",
                     b"\xca\xfe\xba\xbf", b"\xbf\xba\xfe\xca"):
        raise ValueError(f"unrecognized native executable header: {path}")
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return {"sha256": digest.hexdigest(), "size": info.st_size}


def publish(args):
    root_arg = Path(args.root)
    if not root_arg.is_absolute():
        raise ValueError("--root must be an explicit absolute path")
    root = root_arg.resolve()
    home_bin = (Path.home() / "bin").resolve()
    if root == home_bin or home_bin in root.parents or ".cercano-libexec" in root.parts:
        raise ValueError("installation paths (~/bin or .cercano-libexec) are prohibited")
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]{0,127}", args.generation):
        raise ValueError("generation must be a single safe name (1-128 characters)")
    repo = Path(args.repo).resolve()
    for _, module, _ in TARGETS:
        if not (repo / module / "go.mod").is_file():
            raise ValueError(f"missing module: {module}")
    go = args.go or shutil.which("go")
    if not go or not Path(go).is_absolute():
        raise ValueError("Go build tool must resolve to an absolute path")
    go = str(Path(go).resolve())
    root.mkdir(parents=True, exist_ok=True)
    destination = root / args.generation
    if os.path.lexists(destination):
        raise FileExistsError(f"generation already exists: {destination}")
    commit = git(repo, "rev-parse", "HEAD")
    dirty = bool(git(repo, "status", "--porcelain", "--untracked-files=normal"))
    version = os.environ.get("VERSION") or git(repo, "describe", "--tags", "--always", "--dirty").removeprefix("v")
    # Go's ldflags parser splits whitespace, so reject ambiguous injected values.
    if not re.fullmatch(r"[A-Za-z0-9._+/-]+", version):
        raise ValueError("VERSION must contain only letters, digits, . _ + / -")
    toolchain = subprocess.check_output([go, "version"], text=True).strip()
    stage = Path(tempfile.mkdtemp(prefix=".building-", dir=root))
    try:
        manifest = {"schema": 1, "generation": args.generation,
                    "build_id": str(uuid.uuid4()), "source_commit": commit,
                    "source_dirty": dirty, "version": version, "repo": str(repo),
                    "go_tool": go, "go_version": toolchain, "sign_requested": args.sign,
                    "validation": "structural-only; binaries not executed", "binaries": {}}
        for name, module, package in TARGETS:
            output = stage / name
            command = [go, "build", "-ldflags", f"-X main.version={version}",
                       "-o", str(output), package]
            subprocess.run(command, cwd=repo / module, check=True)
            if args.sign:
                subprocess.run([str(repo / "source/server/scripts/codesign-if-available.sh"),
                                str(output), name], check=True)
            manifest["binaries"][name] = {**validate(output), "module": module,
                                            "package": package, "command": command}
        if set(p.name for p in stage.iterdir()) != {t[0] for t in TARGETS}:
            raise ValueError("unexpected build output in generation")
        # Validate the complete pair again after all build/sign commands finish.
        for name, _, _ in TARGETS:
            if validate(stage / name)["sha256"] != manifest["binaries"][name]["sha256"]:
                raise ValueError(f"output changed during build: {name}")
        (stage / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
        for child in stage.iterdir():
            child.chmod(0o444 if child.name == "manifest.json" else 0o555)
        stage.chmod(0o555)
        rename_exclusive(stage, destination)
    finally:
        # Only our unpublished scratch directory, never destination or old generations.
        if stage.exists():
            stage.chmod(0o700)
            shutil.rmtree(stage)
    print(destination)


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    commands = parser.add_subparsers(dest="command", required=True)
    command = commands.add_parser("publish", help="build and publish; never launch")
    command.add_argument("--root", required=True, help="absolute dedicated generation root")
    command.add_argument("--generation", required=True, help="new immutable directory name")
    command.add_argument("--repo", default=str(Path(__file__).resolve().parents[3]))
    command.add_argument("--go", help="absolute Go build-tool path; defaults to build-time discovery")
    command.add_argument("--sign", action="store_true", help="opt into conditional signing helper")
    args = parser.parse_args()
    try:
        publish(args)
    except (OSError, ValueError, RuntimeError, subprocess.CalledProcessError) as error:
        print(f"publication failed: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
