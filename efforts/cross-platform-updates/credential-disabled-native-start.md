# Credential-disabled normal-start evidence

User approved an explicit credential-disabled runtime option for the bounded temporary-install milestone. Default behavior remains unchanged; no inference from HOME or configuration. Native credential integration remains outside this milestone.

## Implementation

- `1be362efd52f`: explicit `CERCANO_DISABLE_CREDENTIALS=1`, rejecting credential store before backend open, OAuth/cloud guards and migration suppression. Absent/`0` retains normal behavior; malformed values fail closed. Fixture children inherit the explicit setting.
- `7b8079bae1c2`: existing-agent reuse refusal when disabled clients cannot establish `credentials_disabled` through existing read-only health RPC. This followed a reproduced old-agent acceptance defect. Connection behavior remains under review for transport-level reconnection; do not interpret it as authentication or runtime credential revocation.
- `426e1d082853`: CLI module graph aligned to already-selected dependencies; no dependency upgrades.
- `d318caaf5500`, `f1283b9673d8`: native normal-start harness and corrected Seatbelt localhost grammar. Initial attempt stopped at sandbox parse failure before application startup; fixed without broadening beyond fixture loopback ports.

## Parent-executed evidence

Focused `go test ./internal/secrets ./internal/cloudfactory ./internal/hostsvc/credentials ./pkg/config -count=1 -timeout=120s` passed. Fake backend tests distinguish disabled zero-dispatch from ordinary enabled dispatch without opening the real credential backend.

On native macOS arm64:

```
CERCANO_NATIVE_INSTALL_TEST=1 CERCANO_NATIVE_NORMAL_START_TEST=1 \
  go test ./internal/updatecoord/acquisition \
  -run '^TestNativeTemporaryInstall$' -count=1 -timeout=15m -v
```

Passed in 21.25 seconds test runtime. Harness builds two current-source fixture releases, authenticates/acquires/stages all six binaries via ephemeral local TUF, and checks 24 stable/direct-child offline version invocations across missing/corrupt/inaccessible updater databases. These are not published-release compatibility tests.

Four actual normal-agent startup cases ran for fixture release 1: stable and direct child, with missing and corrupt updater databases. Each checked ready PID/version, performed chat against local Ollama-shaped fixture HTTP, read back persisted user and assistant turns, requested graceful shutdown and reaped the exact owned child. No paid provider or real credentials were used.

Children receive an explicit environment allowlist and isolated HOME/config/cache/TMPDIR, installation and agent endpoint. The sandbox denies securityd/keychain access and network except the fixture's loopback inference endpoint and allocated agent listener. The default agent port is rejected, and readiness compares the child PID before interaction. Failure cleanup targets only the owned child.

## Expanded native evidence

`a36efba095a7` closes a reproduced transparent gRPC-reconnect bypass: credential-disabled ClientConns use one successful transport, with application reconnect creating/rechecking a fresh connection. `f8dfe4ff03d4` adds an explicit pinned connection for updater native-peer observations independent of credential policy, and anchors protected operation input/status to the retained directory object. These checks are not peer authentication.

`97ffd5accfe0` enables the disabled-client policy in the native harness itself. `03124f2f1ca4` extends normal-start testing to both fixture releases × stable/direct launch × missing/corrupt/inaccessible updater bookkeeping. Each of the twelve agent cases includes a real CLI pseudo-terminal interaction using Python's standard-library PTY driver, voluntary `/quit`, and saved-turn checks through the parent RPC client. The CLI sandbox only permits its isolated agent endpoint and denies agent executable launch; this slice deliberately does not exercise CLI auto-launch.

Parent reran the full expanded harness with both native opt-ins and `CERCANO_DISABLE_CREDENTIALS=1`: PASS, 87.228 seconds package runtime. This supersedes the narrower four-case coverage above. Parent also ran focused race tests across agentclient, setupresetcmd and server for transport isolation, credential compatibility, health and drain; they passed.

## Limits

This proves real normal agent/CLI startup and local fixture-backed interaction with credential access explicitly disabled. It does not verify native keychain behavior, paid providers, cross-version application compatibility, complete updater participant evidence, integrated draining/publication, or native CLI auto-launch/reconnect acceptance. The separate drain-stream and protected handoff implementations remain incomplete update integration. No push or live installation change is authorized.
