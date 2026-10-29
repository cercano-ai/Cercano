# Keychain query error preservation

## Confirmed source-level cause

Baseline: `769a1a066ada755b7fa6d87a09371f02ac84c48f`, branch
`feat/production-generation-integration`.

The pinned `github.com/99designs/keyring v1.2.2` Darwin/cgo `Get` evaluated
`err == ErrorItemNotFound || len(results) == 0` before checking other errors.
An empty result plus a non-missing native query error therefore became
`ErrKeyNotFound`. `GetMetadata` had the same ordering. Cercano's
`internal/secrets/secrets.go` correctly preserves non-missing errors but cannot
recover an error already lost inside keyring. It maps `ErrKeyNotFound` to
`fs.ErrNotExist`, completing the misleading missing-credential classification.

The local module replacement preserves all native errors except the intentional
item-not-found mapping. Error-free empty results still mean missing. Both
Darwin read decoders are patched; other backend behavior is unchanged. A
repository-owned pinned copy was chosen over a parallel native implementation
so existing backend selection, query construction, and all server consumers
retain the same behavior. See `source/server/third_party/keyring/README.cercano.md`.

## Deterministic verification

With `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off CGO_ENABLED=1`:

- Restoring the original ordering temporarily makes fake denied+nil,
  denied+empty, and interaction-not-allowed+nil cases fail.
- Restoring the fix passes `go test github.com/99designs/keyring
  ./internal/secrets -count=1`.
- Fake results cover native missing, error-free empty/nil, errors with populated
  results, successful data/metadata mapping, and unchanged error identity.
- `go list -m` resolves keyring to the repository replacement.

No native queries, credentials, login, inference, agent restart, or cloud host
launch were used. Prior private run files and environment dumps were not read.

## What remains unknown

The bug proves that a native error *can* masquerade as missing; it does not prove
that this caused the reported isolated-host failure. A genuinely absent item,
a different default keychain search list under the private HOME, or native
access/interaction restrictions remain hypotheses. The reported successful
reauth on the existing agent does not establish what the isolated process can
query. The reported CGO-enabled build rules out only the simple explanation
that the Darwin backend was excluded at build time.

## Proposed next diagnostic — not executed

After explicit approval, use a one-shot query-only diagnostic built against
this patch in the same isolated process context (without starting an agent or
provider). Disable keyring debugging. Return at most: selected backend,
query success/missing/native-error classification, native OSStatus integer when
available, and result count. Do not print error strings, labels, accounts,
paths, environment, or data; discard any returned data immediately. A data query
can encounter an authorization prompt; abort rather than authorize or change
native state. A metadata-only query may be less intrusive but cannot establish
whether a subsequent data read succeeds. No login, token refresh, Set/Delete,
inference, or agent restart belongs in this diagnostic.

Compare existing and isolated contexts only if separately approved. Preserve
native status internally rather than exposing raw native errors in public
credential responses, which may contain sensitive context.
