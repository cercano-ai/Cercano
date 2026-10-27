# Everyday launcher execution coordination

User authorized implementation and local landing, not live cutover. Do not execute or modify `/Users/bryancostanich/bin/cercano`. No native credentials or real-model inference in verification. Preserve existing sessions and unrelated files. This is execution of the agreed workflow, not a new planning effort.

Parallel ownership:
- Integration worker: launcher orchestration, production host and CLI integration, installer source, entrypoint acceptance.
- Publication worker: `publish-dev-generation.py`, publisher tests, signing helper. Existing publish CLI retained; adding `fingerprint --repo ...` for stable input-derived generation selection. Canonical manifest remains `build_id` plus `binaries.cercano.sha256` (not fixture-only Build/SHA256).
- Socket worker: `internal/worker/policy.go`, `spawn.go`, focused path-limit tests. Bounded opaque socket names and early size validation; caller supplies short private host resource root.
- Main: coordination, independent review, verification and delegated local landing.

Production baseline hazards established by read-only audit: `dev-host` is intentionally a synthetic fixture startup, not normal provider wiring. `SelectExecutionMode` constructs legacy unpinned runners and must not overwrite prepared policy. `PrepareCoexistence` refuses existing stores; ordinary startup must not stamp or migrate legacy data. Explicit transition must not infer legacy death from failed connectivity or acquire writable state silently. Any proposed migration or changed user-data/security semantics requires approval rather than assuming this brief permits it.

Verification must drive the maintained ordinary launcher, with injected synthetic dependencies clearly identified. No real-provider claim follows from these tests. Installed readiness remains false until a separately approved cutover.
