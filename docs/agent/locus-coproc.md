# Locus Mode — Co-processor Tier (scope)

> **Historical record / design context.** Original dates, status labels,
> proposed APIs and unchecked steps below describe this document’s implementation
> stage, not a current feature matrix or authorization to execute a plan.
> Consult the [current feature guides](features/README.md) and [routing guide](../cloud-routing.md)
> for supported behavior. Dated external tests are not fresh provider verification.

> **Superseded routing:** Current routing uses task destination and quality,
> with defaults in `source/server/pkg/config/routing.go`. Default dispatch is
> Secondary/Standard, Compaction Secondary/Light, and Watchdog Local/Standard.
> The original local-first / co-processor policy below is not the current task
> routing contract. See [cloud routing](../cloud-routing.md).


## Current compatibility boundary (2026-10-22)

The four-mode tables, proposed APIs, CLI/UI affordances, and SmartRouter claims
below are historical, not instructions for current routing. Saved task
assignments and redirects govern current placement; tool names do not promise
local or free inference. Legacy `locus_mode` remains in configuration with
`cloud_primary` as its default; old `local_primary` / `local_only` values normalize
to `open_primary` / `open_only`. It is not the current task-routing authority.
See `source/server/pkg/config/config.go` and `routing.go` and the
[current routing guide](../cloud-routing.md). No historical “nothing leaves the
machine” statement here certifies network isolation.

## Historical proposal

**Status at proposal time:** Scoped 2026-06-27. Not yet planned/implemented. Follow-on to
[locus-mode.md](./locus-mode.md), which shipped the **main-LLM** tier and
deferred this.

Make every one-shot model call Cercano makes on behalf of a tool honor Locus
Mode, instead of hardcoding "always local."

## Problem

All co-processor tools call the agent with a hardcoded flag:

```go
s.grpcClient.ProcessRequest(ctx, &proto.ProcessRequestRequest{ Input: prompt, DirectLocal: true })
```

`direct_local: true` = "skip routing, use the local provider." So co-proc work
is **always local**, ignoring Locus Mode. Call sites (`internal/mcp/server.go`):
`summarize` (709), `extract` (755), `classify` (802), `explain` (844),
`research` / `deep_research` internals (985, 1007, 1131, 1243), and the
`grpcModelCaller` helpers.

## Co-proc resolution table

Differs from the main tier in exactly one mode (Cloud Primary):

| Mode | Co-proc tier | On unavailable |
|---|---|---|
| Cloud Only | cloud | hard-fail |
| Cloud Primary | local | fall back → cloud |
| Local Primary | local | fall back → cloud |
| Local Only | local | hard-fail |

`locus.Coproc()` == `locus.Main()` for every mode **except Cloud Primary**,
which flips to local-preferred (the whole point of that mode: cloud brain,
local grunt work).

## Decisions (settled)

1. **Tool scope: everything that calls the model.** All one-shot co-proc calls
   honor locus — summarize, extract, classify, explain, research, deep_research,
   document, and the internal `grpcModelCaller` paths. Each swaps
   `DirectLocal:true` → the new co-proc signal.
2. **Visibility: structured metadata, client decides.** Each tool returns
   *where it ran* (tier), *which model*, and *whether it fell back* as
   structured output; the client (Cercano's CLI, or a host like Claude Code)
   decides how to surface it. No mandatory text pollution.
3. **Decision site: agent-side.** `agent.ProcessRequest` resolves the co-proc
   tier via `locus.Coproc()` and picks local/cloud from its existing provider
   map. This also retires the SmartRouter from the co-proc path (progress on the
   other deferred item).

## Design

### Resolution
- Add `locus.Coproc() Resolution` (sibling to `Main()`), per the table.
- The agent gains read access to the current locus mode (small wiring, like the
  server's `resolveMainProvider`) and, on a co-proc request, resolves the tier,
  picks the local or cloud provider, applies bidirectional fallback for
  `*_primary`, and hard-fails for `*_only` (returns an error the MCP tool
  surfaces as a tool error).
- Cloud is served by the existing `CloudModel` provider's one-shot `Process()` —
  no tool-loop needed for co-proc.

### Request signal
- New `bool coproc = 8` on `ProcessRequestRequest`. MCP handlers set
  `Coproc: true` instead of `DirectLocal: true`. (`direct_local` stays for any
  caller that genuinely wants to force local.)

### Visibility plumbing (mostly exists)
- `ProcessRequestResponse` already carries `RoutingMetadata{model_name,
  is_fallback, escalated, ...}` and a `notice` string. The agent already sets
  `RoutingMetadata.ModelName`.
- Add a clear served-tier signal (e.g. `RoutingMetadata.is_cloud`, since
  existing `escalated`/`is_fallback` have different semantics — endpoint
  failover, not locus-tier choice).
- The co-proc handlers — which today discard `resp.RoutingMetadata` and
  `resp.Notice` — pass them into each tool's **structured output** (the `any`
  second return value, currently `nil`) via one shared struct, e.g.
  `{ExecutedModel string, ExecutedTier string, FellBack bool}`. Populate
  `notice` on a fallback so a human-readable line is available too.

## Work breakdown (rough)

1. `locus.Coproc()` + table test.
2. Proto: `ProcessRequestRequest.coproc`; `RoutingMetadata.is_cloud`; regen.
3. Agent: locus-mode access + co-proc resolution in `ProcessRequest`
   (local/cloud, fallback, hard-fail); set `RoutingMetadata` (model + tier +
   fallback) and `notice`; drop SmartRouter from this path.
4. MCP handlers: swap `DirectLocal` → `Coproc` at all one-shot sites; thread
   `RoutingMetadata`/`notice` into each tool's structured output via a shared
   metadata struct.
5. Tests: `Coproc()` table; per-tool routing per mode (mock provider); fallback
   sets notice + metadata; `*_only` unavailable → clear tool error.

## Open items / risks

_All items resolved. See Decisions (finalized) below._

## Decisions (finalized — Task 6)

- **`use_model` precedence:** An explicit `use_model` sets `ModelOverride` (the
  model name) but the request still runs on the locus-resolved tier. Both
  `Coproc: true` and `ModelOverride` are set; the agent honors both. Model
  selection is narrowed by the override, but local-vs-cloud routing still
  follows locus.

- **Cost under Cloud Only:** `deep_research` fans many internal model calls
  (via `grpcModelCaller` / `grpcModelCallerWithTokens`). Under Cloud Only, all
  of them route to cloud — intended. This is the user's explicit trade when they
  set Cloud Only. Worth surfacing in tool docs if cost concerns arise; no
  behavior change needed.

- **`cercano_local` follows default SmartRouter routing:** `handleLocal` (the
  `cercano_local` tool) sets neither `DirectLocal` nor `Coproc` — its
  `ProcessRequest` call carries no routing flag, so it follows the default
  SmartRouter path (which may route to cloud). It bypasses locus-managed co-proc
  routing by design, but is NOT guaranteed to run locally.
