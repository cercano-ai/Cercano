# Bounded credential-value compatibility — PASS

TEST ONLY, executed 2026-10-22, Go **1.26.3 darwin/arm64**. This qualifies only
these value-level scenarios, not full compatibility or release/startup activation.

## Exact inputs and reproduction

- Prior: `58befd2801dd6f3d53438e8c8bfc74d937a9e811`.
- Candidate: `0b3f03709e7d0f58e775a8fd2291bb67bfdd8755`.
- Both exact local Git objects are archived (`source/server`) into temporary
  directories, without checkout or compiling the mutable production tree.
- Identical `harness.go.txt` is copied into each module's internal-import boundary
  at `cmd/compat-experiment/main.go`; byte equality is asserted before building.
  No production implementation is copied into the harness.
- `results.txt:1-18` records exact pins, runner/harness source SHA-256, archive
  SHA-256, toolchain, build commands, exits and both executable SHA-256 values.
  Executable hashes are per-run build evidence, not packaged release hashes.
  Archives/binaries and synthetic state are removed on runner exit; the exact
  Git objects plus retained harness reproduce the inputs.

Executed from repository root:

```sh
COMPAT_GOMODCACHE=/Users/bryancostanich/go/pkg/mod \
  python3 efforts/cross-platform-updates/fixtures/compatibility/credentials/run.py \
  > efforts/cross-platform-updates/fixtures/compatibility/credentials/results.txt 2>&1
```

Runner exit **0**. Both builds, all 12 scenario child invocations, and both focused
Go test commands exit **0**. No compatibility failure or dependency blocker was
observed. No failing evidence was discarded or assertions weakened.

## Assertions and real APIs

Signatures and implementations were inspected from both pinned Git sources:
`secrets.Store`, credential `New/Get/Set/Delete/List`, `BeginLogin/Commit`, both
OAuth `TokenSet.Encode/DecodeTokenSet`, and `accountidentity.Identity`.

For both empty and populated prior seeds, separate child processes execute:
prior seed → prior seed-check → candidate seed-check → candidate write →
candidate verify → prior verify. Each non-seed child reloads the exported map.
Read/check modes do not write the bridge file.

The test-owned `map[string]string` implements the actual `secrets.Store`
interface. Its JSON export is **only a synthetic bridge between processes, NOT a
production credential-store format**. All mutations go through the actual
credential service; the map is inspected for assertions/export, not used to
implement credential service or auth logic. Focused tests also exercise actual
`secrets.NewMemory` and a fake keyring error adapter (never a native keyring).

- Empty seed requires an exactly empty map/profile list.
- Populated seed contains an opaque sentinel (Unicode and NUL), opaque work key,
  deletion target, and separate work/other OAuth profiles for each provider.
- Candidate sets a new opaque key, replaces work, and deletes the target (also
  creates/deletes it in the initially empty scenario).
- Actual `BeginLogin(context.Background(), profile, provider).Commit(encoded)`
  replaces each work OAuth bundle and adds a separate profile per provider.
  Attempts are closed; no sign-in flow, token source, or refresh is invoked.
- Exact full map and profile-list comparisons require no unexpected keys or
  changes, byte-preserved unrelated opaque/OAuth sentinels, and absent deletion.
- Both actual decoders compare entire token structs against independent fixed
  fixture inputs: Access, Refresh, fractional UTC ExpiresAt, Identity Email/Name,
  and ChatGPT AccountID. Work, other and added profiles use distinct values.
  Expected encoded values use actual Encode, while decoded full-struct equality
  independently guards field loss. No copied serialization/production logic.
- Only fixed inert synthetic tokens/identities are used. Assertions report labels,
  not credential values. Fixed expiration times are data, not freshness claims.

## Focused offline tests and isolation

Both archived modules run the exact anchored allowlist printed in `results.txt`:
2 secrets tests, 5 credential login ownership/cancellation/profile tests, and
one TokenSet round-trip test for each auth package (9 top-level tests per pin).
Selected test bodies were inspected at both pins. Full auth/credentials suites
are deliberately NOT run because other tests open HTTP listeners or perform
refresh scenarios. No socket/listener, provider request, or native keyring call
is made by the selected paths.

Environment is explicitly constructed rather than inherited: `GOWORK=off`,
`GOPROXY=off`, `GOSUMDB=off`, `GOENV=off`, `GOTOOLCHAIN=local`, readonly module
builds, existing explicitly supplied module cache only, temporary build cache,
HOME, XDG config/data/state/cache, USERPROFILE, APPDATA, LOCALAPPDATA and TMPDIR.
Global/system Git config is disabled. No database is opened; all synthetic state
paths are under the temporary root. No download or normal startup occurs.
This is inspected trusted-code isolation, **not an OS network/filesystem sandbox**.
Subprocess groups have timeouts and are killed on timeout, as in preceding fixtures.

## Explicit non-qualification

Native macOS Keychain, Windows Credential Manager and Linux Secret Service,
permissions/ACLs, signing/code identity, authorization prompts, native persistence,
locked/unavailable stores, startup migration/relocation, packaged release binaries,
refresh/rotation/provider acceptance, crash durability and concurrent mutation
are **NOT qualified**. Config-coupled profile activation/removal/recreation and
startup relocation semantics remain outside this fixture. A successful Commit
here proves only the service's value write, not successful external authentication.

No production format IDs, compatibility declaration, release gate wiring or
activation authorization are created. Full persistent-state compatibility is
**NOT qualified**; the parent synthetic rehearsal and other bounded fixtures do
not change that limitation. No production changes, push or merge were performed.
