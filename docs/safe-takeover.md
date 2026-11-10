# Safe takeover: replacement scope

Lifecycle ownership takeover is no longer a separate RPC family. Normal reopen
uses cooperative OLD controls first, then explicitly confirmed contained-source
recovery when needed. See [current recovery protocol](exact-transfer-recovery.md)
for the removed APIs, retained wire envelope and bounded verification.

Safety requirements remain:

- Bind consent to the exact source process, durable owner/epoch, conversation and
  selected destination. Decline, Enter and Escape do not authorize interruption.
- Disclose whole-host interruption, including sibling conversations and tasks.
- Verify source/writer containment and launch exclusion before recovery; uncertain
  attribution, cancellation or changed ownership must not authorize reassignment.
- Preserve committed history, uncertain persistence handling and durable journals.
  Never treat historical receipts or journal counts as present ownership authority.
- Keep late/stale claims fenced; recovery must return validated current ownership.

The ordinary legacy stop-confirmation path remains supported and protected.
The agentclient terminal-attachment `TakeoverTarget`/`TakeoverConversation` API is
also retained: it conditionally replaces an exact attached terminal after explicit
confirmation, not a durable lifecycle owner. Removing it would break ordinary
attachment conflict handling and is outside obsolete lifecycle-protocol deletion.

Historical terminal acceptance evidence remains in
[resume/takeover acceptance](../efforts/dev-host-generations/resume-takeover-acceptance.md).
Historical exact-transfer implementation evidence remains marked superseded in
[exact-transfer recovery](exact-transfer-recovery.md). Neither historical evidence
nor local fixture tests establish live deployment or recovery of a user's host.
