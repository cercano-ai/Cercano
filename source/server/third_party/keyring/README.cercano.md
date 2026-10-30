# Repository-owned keyring patch

Source: `github.com/99designs/keyring v1.2.2`

- Module checksum: `h1:pZd3neh/EmUzWONb35LxQfvuY7kiSXAq3HQd97+XBn0=`
- go.mod checksum: `h1:wes/FrByc8j7lFOAGLGSNEg8f/PaI3cgTBqhFkHUrPk=`
- Upstream license retained in `LICENSE`.

The server's `go.mod` replaces that module with this directory. Production Go
sources and module manifests are copied from the pinned release; upstream CLI,
fixtures, and tests (which can access native credential stores) are omitted.
Non-Darwin backends are unchanged. No module-cache changes are required.

## Local change

Darwin `Get` and `GetMetadata` checked `ErrorItemNotFound || len(results) == 0`
before handling other query errors. Native errors with empty results were thus
reported as `ErrKeyNotFound`. Check the native error first, preserving its exact
identity, and only treat an error-free empty result as missing. Existing query
construction, data mapping, write, delete, and list behavior is unchanged.

Result decoding is extracted into functions taking fake query results, allowing
tests without native queries. The patch applies to all server keyring users,
including secrets and enterprise, without introducing a separate native adapter.
When updating upstream, compare this change before removing the replacement.

## Offline regression command (Darwin with cgo)

From `source/server`:

```sh
CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off CGO_ENABLED=1 \
  go test github.com/99designs/keyring -run '^TestKeychainFakeQueryResults$' -count=1
CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off CGO_ENABLED=1 \
  go test ./internal/secrets -count=1
```

The nested replacement module is not covered by `go test ./...`; run its explicit
command above. Its tests use only synthetic result structs and errors, not native
keychain APIs or stored values. This proves error preservation, not the cause of
any particular host's native query failure.
