# Bounded first-prompt isolation verification

2026-10-29. This is preparation evidence, not real-cloud acceptance or a filesystem sandbox qualification.

## Executed checks

From source/server, with CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local:

```
go test -race ./internal/worker -run '^TestPreparedRoutingLogFullAssembly$' -count=2 -timeout=60s
```

PASS, exit 0. This exercises actual prepared dependency assembly with an empty provider snapshot, injected memory secrets, temporary HOME and a default HTTP trap. It checks both private diagnostic writers and no entries in fake HOME. It does not exercise a configured provider or a real worker subprocess.

The immediately preceding parent verification also passed the focused routinglog/worker/command argument checks. Python preparation tests were independently rerun: all 12 passed using `python3 -B -m unittest discover -s efforts/production-generation-integration -p 'test_acceptance*.py' -v`. These use fake commands, not application launches.

## Source checks

- spawn.go builds the pinned worker command with --prepared, --failure-log and --routing-log; absent cmd.Env means the child inherits the host environment. runWorkerMode selects NewPrepared with those paths.
- acceptance_preparation.py returns immutable command descriptors only. It does not execute the real launcher or enforce how a later executor uses them. An executor must replace its subprocess environment with the descriptor environment, not overlay ambient variables.
- internal/chatgptauth/source.go Token returns fresh tokens directly. For expired tokens, both options.CheckMutation and secrets.CheckMutation(store) occur before flow.Refresh. This is source-order evidence, not native token freshness/access verification.
- internal/llm/openai/stream_trace.go is gated by cercano_streamtrace, not realcloudacceptance; it is not an enabled writer in the intended build.

## Limits and next gate

No application, native credential query, issuer request, model request, or live-data inspection was performed in these checks. No usage ledger was changed or reset. The source audit did not establish whole-app isolation or arbitrary-tool safety. Actual launch preparation still needs exact arguments/artifact identity, explicit use of the allowlisted environment, existing aggregate-ledger authorization/state, and bounded owned-process lifetime controls. The preparation descriptors alone are not that integration. Do not treat these results as permission to reset a halted ledger or broaden credential access.
