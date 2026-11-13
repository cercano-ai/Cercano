# Full native recovery acceptance (Darwin)

`TestFullNativeRecoveryAcceptance` now joins the component paths described in
[the baseline investigation](full-native-recovery-harness-probes.md). This is a
CLI **model-level** integrated acceptance, not a standalone terminal/PTY or
installed-service acceptance.

## Journey and boundaries

- The CLI test copies an explicitly supplied server test binary to two distinct,
  canonical temporary generation paths, each named `cercano`, and launches its
  own source/destination children with exactly one matching `--host` argument.
  Private HOME/config, store, token, registry, launch intent and lock fixtures
  are used. Cleanup closes pipes, waits, and kills only owned children if needed,
  with bounded reap waits. No installed launcher, service, or user DB is used.
- The source is a real authenticated coexistence server owning a seeded committed
  history turn. **Only cooperative force-relinquish is deliberately stalled** to
  trigger the real observation timeout; source identity/authentication are real.
  It is not a full inference host. The destination uses production development
  host construction with the existing synthetic dependency fixture (no model
  download/credentials). Config, generation manifest and launcher records are
  fixture-created, not produced by executing the launcher.
- Real authenticated resume produces an offer. The CLI production confirmation
  queue/key handler first receives Enter (default No), asserts no recovery command
  and a live child, then makes a fresh request and checks the source epoch is
  unchanged before accepting with `y`.
- No process identity, stop, writer-absence, or guarded recovery adapter is
  substituted. The destination performs the production authenticated native
  containment, generation launch exclusion/inventory, and guarded store recovery.
  The parent independently waits for the exact source child and checks native
  TERM/KILL termination before hydration. Inventory necessarily reads kernel
  process metadata; only the authenticated, exactly attributed test source is
  eligible for signaling. This is not an assertion of zero system enumeration.
- The real CLI recovered-binding reducer and startup hydration message run real
  history/metadata RPC commands and stream reducers. Assertions require completed
  hydration and the committed history in chat entries. Reconnect preserves the
  new exact epoch; the destination checks durable owner and epoch against that
  CLI epoch before clean exit. Background banner/status commands are not driven.

This proves restored model/UI history and ownership, not terminal rendering,
new model inference, worker termination, arbitrary noncanonical writers, replay
of in-flight work, or installed launcher/supervisor behavior. No production code
or containment authority is changed.

## Supported invocation

Run on Darwin with CGO enabled (Go version from `source/server/go.mod`):

```sh
ROOT="$PWD"
BIN="$(mktemp -d)/recovery-server.test"
(cd source/server && go test -c -o "$BIN" ./cmd/cercano)
(cd source/clients/cli && CERCANO_RESUME_PROBE_BINARY="$BIN" go test ./internal/ui -run '^TestFullNativeRecoveryAcceptance$' -count=3 -timeout=180s)
(cd source/server && go test -race -c -o "$BIN" ./cmd/cercano)
(cd source/clients/cli && CERCANO_RESUME_PROBE_BINARY="$BIN" go test -race ./internal/ui -run '^TestFullNativeRecoveryAcceptance$' -count=2 -timeout=180s)
rm -f "$BIN"
rmdir "$(dirname "$BIN")"
```

The test skips without the explicit binary and is Darwin-build constrained.
`.github/workflows/ci.yml` runs normal repetition and race-instrumented CLI **and
children** in `recovery-darwin-acceptance` on `macos-14`, retaining the component
regressions and process-exit bridge. Hosted CI execution is not claimed here.

## Landed status and historical verification, 2026-11-13

Full native acceptance landed in local main at
`f736b1b43a214990ba0f4141a12edec1af8f8c11`, following independent review
with no blockers. The reviewer independently passed three normal and two
race-instrumented executions. The remaining audit diagnostics and retry fixes
landed at `a08f94f1f7afb66d866588ea86eea6e25c05905f`.

The following verification was performed before landing, not rerun for this
documentation correction:

On Darwin, normal acceptance passed three repetitions and the race-instrumented
CLI plus children passed two. Relevant CLI recovery/bridge integration tests,
full CLI suite, full server suite (`go test ./... -count=1`), server build
(`go build -o bin/cercano ./cmd/cercano/`), and race component regressions for
native identity/inventory, authenticated adapter, and asynchronous release passed.
The full server suite emitted existing macOS Keychain deprecation warnings.
That verification did not deploy, restart, or push; the subsequent local landing
and completed review are recorded above. Hosted CI execution remains unobserved,
and cross-version/architecture macOS private API qualification remains open.
Registry-rescan cleanup is optional and deferred. Older hosts that cannot freshly
authenticate intentionally fail closed. The earlier historical parallel-suite
failure remains unclassified despite later full-suite passes; see
[bounded observation](bounded-resume-observation-blocker.md).
