# Failed native-controller residual cleanup

## Outcome

The specifically authorized residual **modelhost PID 99649** from `/private/tmp/nc-1000385746` was successfully stopped through the existing authenticated `ModelHost.Shutdown` gRPC API. Post-call observation found no PID 99649 (`ps` exit 1, empty output), and connecting to its exact endpoint `127.0.0.1:64201` returned connection refused (Darwin errno 61). This closes only this residual cleanup; it does not establish broader rehearsal acceptance.

## Ownership and read-only verification

Read `native-controller-notes.md`, the modelhost protocol, registry, verified-dial implementation, and server/command shutdown implementations before acting. The note identifies this failed fixture and residual (lines 35–49).

- Exact-PID `ps`: PID 99649, PPID 1, UID 501, started November 5, 2026 at 16:13:55; command `/private/tmp/nc-1000385746/Cellar/cercano-agent/A/bin/cercano model-host`.
- Exact-PID `lsof -a -p 99649 -d txt -Fn` mapped that fixture executable. Exact-PID listener inspection identified TCP port 64201.
- Fixture root, `.cercano`, `run`, and `modelhost` directories were non-symlinks owned by the current UID 501 with mode 0700. The private record was a non-symlink owned by UID 501 with mode 0600.
- Explicit RunDir: `/private/tmp/nc-1000385746/.cercano/run/modelhost`. Record: PID 99649, endpoint `127.0.0.1:64201`, API 1, version `dev`, started `2026-11-05T16:13:55.891219-08:00`. Token length 64; token not printed or committed.
- Fixture `built-cercano` and keg executable SHA-256 both matched `4b7fc086caaa4b26e721576e5714f1284ec23058336f0570a11c7c6122dbb04a`.
- Fresh nonce/HMAC-SHA256 Hello proved endpoint knowledge of the private record token before any token-bearing request. Hello also matched PID 99649, API 1, and version `dev`; no persistent coexistence-protection header was asserted.
- Authenticated read-only `Instances` returned `[]`. This was the modelhost control process, not a real model server. No model start or inference was requested.

## Bounded action and evidence

A one-shot helper, retained at `/private/tmp/nc-1000385746/residual-cleanup-helper.go`, used the existing generated ModelHost API. It hard-coded the exact fixture RunDir, PID, endpoint, version, and start time; checked private-path ownership/modes; checked exact-PID executable and listener identity again immediately before shutdown; required an unchanged record and empty instances; and allowed only one transport, refusing reconnect. Its total RPC context was bounded to 15 seconds. It used no `Ensure`, launcher, replacement, attachment, global process inventory, ambient modelhost discovery, keychain, or live configuration.

Executed offline from `source/server` with `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=readonly -p=2' GOMAXPROCS=2`; `go run` exited 0 in 1.358 seconds. Output: verified Hello; `Instances: []`; `Authenticated Shutdown: success`. No protection denial occurred and no bypass was attempted.

Before Shutdown, the helper preserved the exact private record as `/private/tmp/nc-1000385746/residual-modelhost-record-evidence.json` with mode 0600. The server's ordinary shutdown removed its discovery record itself (`cmd/cercano/modelhost.go:171–175`); cleanup did not manually delete any state or evidence. The fixture, private record backup, helper, modelhost log, config, conversations database, and keg executable remain. No raw PID signals, source/plan/verification edits, or fixture removal were performed. Only this effort note is checkpointed.
