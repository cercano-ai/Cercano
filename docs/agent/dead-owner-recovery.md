# Dead-owner recovery (2026-11-02)

This is distinct from live-server forced cutover. A dead source cannot implement
or answer a new RPC. Recovery runs on the destination; it never requests source
shutdown and does not require the historical source to support new methods.

## Flow

The CLI first asks its authenticated default destination to inspect durable
ownership through `LifecycleRecover`. This read-only offer requires verified
source absence. It displays a default-No recovery confirmation, bound to the
exact conversation, source host and epoch, destination, and UI generation.
Confirmation repeats verification and only switches the UI after a validated
pinned connection to the recovered destination. Normal acquire still refuses
abandoned ownership. Live-source cutover remains a separate protocol.

Production Darwin verification holds the source's existing generation-launch
lock, verifies its private host record and retained start intent, then checks the
complete same-user kernel process argument inventory for the exact host ID. It
does not use a failed dial or a missing executable path as evidence of death.
A live/suspended source, unreadable process inventory, inconsistent record, or
concurrent launch refuses recovery. The lock remains held through recovery.
No signals are sent. Unsupported platforms fail closed.

Under all stable conversation-tree locks, recovery rechecks the exact durable
owner and tree inside a SQLite write transaction. It:

* assigns the destination a fresh root epoch and records an idempotence receipt
  in the existing handover receipt table;
* releases abandoned source-owned descendant epochs, refusing independently
  owned or reserved descendants;
* marks accepted/running tasks interrupted, preserving results and history;
* retains pending notifications but marks them claimed so reload does not launch
  automatic completion turns; the next explicit human boundary can inspect them;
* records an autonomy blocker rather than silently continuing an autonomous run;
* leaves unrelated conversations and their tasks untouched.

No schema upgrade, deleted locks, fake successful task completion, or blanket
ownership reset is involved. SQL failure rolls back the whole operation. An
uncertain commit retains all locks and does not admit work. A lost successful
RPC response can retry the same expected owner on the same destination without
allocating another epoch; later normal discovery also finds that destination.

## Boundaries

The process proof covers same-user generation hosts started through the existing
recorded launcher, under its launch lock. It is not a universal fence against
arbitrary custom database writers, other users, or intentionally bypassed launch
protocols. Existing epoch checks and conversation locks remain mandatory.

This does not claim that detached tools or external effects have been rolled
back or universally stopped. They are not replayed. An uncertain commit/reload
or an existing reserved handover remains a refusal, not permission to guess an
outcome. A still-live historical host lacking forced-cutover support is also
not terminated by this implementation.

## Verification

The initial disposable regression reproduced normal acquisition refusing an
abandoned `owned` row after its lock was released. Tests now cover:

* task/descendant interruption, preserved sibling work/history, fresh epochs;
* held locks, rejected death proof, stale confirmation, foreign/reserved children;
* injected SQL failure with atomic rollback and successful explicit retry;
* real localhost recovery RPC, read-only offers and idempotent retries;
* real authenticated CLI recovery confirmation and destination pinned binding;
* native kernel checks against a fixture process while alive and after exit;
* source launch exclusion and private registry/start-intent matching;
* an actual ordinary-source child-process exit, followed by explicit recovery
  and a persisted destination write (extended production resume probe).

Race suites passed for conversation, server, taskrun, agentclient, broker and
brewrestart; selected production startup/resume/registry tests and the full CLI
race suite passed. Tests use disposable state and synthetic backends. The native
process test inventories only its fixture PID; it does not qualify the user's
ambient process inventory. No full interactive PTY journey, real inference,
installation, or live conversation recovery was performed.

Final broad race suites and both binary builds: `/tmp/dr-done-m4_lawdv`.
After tightening proof acquisition to occur inside the recovery lock boundary,
the full server/conversation race suites passed again: `/tmp/dr-last-qu93m_ba`.
Focused three-repeat checks: `/tmp/dr-6da96_aq`.

Commands used for the broad gate:

```text
# source/server
go test -race ./internal/conversation ./internal/server ./internal/taskrun ./pkg/agentclient ./internal/broker ./internal/brewrestart -count=1 -timeout=120s
go test -race ./cmd/cercano -run 'TestProductionResume|TestDevelopmentRecoveryRegistryExclusion|TestOrdinaryGenerationStartup' -count=1 -timeout=120s
go build -o <temporary>/cercano ./cmd/cercano
# source/clients/cli
go test -race ./... -count=1 -timeout=120s
go build -o <temporary>/cercano-cli .
```
