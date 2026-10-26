# Compiled acceptance scope

The build-tag-only `devhostfixture` independent dispatcher now invokes the same configured, pinned worker runner used by foreground execution, rather than waiting in the host. It accepts only the fixed synthetic task with the single `fixture_read` capability; there is no unrestricted fallback. The independent conversation receives the plan profile without the fixture launch capability. The synthetic provider blocks inside the worker process and writes its build and PID; acceptance rejects host/test PIDs and requires generation A.

The barrier must be the canonical, absolute declared fixture root (a deliberately narrower rule than arbitrary descendants). Host startup rejects malformed/outside paths before server startup. Worker marker creation rejects existing barrier entries. Tests probe relative, traversal, and outside paths with zero modelhost entries. Credential refusal remains independently tested. Existing modelhost `ensure.go` checks loopback IP literals; no duplicate production restriction was added.

Acceptance explicitly checks successful allowed-tool completion and error-marked plan-profile refusal. It requires busy destination prepare to succeed and source release/relinquish to return the intended admission-busy message. It cancels the foreground request context **after foreground EOF**, while the independent worker is at its barrier, then verifies the task remains running across unrelated-conversation handover. This is detached-task survival, not cancellation of an actively streaming foreground turn or a physical transport-disconnect test.

After barrier release and notification drain, acceptance parses completion payload records (not merely matching turns), requires exactly one completed notification for the independent task, and exactly two completed, nonpending task records (one seeded, one executed). It does not infer future absence of duplicates from an arbitrary sleep.

Validation in this run:
- Initial subprocess-runner implementation: focused compiled execution passed.
- Strengthened allowed-read assertion first failed because synthetic tool result text is not the event summary. Replaced this assumption with typed successful tool-completion assertions; profile rejection checks the typed error flag and exact summary.
- Final `go test ./cmd/cercano -run 'TestDevHostCompiled' -count=1` passed (16.341s package time).

No external provider, installation, real user configuration, credential loading, or external network is exercised. Local authenticated loopback modelhost/control transport and worker Unix sockets remain required. This note does not claim a pre-change baseline failure or modify the separately owned metadata-fix evidence.
