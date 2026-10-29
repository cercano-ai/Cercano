"""Pure environment construction, NOT a sandbox or launch authorization.

No ambient environment is consulted, no file contents are read, and nothing is
created. Callers own directory creation (0700), explicit launch arguments, build
tags, credential mutation guards, usage budgets, and process lifetime.
"""

import os
from pathlib import Path
import stat
from typing import Mapping, Optional


_DIRECTORY_PATHS = {
    "XDG_CONFIG_HOME": "config",
    "XDG_DATA_HOME": "data",
    "XDG_CACHE_HOME": "cache",
    "XDG_STATE_HOME": "state",
    "XDG_RUNTIME_DIR": "runtime",
    "TMPDIR": "tmp",
    "CERCANO_THEMES_DIR": "config/cercano/themes",
    "CERCANO_SEARCH_SCRIPT_DIR": "search-scripts",
}
_FILE_PATHS = {
    "CERCANO_UI_CONFIG": "config/cercano/ui.yaml",
    "CERCANO_TUI_PERF_LOG": "logs/tui-perf.log",
    "CERCANO_COLORPROBE_LOG": "logs/colorprobe.log",
    "CERCANO_SEARCH_TRACE_LOG": "logs/search-trace.log",
    "CERCANO_ANOMALY_LOG": "logs/stream-anomalies.jsonl",
}


def _absolute(value: str) -> Path:
    # Lexical only: in particular do not stat/resolve supplied HOME or tools.
    if not isinstance(value, str) or not value or any(ord(c) < 32 for c in value):
        raise ValueError("expected a nonempty absolute path without control characters")
    p = Path(value)
    if not p.is_absolute() or str(p) != value or ".." in p.parts:
        raise ValueError("paths must be absolute and normalized (no ~ expansion)")
    return p


def _within(path: Path, parent: Path) -> bool:
    return path == parent or parent in path.parents


def _check_node(path: Path, directory: bool) -> None:
    try:
        info = path.lstat()
    except FileNotFoundError:
        return
    valid_type = stat.S_ISDIR(info.st_mode) if directory else stat.S_ISREG(info.st_mode)
    if not valid_type or info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise ValueError("private paths must be owned, owner-only, and not symlinks: " + str(path))
    if not directory and info.st_nlink != 1:
        raise ValueError("private files must not be hardlinked: " + str(path))


def _private_path(root: Path, path: Path, directory: bool) -> str:
    if path == root or not _within(path, root):
        raise ValueError("artifact paths must be strictly beneath private_root")
    relative = path.relative_to(root)
    current = root
    for part in relative.parts[:-1]:
        current /= part
        _check_node(current, True)
    _check_node(path, directory)
    return str(path)


def build_acceptance_env(
    *,
    home: str,
    private_root: str,
    credential_policy: str,
    ledger_path: str,
    tool_paths: Optional[Mapping[str, str]] = None,
) -> dict[str, str]:
    """Return a fresh allowlist; never merge it with os.environ.

    credential_policy is required: 'disabled' or 'authorized-read-only'. The
    latter merely enables credential acquisition; a prepared read-only service
    and mutation guard MUST separately enforce the user's authorization.
    ledger_path is required, never read, created, initialized, or reset here.
    It must be beneath the same private root, including when resuming a run.

    private_root must already exist, be canonical, owner-only, and outside HOME.
    Missing descendants are allowed: caller must create directory destinations
    and file parents with mode 0700 before launch. Existing descendants must be
    private, owned, and free of symlinks/hardlinked files. This is a point-in-time
    validation, not race-proof filesystem confinement.

    tool_paths may explicitly supply only PATH and GOROOT. No shared cache or
    arbitrary environment overrides are accepted. PATH defaults to os.defpath;
    each supplied component must be an absolute normalized directory spelling.
    No tool is located, inspected, or executed here.
    """
    if credential_policy not in ("disabled", "authorized-read-only"):
        raise ValueError("explicit credential policy required")
    home_path = _absolute(home)
    root = _absolute(private_root)
    if home_path == Path("/") or _within(root, home_path) or _within(home_path, root):
        raise ValueError("private_root and HOME must be disjoint")
    # Only resolve the private root, never ordinary HOME. Reject aliases rather
    # than silently changing artifact authority. Supply /private/tmp on macOS.
    if root.resolve(strict=True) != root:
        raise ValueError("private_root must use its canonical spelling")
    _check_node(root, True)
    ledger = _absolute(ledger_path)
    directories = [root / p for p in _DIRECTORY_PATHS.values()]
    directories += [root / p for p in ("cache/go-build", "cache/go-mod", "data/go", "logs", "config/cercano")]
    files = [root / p for p in _FILE_PATHS.values()]
    if any(_within(p, ledger) for p in directories) or any(
        _within(p, ledger) or _within(ledger, p) for p in files
    ):
        raise ValueError("ledger must not collide with environment destinations")
    env = {"HOME": home, "PATH": os.defpath, "LANG": "C", "TERM": "xterm-256color"}
    for key, relative in _DIRECTORY_PATHS.items():
        env[key] = _private_path(root, root / relative, True)
    for key, relative in _FILE_PATHS.items():
        env[key] = _private_path(root, root / relative, False)
    env.update({
        "CERCANO_DISABLE_CREDENTIALS": "1" if credential_policy == "disabled" else "0",
        "CERCANO_TUI_PERF": "0",
        "CERCANO_SEARCH_TRACE": "0",
        "CERCANO_ACCEPTANCE_LEDGER": _private_path(root, ledger, False),
    })
    tools = dict(tool_paths or {})
    if tools.keys() - {"PATH", "GOROOT"}:
        raise ValueError("only explicit PATH and GOROOT tool paths are supported")
    for key, value in tools.items():
        if not isinstance(value, str):
            raise ValueError("tool paths must be strings")
        for component in value.split(os.pathsep) if key == "PATH" else [value]:
            _absolute(component)
        env[key] = value
    # Prevent Go's normal-HOME defaults even if this environment is used for a
    # separately authorized build. No cache is discovered in the real HOME.
    for key, relative in {"GOCACHE": "cache/go-build", "GOMODCACHE": "cache/go-mod", "GOPATH": "data/go"}.items():
        env[key] = _private_path(root, root / relative, True)
    env.update({"GOPROXY": "off", "GOSUMDB": "off", "GOTOOLCHAIN": "local"})
    return env
