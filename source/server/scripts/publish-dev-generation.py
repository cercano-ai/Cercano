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
--sign uses codesign-if-available.sh on Darwin: stable Developer ID signing,
CERCANO_CODESIGN_ID override (none disables), or no-op without an identity.
Applied signatures are strictly verified. Non-applicable on other platforms.
VERSION overrides the Git-derived version.
Signing fingerprints include hashed identity/discovery evidence and helper policy;
no identity names or discovery listings are persisted in manifests. Darwin signing
may access the Keychain; this tool does not guarantee absence of prompts.

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
Final mismatches report sorted added/removed/changed repo-relative source paths
and top-level metadata key names only (10 names per category, 160 characters per
name, with truncation/counts). No file contents or metadata values are reported.
Failed final collection refuses publication without claiming a partial input diff.
The ordinary launcher uses publish-current --root ABSOLUTE [--repo PATH]
[--go ABSOLUTE] [--sign]: collect inputs, derive the fingerprint-named destination,
then validate reuse or publish from that same initial tuple. Stdout is one absolute
generation path. This takes one collection on reuse, three on publication (initial,
snapshot, final live recheck), rather than the explicit two-command API's two/four.
Full SDK/toolchain hashing remains; no persistent identity cache is used.
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
from concurrent.futures import ThreadPoolExecutor
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


def descendant_paths(root):
    """Glob-compatible traversal without repeated Path/glob selector work.

    Do not descend through directory symlinks. Refuse incomplete scans rather
    than fingerprinting a partial tree. Sort Paths globally at the caller, not
    strings or per-directory DFS order (which differ for names such as a/z and
    a.txt).
    """
    def scan_error(error):
        raise error

    for directory, directories, files in os.walk(root, onerror=scan_error):
        parent = Path(directory)
        for name in directories + files:
            yield parent / name


def file_digest(path):
    """Hash every logical file independently, with bounded read memory."""
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.digest()


def tree_identity(root):
    """Content identity, including ignored files; SDK/toolchain installs are trusted."""
    root = Path(root)
    digest = hashlib.sha256()
    paths = sorted(descendant_paths(root)) if root.is_dir() else [root]
    for path in paths:
        if path.is_file():
            digest.update(str(path.relative_to(root) if path != root else path).encode())
            digest.update(file_digest(path))
        elif path.is_symlink():
            digest.update(str(path).encode() + os.readlink(path).encode())
    return digest.hexdigest()


def effective_identity(go, env, module):
    # Capture diagnostic stderr too: failed final collection must not leak tool
    # output (potential environment/signing data) before its safe refusal.
    # Query in the fixed source module, not the launcher's runtime caller cwd.
    effective = json.loads(subprocess.check_output(
        [go, "env", "-json"], env=env, cwd=module, text=True, stderr=subprocess.PIPE))
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
    trees = []
    if effective.get("GOROOT"):
        trees.append((identity, "goroot_sha256", effective["GOROOT"]))
    if effective["CGO_ENABLED"] == "1":
        for key in ("CC", "CXX"):
            command = shlex.split(effective[key])
            if len(command) != 1:
                raise ValueError("compiler wrappers unsupported")
            tool = shutil.which(command[0], path=env.get("PATH"))
            if not tool:
                raise ValueError("compiler unavailable: " + command[0])
            identity[key] = {"path": str(Path(tool).resolve()),
                             "version": subprocess.check_output([tool, "--version"], env=env, text=True, stderr=subprocess.PIPE)}
            trees.append((identity[key], "sha256", tool))
        if sys.platform == "darwin":
            def xcrun(*args):
                return subprocess.check_output(["/usr/bin/xcrun", *args], env=env, text=True, stderr=subprocess.PIPE).strip()
            sdk = env.get("SDKROOT") or xcrun("--show-sdk-path")
            if not Path(sdk).is_absolute() or not Path(sdk).is_dir():
                raise ValueError("SDKROOT must be an existing absolute SDK directory")
            env["SDKROOT"] = sdk
            identity["sdk"] = {"path": sdk}
            trees.append((identity["sdk"], "sha256", sdk))
            # Hash the selected developer toolchain (clang resources, linker, libs).
            clang = Path(xcrun("--find", "clang")).resolve()
            trees.append((identity, "native_toolchain", clang.parent.parent))
            identity["developer_dir"] = env.get("DEVELOPER_DIR", "")
        else:
            # Native system headers/libraries cannot be identified portably here.
            raise ValueError("cgo publication currently supported only on Darwin; use CGO_ENABLED=0")
    # Only independent trusted toolchain trees overlap, never source snapshot
    # reads or initial/snapshot/final collections. Two workers measured useful;
    # keep the resource bound conservative even though three benchmarked faster.
    # No alias/content hash cache.
    if len(trees) > 1:
        with ThreadPoolExecutor(max_workers=2) as pool:
            hashes = pool.map(tree_identity, (root for _, _, root in trees))
            # Consume in discovery order: deterministic assembly and exceptions.
            # Executor shutdown waits for other reads before propagating failure.
            for (target, key, _), value in zip(trees, hashes):
                target[key] = value
    else:
        for target, key, root in trees:
            target[key] = tree_identity(root)
    return identity


def check_replacements(data, path, repo, go, env):
    # Go's own parser handles quoted strings, blocks, comments and escapes.
    # Never let `go mod edit` touch the live source module.
    with tempfile.TemporaryDirectory(prefix="mod-parse-") as directory:
        temporary = Path(directory) / "go.mod"
        temporary.write_bytes(data)
        parsed = json.loads(subprocess.check_output(
            [go, "mod", "edit", "-json", str(temporary)], env=env, text=True, stderr=subprocess.PIPE))
    for replacement in parsed.get("Replace") or []:
        new = replacement["New"]
        if new.get("Version"):
            continue
        target = Path(new["Path"])
        if target.is_absolute():
            raise ValueError("absolute local module replacement is not snapshot-safe")
        if not (path.parent / target).resolve().is_relative_to(repo / "source"):
            raise ValueError("local module replacement must stay under repo/source")


def signing_identity(repo, requested, env):
    if not requested or sys.platform != "darwin":
        return {"policy": "not-applicable"}
    helper = repo / "source/server/scripts/codesign-if-available.sh"
    resolution = subprocess.check_output(
        ["/bin/bash", str(helper), "--resolve-identity"], env=env, text=True, stderr=subprocess.PIPE)
    identity = resolution.split("\n", 1)[0].strip()
    if identity and not shutil.which("codesign", path=env.get("PATH")):
        raise ValueError("requested Darwin signing unavailable")
    # Only a digest enters manifests; do not persist identity names or discovery
    # output. Pin the resolved certificate for both snapshot signing calls.
    env["CERCANO_CODESIGN_ID"] = identity or "none"
    return {"policy": "codesign-if-available-v1", "enabled": bool(identity),
            "identity_sha256": hashlib.sha256(resolution.encode()).hexdigest(),
            "helper_sha256": hashlib.sha256(helper.read_bytes()).hexdigest()}


def inputs(args, snapshot=None, *, inventory=None):
    """Return the existing fingerprint/metadata/env tuple; optionally record file identities.

    Inventory is diagnostic only and uses exactly the bytes/mode fed to the digest.
    It must only be used after a successful collection.
    """
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
                "toolchain": subprocess.check_output([go, "version"], env=env, text=True, stderr=subprocess.PIPE).strip(),
                "sign": bool(args.sign), "signing": signing_identity(repo, args.sign, env),
                "platform": sys.platform, "machine": os.uname().machine,
                "publisher": hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
    digest = hashlib.sha256(json.dumps(metadata, sort_keys=True).encode())
    # Include ignored and untracked source too (embeds, vendored dependencies,
    # module files, scripts). Build from these exact bytes, never the live tree.
    files = sorted(p for p in descendant_paths(repo / "source")
                   if not any(x in {".git"} for x in p.relative_to(repo).parts)
                   and (p.is_symlink() or not p.is_dir()))
    for path in files:
        if path.is_symlink() or not path.is_file():
            raise ValueError(f"unsupported nonregular source input: {path}")
        relative = path.relative_to(repo)
        data = path.read_bytes()
        if path.name == "go.mod":
            check_replacements(data, path, repo, go, env)
        content_identity = hashlib.sha256(data).digest()
        mode = path.stat().st_mode & 0o111
        digest.update(str(relative).encode() + b"\0" + content_identity)
        digest.update(str(mode).encode() + b"\0")
        if inventory is not None:
            inventory[str(relative)] = (content_identity, mode)
        if snapshot is not None:
            output = snapshot / relative
            output.parent.mkdir(parents=True, exist_ok=True)
            output.write_bytes(data)
            output.chmod(0o755 if path.stat().st_mode & 0o111 else 0o644)
    return digest.hexdigest(), metadata, env


def input_changes(before, after, old_metadata, new_metadata):
    """Bounded, deterministic diagnostics: names only, never identities or values.

    Only top-level metadata keys are reported: nested Go/signing discovery data
    may contain sensitive values (and even caller-controlled dictionary keys).
    JSON quoting prevents control characters in filenames from becoming log lines.
    """
    limit, width = 10, 160

    def names(label, values):
        values = sorted(values)
        shown = [json.dumps(value if len(value) <= width else value[:width] + "…",
                            ensure_ascii=True) for value in values[:limit]]
        suffix = f", +{len(values) - limit} more" if len(values) > limit else ""
        return f"{label} ({len(values)}): [" + ", ".join(shown) + suffix + "]"

    return "; ".join((
        names("added", after.keys() - before.keys()),
        names("removed", before.keys() - after.keys()),
        names("changed", (key for key in before.keys() & after.keys()
                          if before[key] != after[key])),
        names("metadata keys", (key for key in old_metadata.keys() | new_metadata.keys()
                                if key not in old_metadata or key not in new_metadata
                                or old_metadata[key] != new_metadata[key]))))


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


def publish(args, *, fingerprint_generation=False):
    """Publish with explicit naming, or derive the name from this collection.

    The combined operation must not collect externally and pass only a token:
    reuse/build decisions share the initial metadata, environment and inventory.
    Snapshot and final live collections below remain independent safety checks.
    """
    root_arg = Path(args.root)
    if not root_arg.is_absolute():
        raise ValueError("--root must be an explicit absolute path")
    root = root_arg.resolve()
    home_bin = (Path.home() / "bin").resolve()
    if root == home_bin or home_bin in root.parents or ".cercano-libexec" in root.parts:
        raise ValueError("installation paths (~/bin or .cercano-libexec) are prohibited")
    if not fingerprint_generation and not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]{0,127}", args.generation):
        raise ValueError("generation must be a single safe name (1-128 characters)")
    repo = Path(args.repo).resolve()
    if root.is_relative_to(repo):
        raise ValueError("generation root must be outside the source repository")
    for _, module, _ in TARGETS:
        if not (repo / module / "go.mod").is_file():
            raise ValueError(f"missing module: {module}")
    initial_inventory = {}
    fingerprint, metadata, env = inputs(args, inventory=initial_inventory)
    if args.fingerprint and args.fingerprint != fingerprint:
        raise ValueError("source/build fingerprint changed before publication")
    root.mkdir(parents=True, exist_ok=True)
    generation = fingerprint if fingerprint_generation else args.generation
    destination = root / generation
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
        manifest = {"schema": 1, "generation": generation,
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
                helper = snapshot / "source/server/scripts/codesign-if-available.sh"
                subprocess.run(["/bin/bash", str(helper), str(output), name],
                               env=env, check=True, stdout=sys.stderr)
                if metadata["signing"]["enabled"]:
                    subprocess.run(["codesign", "--verify", "--strict", str(output)],
                                   env=env, check=True, stdout=sys.stderr)
            manifest["binaries"][name] = {**validate(output), "module": module,
                                            "package": package, "command": command}
        if set(p.name for p in stage.iterdir()) != {t[0] for t in TARGETS}:
            raise ValueError("unexpected build output in generation")
        # Validate the complete pair again after all build/sign commands finish.
        for name, _, _ in TARGETS:
            if validate(stage / name)["sha256"] != manifest["binaries"][name]["sha256"]:
                raise ValueError(f"output changed during build: {name}")
        final_inventory = {}
        try:
            final_fingerprint, final_metadata, _ = inputs(args, inventory=final_inventory)
        except (OSError, ValueError, KeyError, RuntimeError, subprocess.CalledProcessError) as error:
            # No partial inventory or exception text: errors can contain source
            # contents, environment values or signing discovery output.
            raise ValueError("unable to collect final source/build inputs "
                             f"({type(error).__name__}); publication refused; "
                             "no input diff available") from None
        if final_fingerprint != fingerprint:
            raise ValueError("source/build inputs changed during build; " +
                             input_changes(initial_inventory, final_inventory,
                                           metadata, final_metadata))
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
    command.add_argument("--sign", action="store_true", help="use optional stable Developer ID signing policy on Darwin")
    command.add_argument("--fingerprint", help="require this fingerprint before building")
    command.add_argument("--reuse", action="store_true", help="validate and reuse matching existing generation")
    command = commands.add_parser("publish-current", help="validate or publish the current fingerprint-named generation")
    command.add_argument("--root", required=True, help="absolute dedicated generation root")
    command.add_argument("--repo", default=str(Path(__file__).resolve().parents[3]))
    command.add_argument("--go", help="absolute Go build tool")
    command.add_argument("--sign", action="store_true")
    command.set_defaults(generation=None, fingerprint=None, reuse=True)
    command = commands.add_parser("fingerprint", help="print content/build SHA-256; no builds")
    command.add_argument("--repo", default=str(Path(__file__).resolve().parents[3]))
    command.add_argument("--go", help="absolute Go build tool")
    command.add_argument("--sign", action="store_true")
    args = parser.parse_args()
    try:
        if args.command == "fingerprint":
            print(inputs(args)[0])
        else:
            publish(args, fingerprint_generation=args.command == "publish-current")
    except (OSError, ValueError, KeyError, RuntimeError, subprocess.CalledProcessError) as error:
        print(f"publication failed: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
