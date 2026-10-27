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
--sign requires verified ad-hoc codesign on Darwin (no Keychain access); it is
non-applicable on other platforms. VERSION overrides the Git-derived version.

Reuse API (same environment/--repo/--go/--sign for both commands):
  fingerprint [--repo PATH] [--go ABSOLUTE] [--sign]
    prints one SHA-256 token, does not build.
  publish --root ABSOLUTE --generation NAME [--fingerprint TOKEN] [--reuse]
    prints the absolute complete generation path. --reuse validates manifest and
    both hashes, including concurrent winners; without it existing names fail.
Fingerprint covers all source/ bytes, including ignored/untracked inputs, modes,
VERSION, supported architecture flags, Go executable/version and publisher policy.
Ignored outputs are conservatively included (except .git metadata); this can
invalidate caches or enlarge snapshots. No broad build/dist/embed exclusions.
Darwin cgo fingerprints SDK and developer-toolchain contents; other cgo platforms
fail closed pending system-header/library identity support. Trusted installations
are assumed, not concurrent adversarial toolchain mutation.
Builds use a private content snapshot and recheck live inputs before publication.
Known performance limitation: full SDK/toolchain hashing is repeated (two passes
on reuse, four for a new generation including the launcher's fingerprint query).
No persistent identity cache is used.
GOFLAGS and other unsupported Go/compiler overrides fail closed; builds force
native Go cgo defaults (or explicit CGO_ENABLED=0/1), GOENV=off, GOWORK=off, GOTOOLCHAIN=local, readonly modules and no VCS
stamping. Local module replacements must stay inside source/. Dependency versions
and checksums are captured via module files; trusted Go module cache/toolchain
installation is assumed (this is not a sandbox against malicious build tools).

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
import shlex
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


def build_environment():
    # No ambient Go config, workspace, overlays or compiler overrides.
    allowed = {"GOOS", "GOARCH", "GOAMD64", "GOARM64", "GOARM", "GOMIPS",
               "GOMIPS64", "GOPPC64", "GORISCV64", "GO386", "GOWASM"}
    for key, value in os.environ.items():
        if value and ((key.startswith("GO") and key not in allowed | {
                "GOPATH", "GOCACHE", "GOPROXY", "GOSUMDB", "GOPRIVATE",
                "GONOPROXY", "GONOSUMDB"}) or (key.startswith("CGO_") and key != "CGO_ENABLED") or
                key in {"CC", "CXX", "PKG_CONFIG", "CPATH", "C_INCLUDE_PATH",
                        "CPLUS_INCLUDE_PATH", "LIBRARY_PATH", "COMPILER_PATH",
                        "GCC_EXEC_PREFIX", "MACOSX_DEPLOYMENT_TARGET"}):
            raise ValueError(f"unsupported build environment: {key}")
    if os.environ.get("CGO_ENABLED", "0") not in {"0", "1"}:
        raise ValueError("CGO_ENABLED must be 0 or 1")
    env = dict(os.environ)
    env.update(GOENV="off", GOWORK="off", GOTOOLCHAIN="local")
    return env, {key: env.get(key, "") for key in sorted(allowed)}


def tree_identity(root):
    """Content identity, including ignored files; SDK/toolchain installs are trusted."""
    root = Path(root)
    digest = hashlib.sha256()
    paths = sorted(root.rglob("*")) if root.is_dir() else [root]
    for path in paths:
        if path.is_file():
            digest.update(str(path.relative_to(root) if path != root else path).encode())
            digest.update(hashlib.sha256(path.read_bytes()).digest())
        elif path.is_symlink():
            digest.update(str(path).encode() + os.readlink(path).encode())
    return digest.hexdigest()


def effective_identity(go, env, module):
    # Query in the fixed source module, not the launcher's runtime caller cwd.
    effective = json.loads(subprocess.check_output(
        [go, "env", "-json"], env=env, cwd=module, text=True))
    # Diagnostic module location is not a build input; source module bytes are
    # already hashed below. Retain all other effective toolchain/native inputs.
    effective.pop("GOMOD", None)
    # Go creates a random scratch path for this diagnostic flag on every query.
    # Normalize only that generated prefix, not caller-controlled compiler flags.
    if "GOGCCFLAGS" in effective:
        effective["GOGCCFLAGS"] = re.sub(
            r"-ffile-prefix-map=\S*/go-build[0-9]+=/tmp/go-build",
            "-ffile-prefix-map=<go-scratch>=/tmp/go-build", effective["GOGCCFLAGS"])
    identity = {"go_env": effective}
    if effective.get("GOROOT"):
        identity["goroot_sha256"] = tree_identity(effective["GOROOT"])
    if effective["CGO_ENABLED"] == "1":
        for key in ("CC", "CXX"):
            command = shlex.split(effective[key])
            if len(command) != 1:
                raise ValueError("compiler wrappers unsupported")
            tool = shutil.which(command[0], path=env.get("PATH"))
            if not tool:
                raise ValueError("compiler unavailable: " + command[0])
            identity[key] = {"path": str(Path(tool).resolve()), "sha256": tree_identity(tool),
                             "version": subprocess.check_output([tool, "--version"], env=env, text=True)}
        if sys.platform == "darwin":
            def xcrun(*args):
                return subprocess.check_output(["/usr/bin/xcrun", *args], env=env, text=True).strip()
            sdk = env.get("SDKROOT") or xcrun("--show-sdk-path")
            if not Path(sdk).is_absolute() or not Path(sdk).is_dir():
                raise ValueError("SDKROOT must be an existing absolute SDK directory")
            env["SDKROOT"] = sdk
            identity["sdk"] = {"path": sdk, "sha256": tree_identity(sdk)}
            # Hash the selected developer toolchain (clang resources, linker, libs).
            clang = Path(xcrun("--find", "clang")).resolve()
            identity["native_toolchain"] = tree_identity(clang.parent.parent)
            identity["developer_dir"] = env.get("DEVELOPER_DIR", "")
        else:
            # Native system headers/libraries cannot be identified portably here.
            raise ValueError("cgo publication currently supported only on Darwin; use CGO_ENABLED=0")
    return identity


def check_replacements(data, path, repo, go, env):
    # Go's own parser handles quoted strings, blocks, comments and escapes.
    # Never let `go mod edit` touch the live source module.
    with tempfile.TemporaryDirectory(prefix="mod-parse-") as directory:
        temporary = Path(directory) / "go.mod"
        temporary.write_bytes(data)
        parsed = json.loads(subprocess.check_output(
            [go, "mod", "edit", "-json", str(temporary)], env=env, text=True))
    for replacement in parsed.get("Replace") or []:
        new = replacement["New"]
        if new.get("Version"):
            continue
        target = Path(new["Path"])
        if target.is_absolute():
            raise ValueError("absolute local module replacement is not snapshot-safe")
        if not (path.parent / target).resolve().is_relative_to(repo / "source"):
            raise ValueError("local module replacement must stay under repo/source")


def inputs(args, snapshot=None):
    repo = Path(args.repo).resolve()
    env, flags = build_environment()
    go = args.go or shutil.which("go")
    if not go or not Path(go).is_absolute():
        raise ValueError("Go build tool must resolve to an absolute path")
    go = str(Path(go).resolve())
    version = os.environ.get("VERSION") or git(
        repo, "describe", "--tags", "--always", "--dirty").removeprefix("v")
    if not re.fullmatch(r"[A-Za-z0-9._+/-]+", version):
        raise ValueError("VERSION must contain only letters, digits, . _ + / -")
    metadata = {"format": 2, "version": version, "flags": flags,
                "effective": effective_identity(go, env, repo / "source/server"),
                "policy": "native-cgo GOENV=off GOWORK=off GOTOOLCHAIN=local -mod=readonly -buildvcs=false",
                "go": go, "go_sha256": hashlib.sha256(Path(go).read_bytes()).hexdigest(),
                "toolchain": subprocess.check_output([go, "version"], env=env, text=True).strip(),
                "sign": bool(args.sign), "platform": sys.platform, "machine": os.uname().machine,
                "publisher": hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
    digest = hashlib.sha256(json.dumps(metadata, sort_keys=True).encode())
    # Include ignored and untracked source too (embeds, vendored dependencies,
    # module files, scripts). Build from these exact bytes, never the live tree.
    files = sorted(p for p in (repo / "source").rglob("*")
                   if not any(x in {".git"} for x in p.relative_to(repo).parts)
                   and (p.is_symlink() or not p.is_dir()))
    for path in files:
        if path.is_symlink() or not path.is_file():
            raise ValueError(f"unsupported nonregular source input: {path}")
        relative = path.relative_to(repo)
        data = path.read_bytes()
        if path.name == "go.mod":
            check_replacements(data, path, repo, go, env)
        digest.update(str(relative).encode() + b"\0" + hashlib.sha256(data).digest())
        digest.update(str(path.stat().st_mode & 0o111).encode() + b"\0")
        if snapshot is not None:
            output = snapshot / relative
            output.parent.mkdir(parents=True, exist_ok=True)
            output.write_bytes(data)
            output.chmod(0o755 if path.stat().st_mode & 0o111 else 0o644)
    return digest.hexdigest(), metadata, env


def reusable(destination, fingerprint):
    if destination.is_symlink() or not destination.is_dir():
        raise ValueError("invalid existing generation")
    manifest = json.loads((destination / "manifest.json").read_text())
    if manifest.get("fingerprint") != fingerprint or not manifest.get("build_id"):
        raise ValueError("existing generation fingerprint mismatch")
    if {p.name for p in destination.iterdir()} != {"manifest.json", "cercano", "cercano-cli"}:
        raise ValueError("invalid existing generation contents")
    for name, _, _ in TARGETS:
        if validate(destination / name)["sha256"] != manifest["binaries"][name]["sha256"]:
            raise ValueError("existing generation binary hash mismatch")


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
    if root.is_relative_to(repo):
        raise ValueError("generation root must be outside the source repository")
    for _, module, _ in TARGETS:
        if not (repo / module / "go.mod").is_file():
            raise ValueError(f"missing module: {module}")
    fingerprint, metadata, env = inputs(args)
    if args.fingerprint and args.fingerprint != fingerprint:
        raise ValueError("source/build fingerprint changed before publication")
    root.mkdir(parents=True, exist_ok=True)
    destination = root / args.generation
    if os.path.lexists(destination):
        if args.reuse:
            reusable(destination, fingerprint)
            print(destination)
            return
        raise FileExistsError(f"generation already exists: {destination}")
    commit = git(repo, "rev-parse", "HEAD")
    dirty = bool(git(repo, "status", "--porcelain", "--untracked-files=normal"))
    version, go, toolchain = metadata["version"], metadata["go"], metadata["toolchain"]
    stage = Path(tempfile.mkdtemp(prefix=".building-", dir=root))
    snapshot_temp = tempfile.TemporaryDirectory(prefix="cercano-build-source-")
    snapshot = Path(snapshot_temp.name)
    try:
        if inputs(args, snapshot)[0] != fingerprint:
            raise ValueError("source changed while taking build snapshot")
        manifest = {"schema": 1, "generation": args.generation,
                    "build_id": str(uuid.uuid4()), "fingerprint": fingerprint, "source_commit": commit,
                    "source_dirty": dirty, "version": version, "repo": str(repo),
                    "build_inputs": metadata, "go_tool": go, "go_version": toolchain, "sign_requested": args.sign,
                    "validation": "structural-only; binaries not executed", "binaries": {}}
        for name, module, package in TARGETS:
            output = stage / name
            command = [go, "build", "-mod=readonly", "-buildvcs=false", "-ldflags", f"-X main.version={version}",
                       "-o", str(output), package]
            subprocess.run(command, cwd=snapshot / module, env=env, check=True)
            if args.sign and sys.platform == "darwin":
                signer = shutil.which("codesign")
                if not signer:
                    raise ValueError("requested Darwin signing unavailable")
                subprocess.run([signer, "--force", "--sign", "-", "--identifier",
                                name, str(output)], check=True, stdout=sys.stderr)
                subprocess.run([signer, "--verify", "--strict", str(output)],
                               check=True, stdout=sys.stderr)
            manifest["binaries"][name] = {**validate(output), "module": module,
                                            "package": package, "command": command}
        if set(p.name for p in stage.iterdir()) != {t[0] for t in TARGETS}:
            raise ValueError("unexpected build output in generation")
        # Validate the complete pair again after all build/sign commands finish.
        for name, _, _ in TARGETS:
            if validate(stage / name)["sha256"] != manifest["binaries"][name]["sha256"]:
                raise ValueError(f"output changed during build: {name}")
        if inputs(args)[0] != fingerprint:
            raise ValueError("source/build inputs changed during build")
        (stage / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
        for child in stage.iterdir():
            child.chmod(0o444 if child.name == "manifest.json" else 0o555)
        stage.chmod(0o555)
        try:
            rename_exclusive(stage, destination)
        except FileExistsError:
            if not args.reuse:
                raise
            reusable(destination, fingerprint)
    finally:
        snapshot_temp.cleanup()
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
    command.add_argument("--sign", action="store_true", help="require ad-hoc signing on Darwin (no Keychain)")
    command.add_argument("--fingerprint", help="require this fingerprint before building")
    command.add_argument("--reuse", action="store_true", help="validate and reuse matching existing generation")
    command = commands.add_parser("fingerprint", help="print content/build SHA-256; no builds")
    command.add_argument("--repo", default=str(Path(__file__).resolve().parents[3]))
    command.add_argument("--go", help="absolute Go build tool")
    command.add_argument("--sign", action="store_true")
    args = parser.parse_args()
    try:
        if args.command == "fingerprint":
            print(inputs(args)[0])
        else:
            publish(args)
    except (OSError, ValueError, KeyError, RuntimeError, subprocess.CalledProcessError) as error:
        print(f"publication failed: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
