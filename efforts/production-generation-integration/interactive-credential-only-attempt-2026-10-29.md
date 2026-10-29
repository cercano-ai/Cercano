# One supervised interactive credential-only attempt — 2026-10-29

## Result: unknown / timeout; stopped

The sole authorized interactive native invocation timed out at **120.008 seconds**. No completed numeric OSStatus was returned; status is **unknown**, not 0, denial, or missing. Python `subprocess.run(timeout=120)` killed and waited only its owned probe. No retry or further native query was performed. Progress immediately before execution announced the exact new build and impending prompt opportunity. No prompt was inspected or automated, and no Always Allow selection was made by this work. Whether macOS displayed a prompt, or a user acted on one, is not established by this observation.

Read `AGENTS.md` and `normal-home-isolation-fix-2026-10-29.md`. No whole-app startup, inference, login, refresh, credential enumeration, Set/Delete/ACL/search-list mutation, install, push/merge, or existing-process change was performed. No credential contents were obtained by this work. The timeout does not establish whether the OS completed an internal read before process termination.

## Minimal source change

The existing early `acceptance-read-existing-credential` command accepts either no option (noninteractive, unchanged default) or exactly `--interactive`. Unknown, duplicate, and additional arguments return numeric `-50` without querying. Ordinary commands are untouched. Native interaction remains possible only in `realcloudacceptance && darwin && cgo`; other builds fail closed with `-4`. The existing dispatch in `main.go:1031` remains before normal startup.

The tagged native implementation sets user-interaction permission according to the explicit option, aborting if that call fails. It still queries exactly generic-password service `cercano`, account `openai-responses`, with one-result limit and data return. C immediately releases any result without inspecting its contents or copying it to Go. Only an integer crosses the Go boundary. No native mutation or enumeration was added.

The native boundary now uses `secrets.CheckAllowed()` before entering C: disabled (`1`), empty, and malformed credential policy settings fail closed. Absent or explicit `0` retains the existing allowed policy. Tests use injected fake numeric functions; no native credential access occurs in tests.

## Offline focused verification

Both commands exited 0, with `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off` and `CGO_CFLAGS=-Wno-deprecated-declarations`:

```text
go test -tags realcloudacceptance ./cmd/cercano -run TestCredentialAcceptance -count=1 -timeout=60s
# ok cercano/source/server/cmd/cercano 0.617s
go test ./cmd/cercano -run TestCredentialAcceptance -count=1 -timeout=60s
# ok cercano/source/server/cmd/cercano 0.496s
```

Coverage includes fake numeric statuses, default versus explicit interaction dispatch, strict option rejection, ordinary-command passthrough, and disabled/malformed policy refusal before the injected native boundary. These are focused tests, not the whole suite. Existing toolchain caches were used; compilation is not claimed OS-sandboxed.

## New actual immutable signed generation (not old A)

Built and published both server and CLI using the existing `source/server/scripts/publish-dev-generation.py publish --sign` publisher, with a private wrapper adding `-tags=realcloudacceptance` only to `go build`. No diagnostic substitute, installation, activation, or relabeling of old A. Publisher structural validation does not execute the application. Native deprecation warning only; publication succeeded. Server signature additionally verified with `codesign --verify --strict`; manifest server hash matched immediately before execution.

```text
root: /private/tmp/cercano-interactive-generation-zvdnzsvg
generation: generations/normal-home-interactive-credential-20261029
build_id: c4a75604-2310-4a22-9c2f-2c5f95e80834
fingerprint: 7f1c68d65f39f501a8d075cd8aeb7a246ddeede90a2f84a41efa40bf5a221ef2
source_commit: 72f05d5de6b12daa3f16d6b11c513689c7ca1415 (dirty source snapshot)
server sha256: 33e6e3f2894841b51376a80370f6c9800a9aa193af8095475ae7a29a84706076
CLI sha256: 7c8162ca61447d7e52637deb3f07d02a9bb4ab7dd3110b0d891161cdf793045f
go-wrapper sha256: 14d12b659841d18ad1e3b75d6bc69ac1a38cf692f91008c6d72d748d22e5b169
```

The dirty snapshot includes the pre-existing plan edit, which is excluded from this checkpoint. This build's observation cannot establish old A's executable-specific access behavior. Published generations are immutable by publisher convention, not owner-tamper-proof.

## Exact bounded execution

Only one application invocation was made in this work:

```text
[<root>/generations/normal-home-interactive-credential-20261029/cercano,
 "acceptance-read-existing-credential", "--interactive"]
```

Fresh environment was constructed from scratch, containing only unchanged normal `HOME`, fixed system `PATH`, private `TMPDIR`, `XDG_CONFIG_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`, `XDG_DATA_HOME`, `XDG_RUNTIME_DIR`, `CERCANO_SEARCH_SCRIPT_DIR`, `CERCANO_ANOMALY_LOG`, `CERCANO_DEVELOPMENT_CONFIG_AUTHORITY`, and explicit `CERCANO_DISABLE_CREDENTIALS=0`. No inherited CERCANO variables, provider keys, GODEBUG or GOTRACEBACK were carried through. CWD and all supplied writable runtime paths were under `<root>/probe-private`; anomaly and authority paths were non-existing private files. No ordinary configuration/history was inspected.

Private tree after timeout contained only the seven precreated empty directories (`cache`, `config`, `data`, `runtime`, `search`, `state`, `tmp`). Sanitized record: `<root>/interactive-probe.json`; full publisher identity: `<root>/generations/normal-home-interactive-credential-20261029/manifest.json`. Timeout output is not interpreted as an OSStatus. The private-tree check is not a machine-wide filesystem trace; macOS security services may do internal bookkeeping. The early command is not approval for whole-app normal-HOME execution, and no real-cloud acceptance passed.
