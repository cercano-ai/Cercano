# Coordinated worker export guard evidence

Reproduction added before the guard (2026-10-28). With a memory-only coordinated
counting getter, the focused probe failed: static messages/responses, Bedrock,
and unknown/proxy profiles exported the fake raw token and called Get once;
subscription profiles returned untyped/failoverable source errors. Both Service
facades and readonly-plus-Service facades reproduced the issue.

After the guard, the same probe passes, as does the race run: all coordinated
profiles return empty token/account and a sanitized, non-failoverable existing
CredentialError before Get/token-source resolution. Ordinary memory fixture
export still succeeds through both facades. A source-AST assertion checks that
the actual production host SnapshotConfig call still passes an empty raw
resolved credential; it is not a spawned-worker integration test.

Exact commands, from source/server (both passed after the change):

```sh
CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off go test ./internal/worker -run '^TestWorker(CoordinatedCredentialExportRejected|OrdinaryCredentialExportPreserved|ProductionSnapshotHasNoRawCredential)$' -count=1
CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off go test -race ./internal/worker -run '^TestWorker(CoordinatedCredentialExportRejected|OrdinaryCredentialExportPreserved|ProductionSnapshotHasNoRawCredential)$' -count=1
```

Only fully fake test scopes enable credential policy; no native credentials,
network, or child processes are used by these probes. No IPC, Service.Get
restriction, factory/Anthropic edits, activation, adoption, push, or merge.
