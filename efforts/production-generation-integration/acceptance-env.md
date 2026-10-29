# Bounded normal-HOME acceptance environment

`acceptance_env.py:build_acceptance_env` constructs a fresh `dict[str, str]` for
an explicitly authorized acceptance caller. It does not launch anything or
modify the filesystem. It supplements, rather than replaces, the existing Go
acceptance HTTP/ledger instrumentation and dev-host process harness.

## API and preparation

```python
from acceptance_env import build_acceptance_env

env = build_acceptance_env(
    home=supplied_ordinary_home,       # explicit; never discovered or inspected
    private_root=canonical_private_root,
    credential_policy="disabled",    # REQUIRED; no implicit credentials
    ledger_path=explicit_existing_run_ledger_path,
    # Optional, explicitly supplied tool directories, never ambient discovery:
    # tool_paths={"PATH": supplied_tool_path, "GOROOT": supplied_go_root},
)
```

The other policy is `authorized-read-only`: it sets
`CERCANO_DISABLE_CREDENTIALS=0` while retaining the supplied HOME so separately
authorized native acquisition can operate. **That environment flag does not
make credentials read-only.** The prepared host's read-only credential service
and mutation guard must remain intact. Refresh, login, credential inspection,
or persistence are not authorized by this helper; credential errors remain a
stop. The required policy and ledger arguments have no defaults.

The caller must provision an invocation-owned canonical root outside HOME with
mode 0700 (on macOS use the canonical `/private/tmp/...`, not `/tmp/...`). No
HOME expansion, real-user configuration, tokens, or shared caches are read.
Only private-root metadata is inspected; HOME and tool path validation is
lexical. Existing private descendants must be owner-only, owned by the current
UID, and correctly typed, with no symlinks or multiply linked files. Missing
descendants are allowed. Before any separately authorized launch, the caller
must create directory destinations and file parents at 0700, including `logs`,
`config/cercano`, and `runtime`. Keep the root private throughout use. Do not
merge the result into `os.environ` or append ambient variables.

Defaults include deterministic `LANG=C`, `TERM=xterm-256color`, and
`PATH=os.defpath`. Only explicit `PATH` and `GOROOT` tool overrides are accepted;
relative/empty PATH components are refused. Go caches/GOPATH are private and
Go proxy/checksum downloads and toolchain auto-download are disabled. These
settings do not build or approve a build. A caller needing another compiler
contract must make a separately reviewed extension, not copy ambient flags.

## Destination contract

All destinations below are relative to the explicit private root:

| Variable | Destination/value |
| --- | --- |
| `XDG_CONFIG_HOME` | `config` |
| `XDG_DATA_HOME` | `data` |
| `XDG_CACHE_HOME` | `cache` |
| `XDG_STATE_HOME` | `state` |
| `XDG_RUNTIME_DIR` (not `XDG_RUNTIME_HOME`) | `runtime` |
| `TMPDIR` | `tmp` |
| `CERCANO_UI_CONFIG` | `config/cercano/ui.yaml` |
| `CERCANO_THEMES_DIR` | `config/cercano/themes` |
| `CERCANO_TUI_PERF` / `CERCANO_TUI_PERF_LOG` | `0` / `logs/tui-perf.log` |
| `CERCANO_COLORPROBE_LOG` | `logs/colorprobe.log` |
| `CERCANO_SEARCH_TRACE` / `CERCANO_SEARCH_TRACE_LOG` | `0` / `logs/search-trace.log` |
| `CERCANO_SEARCH_SCRIPT_DIR` | `search-scripts` |
| `CERCANO_ANOMALY_LOG` | `logs/stream-anomalies.jsonl` |
| `GOCACHE` / `GOMODCACHE` / `GOPATH` | `cache/go-build` / `cache/go-mod` / `data/go` |
| `CERCANO_ACCEPTANCE_LEDGER` | required explicit path beneath root |

Ledger selection must reuse the aggregate run's original authority across
hosts, workers, and retries. The helper never reads, initializes, resets, or
chooses a default ledger. A missing path passing validation is **not** permission
to start a new counter; the caller must independently establish the correct
existing ledger and remaining authorization. Ledger collisions with other
environment destinations are refused. Provider keys, routing/debug switches,
proxy settings, loader variables, and arbitrary build flags are not inherited.

Names were checked against CLI `internal/uiconfig/uiconfig.go`,
`internal/ui/{tui_perf,zz_colorprofile_probe,conversation_search_trace}.go` and
server `internal/{web/script.go,llm/anomaly.go,llm/responses/acceptance_run.go}`.
See `normal-home-first-prompt-residual-audit.md` and
`real-cloud-observation-2026-10-29.md` for remaining acceptance constraints.

## Hermetic verification and limits

From the repository root, using Python on PATH:

```
python3 -B -m unittest discover -s efforts/production-generation-integration -p test_acceptance_env.py -v
```

Tests inspect output only, using fake HOME/private temporary directories and a
hostile ambient environment. They cover the exact allowlist, disabled default
flags, both explicit credential policies, absent required arguments, invalid
paths/tool overrides, canonical root, private permissions/types, symlinks,
hardlinks, unchanged ledger contents, and no HOME metadata/content access.

This is environment construction, **not a sandbox or full first-prompt
qualification**. Metadata validation is point-in-time, not race-proof. It does
not certify all HOME consumers, subprocess behavior, tagged builds, worker
failure/routing-log propagation, native credentials, network restrictions,
configuration routes, runtime administration, or request/time/token budgets.
Prepared host/worker log authorities and CLI launch arguments still need their
own private explicit paths. Retaining ordinary HOME deliberately leaves that
identity visible to application code. No app/model/native credential access,
network, build, signing, installation, or live acceptance run is needed or
performed by these tests.
