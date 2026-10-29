# Native read-only diagnosis: HOME changes default keychain visibility

## Finding and scope

The controlled one-shot helper isolates a **HOME-dependent native lookup
failure**, not a file-backend fallback. With the same executable, inherited
process context, service/profile, and native no-interaction setting, changing
only HOME removes the native default keychain and makes this target genuinely
not-found. Restoring normal HOME restores metadata visibility. This explains
why reauthentication in the ordinary agent need not be visible to a process
launched with private HOME.

Confidence is high for this causal mechanism in the helper and strong for its
relevance to the reported isolated generation. This is **not** a native trace
inside generation A, nor proof of generation A's executable-specific ACL/data
access. No host/provider was launched and no existing process was restarted.

## Observed results

All helper invocations exited 0, well inside a 12-second subprocess deadline.
`SecKeychainSetUserInteractionAllowed(false)` returned OSStatus 0 in each;
there was no authorization approval. Keyring debugging was explicitly false.

| Context | Default keychain status | Search-list count | Backend | Target metadata | Target data-query status/count |
| --- | ---: | ---: | --- | --- | --- |
| Ordinary HOME | 0, path under HOME | 2 | keychain | 0 / 1 | -25293 / 0 |
| First private root as HOME | -25307 | 1 | keychain | -25300 / 0 | -25300 / 0 |
| Second private root as HOME | -25307 | 1 | keychain | -25300 / 0 | -25300 / 0 |
| First private root's `home` subdirectory | -25307 | 1 | keychain | -25300 / 0 | -25300 / 0 |
| Second private root's `home` subdirectory | -25307 | 1 | keychain | -25300 / 0 | -25300 / 0 |
| Ordinary HOME, repeated last | 0, path under HOME | 2 | keychain | 0 / 1 | -25293 / 0 |

Native status meanings: `-25307` = errSecNoDefaultKeychain;
`-25300` = errSecItemNotFound; `-25293` = errSecAuthFailed.
Actual replacement-adapter `GetMetadata` also succeeded normally and classified
missing in every private-HOME context. The native queries use exactly the
adapter's fixed service/profile, generic-password class, one-result limit,
attribute return, and metadata/data flags. They do not set a keychain path,
just as production `secrets.OpenKeychain` does not.

The helper's ordinary-HOME data query was **denied**, not successful. Therefore
this investigation obtained no credential data. It never copies native result
data into Go, emits attributes, or formats native errors; returned native
objects are released without inspection. No unrelated items were queried.
The search-list probe returns only its count; the default-path probe emits only
a boolean comparison against HOME. No environment/configuration dumps were
read. No Set/Delete, refresh, login, settings/ACL/search-list mutation, or
inference occurred. The interaction setting is confined to each helper process.

## Comparison with original adapter and fallback hypothesis

Production opens `keyring.Config{ServiceName: "cercano",
KeychainTrustApplication: true}` without KeychainName. The Darwin opener returns
a keychain adapter without checking native default-keychain availability.
`Get` then queries the native default search context. `keyring.Open` can try
another backend only when an **opener** fails, not when `Get` later fails.
The helper observes the actual selected adapter type: **keychain**, in all
contexts. An ordinary encrypted-file keyring fallback is not implicated.

For the private-HOME native `-25300` / zero-result outcome, both the original
adapter and checkpoint `a0531bda19ac42fe4758bcf0e19066cf9c6e6b18` correctly
return ErrKeyNotFound; production maps that to fs.ErrNotExist / missing.
The patch cannot fix the absent default keychain/search-context problem.
For the ordinary-HOME `-25293` / zero-result data query, the original adapter
would incorrectly return missing, while the patch preserves the native error.
That second result demonstrates the original bug's relevance without claiming
it caused the private-HOME result.

## Identity boundary and stopping point

`codesign -dvv` was captured privately and reduced to only: exit 0,
helper ad-hoc signature true, team identifier present false. No signing/account
identity was emitted. The helper is **not** generation A, even though the
prior run also described A as ad-hoc signed. The helper's ordinary data denial
cannot establish whether A or the existing agent is trusted, whether their
code requirements differ, or whether a prompt would have been required.
No prompt was enabled to resolve that question.

A is not instrumented here; it may have additional environmental differences.
Nevertheless HOME alone is sufficient to reproduce the native missing result,
and reversing HOME restores metadata visibility without reauthentication.
The exact search-list members and per-item ACLs were deliberately not exposed.
Testing data access with the actual generation identity, changing its launch
HOME/configuration, or supplying a specific keychain would be a separate
boundary. Do not fix this by copying tokens, logging in again, changing ACLs,
or modifying the user's keychain/search lists. No such action is necessary to
identify the demonstrated lookup mechanism.

## Exact commands and artifact handling

From `source/server`, a Python `subprocess.run` wrapper built the private
helper (90-second timeout) using this argv and environment overrides; the
binary's parent was created with `tempfile.mkdtemp` and mode 0700:

```text
argv: go build -o /private/tmp/keychain-diag-ptd84lmz/probe ./cmd/keychain-read-diagnostic
CGO_ENABLED=1 GOPROXY=off GOSUMDB=off CERCANO_DISABLE_CREDENTIALS=0
CGO_CFLAGS=-Wno-deprecated-declarations
```

An irrelevant misspelled `CG0_ENABLED=1` was also present in the build wrapper;
Go used the correctly spelled `CGO_ENABLED=1`. Build exit 0. No module-cache
patching or downloading occurred; the existing module replacement and pinned
go-keychain dependency were used.

Each query was exactly argv
`["/private/tmp/keychain-diag-ptd84lmz/probe"]`, run with
`subprocess.run(..., stdout=PIPE, stderr=DEVNULL, timeout=12)` and inherited
normal environment plus `CERCANO_DISABLE_CREDENTIALS=0`. Sequential HOME
variants were:

```text
normal (HOME unchanged)
/private/tmp/rc-m6lgc0vh
/private/tmp/rc-lby96zxz
/private/tmp/rc-m6lgc0vh/home
/private/tmp/rc-lby96zxz/home
normal (HOME unchanged, repeated last)
```

Before querying the `home` variants, the wrapper checked only
`os.path.isdir(root + '/home')`; both existed. Signing inspection was argv
`["codesign", "-dvv", "/private/tmp/keychain-diag-ptd84lmz/probe"]`, timeout 8,
with both streams captured and only the booleans above emitted. Query stdout
was already sanitized by the helper. No raw query/signing output or environment
dump was persisted. The retained private binary contains code, not credentials.
The repository retains only diagnostic source and this sanitized evidence.

Afterward: `gofmt -w cmd/keychain-read-diagnostic/main.go`. The gofmt operation
changes formatting only; the executed helper was built before formatting.

Non-native regression/compile verification also passed (exit 0):

```sh
CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off CGO_ENABLED=1 \
 CGO_CFLAGS=-Wno-deprecated-declarations \
 go test github.com/99designs/keyring ./internal/secrets \
 ./cmd/keychain-read-diagnostic -count=1 -timeout=30s
```

The diagnostic command has no tests; this compiles it without executing main.
