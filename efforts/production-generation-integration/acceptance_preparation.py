"""Non-executing real-cloud preparation; no launch, publication, or authorization.

Only private-path metadata is inspected. Commands are caller-supplied, not
inferred from an installed application or from ambient configuration.
"""
from dataclasses import dataclass
from pathlib import Path
from types import MappingProxyType
from typing import Mapping, Optional, Sequence

from acceptance_env import build_acceptance_env


@dataclass(frozen=True)
class PreparedCommand:
    argv: tuple[str, ...]
    env: Mapping[str, str]


@dataclass(frozen=True)
class PreparedRealCloudLaunch:
    launcher: PreparedCommand
    host: PreparedCommand
    cli: PreparedCommand


def _argv(value: Sequence[str]) -> tuple[str, ...]:
    if isinstance(value, (str, bytes)):
        raise ValueError("supply an argv sequence, not a shell command")
    args = tuple(value)
    if not args or any(not isinstance(a, str) or "\0" in a for a in args):
        raise ValueError("nonempty argv without NUL required")
    executable = Path(args[0])
    if (not executable.is_absolute() or str(executable) != args[0]
            or ".." in executable.parts):
        raise ValueError("explicit absolute normalized executable required")
    return args


def prepare_real_cloud_launch(
    *,
    home: str,
    private_root: str,
    credential_policy: str,
    ledger_path: str,
    launcher_argv: Sequence[str],
    host_argv: Sequence[str],
    cli_argv: Sequence[str],
    tool_paths: Optional[Mapping[str, str]] = None,
) -> PreparedRealCloudLaunch:
    """Prepare command descriptors only; never execute them or create files.

    The existing aggregate ledger is mandatory. It is not parsed, initialized,
    copied, reset, or unhalted. Its authority/budget must be verified separately.
    All three descriptors share one immutable allowlist. A separately authorized
    executor must pass env=command.env, never overlay the ambient environment.
    Workers should inherit the host environment without ambient additions; this
    module cannot enforce the real host's internal subprocess behavior.

    Directory provisioning, explicit CLI/host arguments and build tags, private
    logging authorities, credential mutation guards, and lifetime controls are
    separate launch prerequisites. This is not filesystem or network isolation.
    """
    commands = tuple(_argv(v) for v in (launcher_argv, host_argv, cli_argv))
    env = build_acceptance_env(
        home=home, private_root=private_root,
        credential_policy=credential_policy, ledger_path=ledger_path,
        tool_paths=tool_paths,
    )
    # build_acceptance_env already validates ownership, mode, links and type,
    # but deliberately allows missing files. Preparation must not start a fresh
    # accounting authority. stat() reads metadata only, not the ledger contents.
    Path(env["CERCANO_ACCEPTANCE_LEDGER"]).stat()
    shared = MappingProxyType(env)
    return PreparedRealCloudLaunch(*(PreparedCommand(a, shared) for a in commands))
