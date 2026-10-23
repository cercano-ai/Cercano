# Advanced Context Management

## What it is

Rolling background compaction digests older conversation turns into layered
summaries while retaining recent working context. The agent sends a compacted
view rather than replaying an ever-growing transcript on every request.
Automatic compaction runs asynchronously, so request assembly does not wait
for a summarizer; explicitly requested regeneration reports progress until done.
The stored conversation history remains separate from that compacted view.

Cercano's compaction algorithms include **map-reduce**: summarize bounded
segments of older raw history, then merge their structured summaries. This
avoids repeatedly feeding the whole transcript into one summarization request
and gives long histories a manageable representation. Map-reduce is an
implemented compaction technique, not the name of a user-selectable mode or a
claim that every background compaction operation follows that algorithm.

## Why it matters

Keep working through long investigations and large refactors without stopping
for periodic summarization. A smaller active context leaves more space for
current work and reduces repeated history tokens sent to the model. That can
reduce token costs, although compaction itself consumes inference and savings
depend on your models and workload. Keeping repetitive history out of the active
window leaves more room for relevant evidence and current decisions, supporting
longer, more focused sessions—not a guarantee of lossless recall or unchanged
answer quality at arbitrary conversation lengths.

## Try it

- `/c` — open the active-conversation context viewer (also in `/config` → **Context**).
- `/context` — read the project’s `.cercano/context.md`, not the active context meter.
- `/compact` — start incremental background compaction.
- `/context-regen` — rebuild compacted context from stored turns.
- `/clear-compacted-context` — remove summaries and rehydrate from stored turns.
- `/elide-context` — reclaim context by stubbing prior tool outputs without inference.

Use the live context meter to follow a long session's working context.

## Controls and limitations

- Background work can fall behind. When compaction and its hard-limit override
  are enabled and a positive context window is available, request assembly schedules
  compaction and reduces the outgoing view with tool-output elision and, if
  necessary, removal of oldest messages—not a blocking summarizer call.
- Summaries are lossy. A manageable context is not a guarantee that every old
  detail is present in every request. Stored history and model-visible context
  are different things.
- Compaction thresholds and retention behavior are configurable. Clearing a
  summary does not make an arbitrarily large raw history fit the model window.
  Regeneration can only use remaining stored turns; it cannot recover bodies
  already removed by [retention aging](automatic-session-retention.md).
- `/elide-context` affects the in-memory send view, not stored raw turns; its
  effect resets on agent restart.

See the [context-management design](../context-management/design.md),
[non-blocking compaction design](../context-management/compaction/compaction-nonblocking-design.md),
and [agent command reference](../README.md).

Back to the [feature index](README.md).
