# Partial connected updater owner / identity / drain slice

This checkpoint is **not an installed update or complete handoff**. Native macOS
arm64 `cercano-updater execute --input /absolute/private-operation/operation.json`
now opens a bounded controlled operation. It always exits nonzero. `version`
remains the only successful command. No acquisition, publication, shutdown,
kill, arbitrary command/configuration, or automatic restart is implemented.

## Fixed caller schema

The existing boot layout separates enrolled installation root from updater state
root, so the minimal explicit input includes both locators. There is no generic
registry or path map. Use a fresh canonical private directory per attempt; the
input must be a single-link owner-only regular `operation.json` (0600 on Unix).
All fields are mandatory, exact-case, unique, non-null, and unknown fields are
rejected, including inside participant identities. Total JSON limit: 64 KiB;
participant limit: 64; PID duplicates refused; exactly one agent required.

Top-level fields:

- `install_root`: canonical absolute enrolled root.
- `install_id`: enrollment ID.
- `state_root`: canonical absolute root consumed by `installcontext.BootDirectory`;
  existing protected `<root>/Cercano/updater/<id>` on macOS, not a database path.
- `operation_token`: 32 lowercase hexadecimal characters.
- `agent_endpoint`: canonical numeric loopback `address:port`, nonzero port.
- `participants`: complete initial controlled set, each `{ "role": "agent" |
  "client", "identity": <nativeprocess.Identity JSON> }`. The identity object
  uses the existing Go type's exact exported field names: `PID`, `UID`,
  `Executable`, `StartTicks`, `StartSeconds`, `StartMicroseconds`, `WindowsSID`,
  `WindowsStartFileTime`. Include platform-inapplicable zero/empty values too.
  Disconnected clients must be supplied. Completeness is still the harness's
  assertion; the updater does not discover omitted processes.
- `status_file`: **only** `status.json` is accepted. This selects the fixed sibling
  associated with `operation.json`, not any arbitrary output path. Existing
  status files are never overwritten. Use a new directory for each attempt.

`CERCANO_DISABLE_CREDENTIALS` is inherited unchanged, validated, and honored by
`DialExisting`'s existing credential-isolation attestation. Tests explicitly set
it to `1`; no live agent or keychain is used. HOME never selects credential mode.

## Connected behavior and limitations

One updater-owned installation Update barrier supplies writer serialization and
launch exclusion without SQL. The ordinary boot selection/enrollment/image
validation is reused under that existing barrier rather than acquiring a nested
Launch lease. All supplied identities are independently inspected and retained
before drain; roles must resolve to exact selected images and same-user born
lifetimes. Native `HoldsListener` binds the loopback listener to the supplied
agent before and after `DialExisting`. This is not proof of unique socket ownership.

The existing DrainForUpdate stream must return token-bound SEALED before the
small status record is written. Its state is
`owner_evidence_sealed_incomplete`, with operation token and participant count.
**This is not a TUI acknowledgement**, nor durable evidence that an owner is still
alive: it is a historical status snapshot. The updater consumes DRAINING/IDLE
and waits on the owner stream until failure, cancellation or the fixed 30-second
deadline. Interrupt cancels it. Drain and OS barrier are released on return;
process death releases connection/OS ownership. Lock acquisition has a 250ms
bound. Stream loss and IDLE never imply participant exit or installation success.

Pending: SubscribeEvents acknowledgement and initiating-client handoff; bundled
old-image independent launch integration; exact worker/managed-runtime identity
handoff and closed owner set; repeated native exit proof; acquisition and staged
validation; shutdown; publication/readback; manual normal-start outcome. Nothing
in this checkpoint authorizes publication, even if all supplied processes exit.

## Focused evidence

Native macOS arm64 tests use selected copies of the test executable in an inert
temporary enrolled installation. One disposable copy runs a fake gRPC agent;
another remains a disconnected client. Corrupt SQL does not prevent coordination.
Tests cover strict JSON/permissions, changed/unknown participant refusal before
RPC, wrong role/image and listener refusal, all evidence before drain, protected
sealed status, launch exclusion while held, cancellation/deadline owner release,
and still-live participants never being treated as installed or stopped. The
only process kills are test cleanup of these disposable test-owned copies.
Existing installcontext and agentclient suites also run; no generated protocol
or agentclient transport changes are part of this checkpoint.
