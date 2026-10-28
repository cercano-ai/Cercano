# Shutdown scheduler admission completion

Invocation-owned `workadmission.Gate` linearizes entry against fencing; its lock never spans callback work. Fence does not join. Already-entered callbacks retain execution authority until their owner joins them.

- Catalog refresh and **each** warm resolution enter independently. Fence stops both schedules; Stop joins loops and admitted callbacks. Scheduling has a separate short-held lock so cache persistence cannot delay the fence. Start cannot resurrect a fenced manager.
- Retention fences at ordinary stopAdmission, then cancels its schedule; closeData joins both the loop and admitted sweeps before closing the store. The initial timer is stopped on cancellation.
- Config and permission watcher event callbacks have entry gates, closing the ready-event-vs-cancel select gap. Watcher owners join callbacks before disposal.
- Provider rebuild enters **after** acquiring rebuildMu. A rebuild queued behind an entered builder therefore cannot resolve config, credentials, or construct providers after the fence. The admitted builder may finish and publish; the owner waits for it.
- Catalog joins precede MCP/server dependency disposal. Watchdog/admitted-turn behavior, development dispatch, prepared activation, UI, and proto are unchanged by this work. Activation remains disabled; no helper or force termination was added.

## Executed evidence (from source/server)

Passed, including 20 repetitions of deterministic channel-barrier regressions:

```sh
go test -race ./internal/ollamacatalog ./internal/retention ./internal/hostsvc/providers ./internal/server -run '^TestShutdownAdmission' -count=20 -timeout=120s
```

Coverage: active refresh and warm joins; no second fetch after fencing; cache-lock-independent catalog fence; no restart after fence; active destructive sweep joined; queued rebuild cannot even dereference config (therefore zero credential/provider openers); entered provider builder survives and publishes; selected watcher event rejected after fence; entered watcher joined; sibling watcher remains independent. Fixtures use fake HTTP transport, memory credentials, fake stores and channel barriers, not native providers, network, user data, or 30-second timer sleeps.

Also passed:

```sh
go test -race ./internal/ollamacatalog ./internal/retention ./internal/hostsvc/providers ./internal/hostsvc/permissions -count=1 -timeout=120s
go test -race ./cmd/cercano -run 'TestProduction(RemoteStatusDuringBlockedCallback|IncompleteShutdownControl|FailureStages|ListenerFailureClosesOwnedStore)|TestProduction.*Catalog' -count=1 -timeout=120s
```

The focused production command was rerun after the final lifecycle integration edits. This evidence is focused, not a claim of a repository-wide test pass.
