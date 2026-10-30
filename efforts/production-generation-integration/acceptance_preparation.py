"""Real-cloud descriptors and read-only executor checks; no launch authorization.

Preparation inspects only private-path metadata. The separate execution check
reads reviewed artifact bytes and the existing ledger, without changing either.
Commands are caller-supplied, never inferred from ambient configuration.
"""
from dataclasses import dataclass
from datetime import datetime, timezone
import hashlib
import json
import os
import stat
from pathlib import Path
from types import MappingProxyType
from typing import Mapping, Optional, Sequence

from acceptance_env import build_acceptance_env


@dataclass(frozen=True)
class PreparedCommand:
    argv: tuple[str, ...]
    env: Mapping[str, str]

    def check_execution(self, artifact_sha256: Mapping[str, str]) -> None:
        """Read-only, point-in-time checks, not authorization or HTTP admission.

        Digests must come from separately reviewed artifacts, not be discovered
        by the executor. Pin the executable and every absolute file argument
        (including interpreter scripts/configs). No PATH or shell discovery.
        The Go transport remains the locked, aggregate accounting authority.
        """
        _argv(self.argv)
        if not artifact_sha256 or self.argv[0] not in artifact_sha256:
            raise ValueError("explicit executable digest required")
        for arg in self.argv:
            path = Path(arg)
            if path.is_absolute() and path.is_file() and arg not in artifact_sha256:
                raise ValueError("absolute file argument requires reviewed digest")
        for name, digest in artifact_sha256.items():
            path = Path(name)
            if not path.is_absolute() or not path.is_file() or hashlib.sha256(path.read_bytes()).hexdigest() != digest:
                raise ValueError("artifact missing or digest mismatch")
        if not os.access(self.argv[0], os.X_OK):
            raise ValueError("executable permission required")
        ledger = Path(self.env["CERCANO_ACCEPTANCE_LEDGER"])
        info = ledger.lstat()
        if (not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid()
                or info.st_mode & 0o077 or info.st_nlink != 1):
            raise ValueError("unsafe ledger")
        # Actual schema/limits: internal/llm/responses/acceptance_run.go.
        # Never lock/create/write/reset this ledger in Python. A concurrent
        # transport update can only make this preflight stale; HTTP admission
        # rechecks under its existing lock before every request.
        try:
            state = json.loads(ledger.read_bytes())
            def timestamp(value):
                result = datetime.fromisoformat(value.replace('Z', '+00:00'))
                if result.tzinfo is None:
                    raise ValueError("timezone required")
                return result
            now = datetime.now(timezone.utc)
            age = (now - timestamp(state['started'])).total_seconds()
            counts = [state[k] for k in ('attempts', 'input_tokens', 'output_tokens')]
            if any(type(n) is not int or n < 0 for n in counts):
                raise ValueError("invalid accounting counters")
            attempts, input_tokens, output_tokens = counts
            active = state['active']
            if active is not None and not isinstance(active, dict):
                raise ValueError("invalid active attempts")
            if (type(state['halted']) is not bool or state['halted']
                    or not 0 <= age < 1800 or attempts >= 40
                    or input_tokens + output_tokens >= 300000 or output_tokens >= 30000
                    # A launch/resume preflight is deliberately stricter than
                    # HTTP admission: do not relaunch over any unresolved work.
                    or bool(active)):
                raise ValueError("halted, expired, unresolved or exhausted ledger")
        except (KeyError, TypeError, AttributeError, OverflowError) as exc:
            raise ValueError("invalid acceptance ledger") from exc


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
