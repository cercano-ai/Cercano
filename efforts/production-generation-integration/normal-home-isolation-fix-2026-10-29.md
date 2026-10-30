# Normal-HOME search isolation fix and actual-generation credential probe

## Result

**Search-script escape fixed; credential acceptance blocked by access denial.** The one native read-only query from the new actual generation returned `-25293` (`errSecAuthFailed`), not missing and not success. Stopped on denial. No retry, dialog approval, login/refresh, Keychain Set/Delete/ACL/search-list change, provider request or inference followed. No credential data was obtained, inspected, copied to Go, printed or persisted.

Read `AGENTS.md`, `normal-home-isolation-blocker-2026-10-29.md` and `native-keychain-observation-2026-10-29.md`. The earlier helper and old generation A remain distinct from this new executable.

## Source and regression

`EnsureSearchScript("")` now checks an explicit `CERCANO_SEARCH_SCRIPT_DIR` absolute directory. An explicit function argument still wins; an unset/empty override retains `DefaultScriptDir()` unchanged, including its historical disregard of XDG. Relative environment overrides fail closed. This is an explicit directory seam, not a filesystem sandbox; callers must provide a trusted private directory.

The temporary-HOME regression reproduced the original escape before the implementation in the carried work. It was also independently replayed against unmodified HEAD source using a Go overlay, without reverting working source: `TestSearchScriptPrivateOverride` failed because it returned `<temporary HOME>/.config/cercano/scripts/ddg_search.py` rather than the private override, despite private XDG_CONFIG_HOME. Replay output is retained privately at `/private/tmp/cercano-credential-generation-qg2olttf/pre-fix-replay.txt`.

New tests cover the private override without HOME writes, unchanged default, explicit argument precedence and relative-override refusal. Credential dispatch tests inject numeric statuses, reject extra arguments without querying, leave ordinary commands alone, and exercise credentials-disabled refusal. Native data access is not run by these tests.

Offline verification (exit 0, credentials disabled, GOPROXY/GOSUMDB off, deprecated native warnings suppressed):

```text
go test -tags realcloudacceptance ./internal/web ./cmd/cercano ./internal/llm/responses \
  -run 'TestSearchScript|TestCredentialAcceptance|TestAcceptance' -count=1 -timeout=60s
# web 0.177s; cmd/cercano 0.606s; responses 3.225s

go test ./internal/web ./cmd/cercano \
  -run 'TestSearchScript|TestCredentialAcceptance' -count=1 -timeout=60s
# web 0.243s; cmd/cercano 0.487s
```

Existing acceptance HTTP tests use offline fake transports; no live inference was exercised. These were focused suites, not `go test ./...`. Go compilation used existing ordinary toolchain caches; no claim that build tooling was OS-sandboxed or that every compiler cache path was private. Application test data uses temporary directories; probe runtime writable paths are private.

## Probe-only startup-write audit

`main.go:1030-1033` dispatches `acceptance-read-existing-credential` before command dispatch, flags, settings/config/history reads, host composition, workers, MCP, model/runtime construction, logging and inference. Extra arguments emit `-50` without a query. Ordinary builds/non-Darwin/non-CGO builds return `-4`; native querying requires `realcloudacceptance && darwin && cgo`. `CERCANO_DISABLE_CREDENTIALS=1` returns `-4` before native access even in that build.

The build-tagged C query disables user interaction before querying and aborts if disabling fails. It uses exactly generic-password service `cercano`, account `openai-responses`, one-result limit and data return; it releases the result immediately, without inspecting its contents. It has no enumeration, write, refresh or network code. The Go boundary receives only an integer. Process exit status is not the credential outcome: stdout's OSStatus is authoritative.

Reachable package initialization was examined separately from `main`: main init only trims version; builtins init registers a catalog callback (does not execute it); vendored Keychain/file/pass inits register backend openers (do not open stores); generated protobuf init constructs descriptors. Local package global initialization includes regexes, lazy tokenizer setup, process-random usage identity, and platform executable-name selection, not configuration/history opening. The dependency init scan found standard/library registration, networking/libc tables and related setup; it is not by itself a sandbox proof. The actual credentials-disabled generation execution additionally verified the early return path: numeric `-4`, empty stderr, no files created under any supplied runtime directory.

Previously identified writes are excluded by this early return, not all repaired: `setupDispatchLogFile` can write `~/.cercano-dispatch.log` on ordinary dispatch paths; anomaly logging needs its own override; normal host settings/database/telemetry/accounting, worker resources, MCP children, local-runtime/model caches and ordinary handler writes remain outside this probe's path. The explicit search-script seam fixes the known tool escape but **does not qualify a normal-HOME whole-app/inference launch**. Canonical catalog/enterprise/activation guards were not changed or bypassed.

## New immutable actual generation identity

Published build-only through `source/server/scripts/publish-dev-generation.py`, not a separately built diagnostic executable. Publisher structurally validates without executing the application. A private Go wrapper adds `-tags=realcloudacceptance` to `go build`; its SHA-256 is recorded in the manifest. Both server and CLI were published together, ad-hoc signed, without installing or activating them.

```text
root: /private/tmp/cercano-credential-generation-qg2olttf
generation: generations/normal-home-credential-20261029
build_id: 738e8a1d-751f-4bca-bc31-5dba1f8be15c
fingerprint: 533cc6500f284af4e92fcc44562c1eec0e966c2308958adcfa4ab9012d9e82da
source_commit: da790cf8713097b235cd90bf6e09c8de1cd04574 (dirty source snapshot)
server sha256: f942bb2f05f6657fa3ff9602c39753a6517cacfa32b76a4eee120868b396a6bb
CLI sha256: caebaabcd4256ee247ad0fd02c8b105d9c4374618ecb0591cdc57a021e03002c
go-wrapper sha256: 8111c487e04c0e355a7fa4b9f26d1c17feff0ae83c193e88dd06aa45f57204cb
```

Manifest and server hash checked before execution. This is **new generation normal-home-credential-20261029**, not old A; its denial cannot establish A's executable-specific ACL behavior. The dirty build includes the pre-existing plan edit; this checkpoint does not stage that edit. No reproducible-binary equivalence claim is made.

## Exact bounded execution and sanitized observations

Only argv `[<new generation>/cercano, "acceptance-read-existing-credential"]` was executed, using Python `subprocess.run(..., stdout=PIPE, stderr=PIPE, timeout=12)`. CWD was the private `probe-private` directory beneath the root above. HOME remained inherited and unchanged. Inherited CERCANO_* variables were cleared, then TMPDIR, XDG config/state/cache/data/runtime and CERCANO_SEARCH_SCRIPT_DIR were set to private subdirectories; CERCANO_ANOMALY_LOG and CERCANO_DEVELOPMENT_CONFIG_AUTHORITY pointed at private non-existing files. No ordinary configuration/history was read. GODEBUG/GOTRACEBACK were removed. The native branch itself does not consult those config paths.

| Execution | stdout status | exit | elapsed | stderr | deadline |
| --- | ---: | ---: | ---: | --- | ---: |
| Credentials disabled (`=1`) | -4 | 0 | 0.568s | empty | 12s |
| One native query (`=0`) | -25293 | 0 | 0.026s | empty | 12s |

Native stdout was strictly numeric plus newline. Neither call timed out. Private runtime tree contained only the seven precreated empty directories; native query left it unchanged. Sanitized records: `<root>/disabled-probe.json` and `<root>/native-probe.json`. This checks the supplied private tree, not a machine-wide write trace. OS Keychain/security services may perform their own internal bookkeeping; no application mutation API was called.

## Boundaries

No whole-app startup, provider inference, live cutover, install, push, merge or existing-process change. No ordinary configuration/history inspection beyond the already authorized fixed profile identity. No secret transfer or credential persistence. No ACL remediation attempted: allowing access would require a separate security decision/authorization. Preserve the unrelated `efforts/production-generation-integration/plan.md` modification and exclude it from this checkpoint. Real-cloud acceptance remains blocked, not passed.
