# Independent delegated tasks

The native agent's public `dispatch` tool and `workflow` alias launch host-owned work. A successful response is an **acceptance receipt**, not the child's answer:

```json
{"task_id":"…","child_id":"…","status":"accepted"}
```

The status can already have advanced when the receipt arrives. Use the task handle to observe the work:

- `task_list` lists tasks belonging to the current chat.
- `task_inspect` retrieves retained state and results.
- `task_wait` explicitly waits for an outcome. Canceling the wait does not stop execution.
- `task_stop` explicitly stops the selected task, without stopping siblings.

Consumers that previously treated dispatch's returned text as the final answer must inspect or wait for the task instead. Synchronous internal consumers, including the dedicated review and research implementations, retain their existing result contracts; the public dispatch/workflow launch contract is the one that changed.

## Launch selection

`class` selects saved task routing; omission uses Default dispatch. Omit `tier`
to preserve that class's quality. An explicit `light`, `standard`, or `deep`
overrides quality with Economy, Standard, or Premium respectively, not the
saved destination. `cwd` (or its `path` alias) selects the working directory;
otherwise the caller's directory is used. Tool grants use plain registered
names, default to read-only when omitted, and write-capable grants require
approval. These are launch inputs, not extra authority conferred by acceptance.

## Lifetime and permissions

Steering the main conversation, finishing its originating turn, and disconnecting the CLI do not stop delegated work. Closing a child tab dismisses its presentation; it is not a stop request. Restarting the agent does stop unfinished execution: persisted tasks become `interrupted`, with available evidence retained. Tools are never automatically replayed after restart. Partial output is not proof that an interrupted tool had no side effects.

Existing routing, budgets, approved tool grants, and policy checks still apply. Task completion does not authorize new tool use. Task controls require host-established chat identity; a caller-provided `conversation_id` does not establish ownership. Unbound external MCP callers are rejected rather than granted access to another chat's tasks.

## Progress and completion

Before reporting a child's progress, use `task_inspect`. Its optional `evidence`
field summarizes the latest 32 recorded events: last recorded activity, recent
model/tool requests and outcomes, and up to 1,024 characters of recent
child-authored text. Child text is an unverified claim, not proof of successful
execution. A request without a recorded result does not prove it is still
executing, and silence does not prove a stall. `window_limited` indicates omitted
older events, oversized payloads, or malformed payloads. No evidence is returned
when no events are available; lifecycle and saved-result fields remain intact.
The evidence summary excludes raw prompts, reasoning blocks, tool arguments,
and tool-result bodies. Inspection neither acknowledges completion nor changes
task state.

The CLI uses a dedicated, chat-scoped `SubscribeTasks` stream, independent of the main turn. It receives authoritative lifecycle snapshots and durable progress records. Reconnect cursors avoid duplicate records, and stale events from a previously selected chat cannot update the new chat. Progress is currently published at persisted model-response and tool-event boundaries, not token-by-token.

A child finishing while the main agent is busy leaves a durable pending notification. The host supplies it at a safe turn boundary rather than canceling active work. When idle, the host can run a completion-handling turn without a connected CLI. Pending tool approvals remain gated and available for reconnection. Interrupted completion handling leaves the result inspectable rather than repeatedly triggering automatic retries.

## Implementation notes and remaining acceptance work

Task status, child progress, restart reconciliation, and host completion handling have regression coverage. Host-generated main-chat replies now arrive through the normal conversation attachment, separately from child progress. Event cursors prevent duplicate replay, and saved-turn identities prevent a completed reply from duplicating restored history. Worker-main completion, canceled observers, concurrent stop/completion, ownership isolation, and consumer guidance have also been checked with deterministic integration and race-enabled tests.

The progress transport reads durable storage in bounded pages, with a 200 ms tail interval. It uses transport backpressure rather than an unbounded event queue. Lifecycle scanning is proportional to the chat's retained task history.

## Quiet main-chat presentation

Normal main chat hides routine `task_list`, `task_inspect`, and `task_wait`
activity — their successful/in-progress rows and "running <task tool>" progress
notes — in every presentation mode, development mode included. Genuine tool failures and explicit `task_stop` activity remain visible.
Synthetic observer-cancellation rows are hidden in main chat even in development
mode; their retained data remains available to diagnostic/non-main views. Retained poll evidence stays
in the entry data, child tabs, and inspection paths; hiding is presentation
only. Child tabs retain their tool evidence. Persisted host completion and
autonomous-continuation envelopes are retained as metadata, not rendered as
ordinary main-chat prose, including in development mode. Actual assistant
follow-ups, permission prompts, and actionable autonomy pause notices remain
visible.

Live progress carries an explicit category through the runner, worker transport,
conversation replay, and client decoder. Tool activity uses the existing tool
row rather than adding a second status line. Host continuation/completion
bookkeeping is retained as diagnostic activity but hidden in main chat, including
development mode. Neither rule depends on the message's wording, punctuation,
capitalization, or arrival order. Ordinary and unknown progress categories keep
the existing visible-notice behavior; assistant text is not classified by these
rules. Legacy untyped tool notices retain a compatibility deduplication path.
Both the server and CLI must be rebuilt/restarted to use the structured contract.

Inspection is still read-only: merely listing or retrieving a result does not
acknowledge its completion notification. There is not yet a durable signal that
proves a retrieved result was reported to the user, so automatic follow-ups are
not suppressed based on polling alone.
