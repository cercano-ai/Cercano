# Bounded README audit: `/history` and `/clear`

## Scope and evidence

Only the slash-table rows in [agent/README.md](agent/README.md), lines 171 and 174, were audited. Read the root `AGENTS.md`; no nested `AGENTS.md` files were found under `docs` or `source/clients/cli`. The existing branch is `docs/core-closure-bounded`.

Sources inspected:

- [history.go](../source/clients/cli/internal/slash/history.go), lines 18–25: `/history` returns `ResultOpenHistoryPicker`; its help describes resuming an earlier session.
- [basic.go](../source/clients/cli/internal/slash/basic.go), lines 16–22: `/clear` returns `ResultClearConversation`.
- [model.go](../source/clients/cli/internal/ui/model.go), lines 3246–3260 and 3265–3268: the single downstream source file inspected. Clear removes displayed entries, assigns `newConvID()`, updates the conversation reference, and resets local session state. History opens a history view.

## Findings

- **`/history`: no concrete stale statement.** “Pick a past conversation to resume” matches the registered command's picker result and help, and the UI dispatch opens the history view. No README edit is needed.
- **`/clear`: no concrete stale statement at the inspected boundary.** Clearing displayed messages and starting a new conversation ID are explicit in the UI handler. The branch contains no deletion/reset request for the previous stored conversation; it switches the local conversation identity instead. This supports retaining the existing row, including its stored-history distinction, without claiming a persistence-wide guarantee.
- The `/clear` handler's help text says “server-side reset deferred,” but that is not the README row and was not changed. The actual UI branch, rather than that abbreviated help text alone, supplies the new-ID evidence.

## Exclusions and limits

No persistence-wide tracing was performed. Durable retention, deletion policies, concurrent/in-flight effects, ID-generation internals, picker implementation, and end-to-end resume behavior remain unverified. No additional downstream source file was read. The nearby `/rename` and `/resume` rows were not re-audited; the prior rename/resume report was not read. No broader guide audit, shared ledger edits, application edits, tests, builds, network calls, restarts, or delegation were performed.

## Validation

The README is intentionally unchanged: only this report is to be committed. Scoped whitespace and local-link existence checks are run separately before checkpointing; their actual results are included in the completion response. The first inspection command could not run `rg` (not installed); subsequent inspections used `grep` successfully. No application tests were run.
