# Agent execution isolation — architecture (as-built)

**Status:** Architecture snapshot with historical phase notes. Production configuration
defaults to `worker`; embedded/test construction can use `in_process`.
The phase checklist below is not a current implementation backlog.
`worker_idle_timeout_seconds` defaults to a five-minute idle window (zero selects
the default; negative disables idle reaping). See `pkg/config/config.go`.
This is the AS-BUILT complement to `design.md` (the forward-looking design) and
the per-phase plans (`plan-phase{2,3,4,5}.md`). Where the design describes intent,
this describes the code as it stands: which types exist, what they own, and how a
turn actually flows through them. Read `design.md` first for the *why*; read this
for the *what-is*.

> **Founding principle:** *an agent dying must not take down another agent.*
> Every structural decision below traces back to that sentence.

---

## 1. Purpose — the problem it solves

The agent server (`internal/server.Server`) is a singleton gRPC process. Before
this work, every conversation's turn executed in that one address space, sharing
mutable execution state. That produced a recurring class of bugs — each fixed
one at a time, all the same defect underneath:

- **cwd cross-talk.** The turn handler did a process-global `os.Chdir(WorkDir)`
  with `defer os.Chdir(prev)`. Concurrent turns from different conversations
  raced the process cwd; a relative-path `Read`/`Write`/`Bash` in one
  conversation could resolve into another's repo.
- **Session cross-delivery / turn overlap.** Shared upstream-session routing
  state and shared turn state let two turns interleave history writes or share
  one upstream session key.
- **No crash isolation.** One panic, OOM, or fatal runtime error took down
  *every* conversation, because they all lived in one process.
- **The `Server` god-object.** One struct with ~30 fields and three cross-cutting
  mutexes fused every responsibility (client streams, gRPC, config, providers,
  the DB, subprocess managers, permissions, the turn loop). Each bug above was
  "a field on the big `Server` mutated by two paths." It was also un-embeddable:
  it dragged a gRPC server and subprocess managers with it.

The goal: make the cross-talk bug class **unrepresentable** (owned per-conversation
execution state), add **crash isolation** (one conversation's death contained),
support **multi-surface attach** (CLI + VS Code on one conversation), and keep the
core **embeddable** (runs in-process when Cercano is hosted in another app).

**Core idea — one execution core, two deployment wrappers.** The correctness bugs
come from shared *mutable state*, not from the lack of a process boundary. So the
isolation lives in an execution **core** (`internal/runner.Core`) that holds zero
process-global mutable state; the process boundary is an *additional* wrapper on
top (`internal/worker`), not a second implementation. The same `Core` runs
in-process (embedded / test mode) or one-per-child-process (worker mode).

---

## 2. The big picture

```
                         ┌──────── surfaces ────────┐
                         │  CLI      VS Code   Zed   │  (gRPC clients)
                         └────┬─────────┬────────────┘
                              │ StreamProcessRequest / AttachConversation
                              │ (proto)
   ┌──────────────────────────▼───────────────────────────────────────────┐
   │  HOST PROCESS  (singleton :50052)                                      │
   │                                                                        │
   │  ┌───────────────────── Server (front door) ───────────────────────┐  │
   │  │  proto.UnimplementedAgentServer + handlers; ~13 fields.          │  │
   │  │  terminate client conns, route, proto ↔ domain. Thin.           │  │
   │  └───┬───────────────┬───────────────┬──────────────┬─────────────┘  │
   │      │ delegates to  │               │              │ turnRunner      │
   │      ▼               ▼               ▼              ▼  .RunTurn(...)   │
   │  ┌─────────┐   ┌──────────┐   ┌───────────┐   ┌──────────────────┐    │
   │  │ hostsvc │   │  broker  │   │  hostsvc  │   │   TurnRunner     │    │
   │  │ services│   │ (fan-out,│   │  services │   │  (interface)     │    │
   │  │ (6)     │   │ turn     │   │           │   │                  │    │
   │  └─────────┘   │ fence,   │   └───────────┘   │  in-process:     │    │
   │   SHARED /     │ replay)  │                   │   runner.Core    │    │
   │   host-owned:  └──────────┘                   │  OR              │    │
   │   config, DB,     ▲  fan-out to surfaces      │  worker mode:    │    │
   │   keychain,       │                           │   workerRunner ──┼────┼──┐
   │   permissions,    └── events published ───────┘                  │    │  │
   │   broker                                       └──────────────────┘    │  │
   └────────────────────────────────────────────────────────────────────────┘  │
                                                                                │ spawn +
                                          ISOLATED per-conversation execution   │ bidi gRPC
                                                                                ▼ (unix socket)
                                          ┌──────────────────────────────────────────┐
                                          │  WORKER PROCESS  ("cercano worker")        │
                                          │  proto.Worker server. Stateless compute.   │
                                          │  Builds execution Deps from ConfigSnapshot │
                                          │  + host-resolved credential; runs the SAME │
                                          │  runner.Core; proxies state ops to host:   │
                                          │   events · permission · persist · cred     │
                                          └──────────────────────────────────────────┘
```

**SHARED vs ISOLATED — the load-bearing split:**

| Host-owned (SHARED, singleton) | Per-conversation (ISOLATED) |
|---|---|
| config + secrets | turn execution (tool loop, tool exec, LLM stream) |
| conversation DB (one writer) | cwd (carried in `Request.WorkDir`, never `os.Chdir`) |
| keychain / OAuth refresh | provider/session identity |
| broker: fan-out, turn fence, replay | the `runner.Core` (or the worker process running it) |
| permission decisions (policy + prompt) | |

The worker holds **no durable state**. Anything stateful/UI it needs, it proxies
back to the host over the stream.

---

## 3. The layers / components

The field lists and approximate line numbers below are phase-era sketches, not
exhaustive current inventories. For example, `runner.Request` now also carries
chat-route/model overrides, authentication recovery and input attribution; its
`Deps` include vision and capability-profile accessors. Read the named types for
the complete API.

### 3.1 The front door — `internal/server/server.go`

`Server` embeds `proto.UnimplementedAgentServer` and terminates client
connections. Phase 2 shrank it from a ~30-field god-object to ~13 fields; it now
*delegates* to focused services instead of *being* them. Its job is thin:
terminate gRPC, route RPCs, map proto ↔ domain. Its fields:

```
agent        *agent.Agent
providerSvc  providers.Resolver      // the 6 hostsvc services ↓
cfgSvc       cfgsvc.Service
toolSvc      toolssvc.Catalog
persistSvc   persistsvc.Service
permBroker   permissions.Broker
runtimesSvc  runtimessvc.Supervisors
watchdog     *watchdog.Watchdog
turnRunner   runner.TurnRunner       // in-process Core OR workerRunner
events       *eventHub               // legacy server→client push
turnBroker   *broker.Broker          // Phase 4 turn fence + fan-out
```

The turn handler `streamProcessRequestWithToolLoop` (server.go ~1978) is the
single per-turn choreographer: it opens the turn fence (`beginTurn`), attaches
the initiator as the first (lossless) broker subscriber, spins the turn in a
goroutine that calls `s.turnRunner.RunTurn(ctx, req, sink, requester, persist)`
(server.go ~2081), and drains broker events to the initiator's stream. Both modes
share this handler and `TurnRunner` interface (§3.4, §3.6); worker-specific
conversation setup and post-turn bookkeeping still run on the host.

### 3.2 The six host services — `internal/hostsvc/`

Phase 2 decomposed the god-object into six services behind interfaces, wired by
the front door as a thin composition root. One line each:

| Service | Package | Owns |
|---|---|---|
| **Config** | `hostsvc/config` | the live `config.Config` + secrets; parse/validate/persist; change notification. Sole owner of `configPath`/`cfgMu`. |
| **Providers** | `hostsvc/providers` | given config + locus, hands out the right provider (cloud/open, router, coordinator, engine registry, catalog manager). Implements `providers.Resolver`. |
| **Tools** | `hostsvc/tools` | the tool universe + dispatch — tool registry, capability registry, dispatch engine. Implements `tools.Catalog`. |
| **Persistence** | `hostsvc/persistence` | durable conversation state — the shared transcript; retention sweeper, compaction gen, context loader. One writer. Implements `persistence.Service`. |
| **Permissions** | `hostsvc/permissions` | policy + the confirm round-trip — permission store, pending decisions, mode. Implements `permissions.Broker`. |
| **Runtimes** | `hostsvc/runtimes` | spawned-subprocess lifecycle — meridian manager, local runtime manager, MCP manager. **Workers join this family.** Implements `runtimes.Supervisors`. |

Why full decomposition is load-bearing (not aesthetic): (1) it is the precondition
for a cuttable worker boundary — a worker needs exactly config + providers + tools
+ persist/permission callbacks, and you cannot hand that clean subset out of a
tangled struct; (2) it makes Cercano embeddable — a host app instantiates the
service set it wants; (3) the bugs we fought were god-object symptoms; (4) lock
soup (three cross-cutting mutexes) becomes per-component ownership.

### 3.3 The broker — `internal/broker/broker.go`

The `Broker` owns three per-conversation concerns, all under one `mu`. It is
**proto-free** and does not import `internal/server`.

- **Turn exclusivity + generation fence.** `BeginTurn(parent, conv)` supersedes
  any turn already live on that conversation (cancels its ctx), increments the
  per-conversation generation counter, resets the replay buffer, and returns
  `(ctx, gen, release)`. `IsCurrent(conv, gen)` is the fence: a superseded turn
  fails it and goes quiet. `Publish`/persist paths check the gen, so a stale
  turn's late events and writes are silently dropped — never interleave into
  live history.
- **Fan-out.** `Publish(conv, gen, ev)` fans a `runner.Event` to every attached
  subscriber, under `mu`, fenced by gen.
- **Replay buffer.** `Publish` also appends to the current turn's buffer.
  `Attach` atomically snapshots that buffer (replay) *and* registers a live
  channel under the same lock, so every event is **either** in replay (published
  before Attach) **or** eligible for live delivery (published after), without an attach-boundary race. Passive live delivery can still drop on overflow.
- **Lossy vs lossless subscribers.** `Attach` returns a cap-64 **drop-on-full**
  channel — correct for a passive observer that must never stall the turn.
  `AttachLossless` returns a channel backed by an unbounded queue drained by a
  background goroutine — Publish never drops, never blocks. The turn's initiator
  uses `AttachLossless` (its stream is authoritative output); passive attachers
  use `Attach`.

### 3.4 The runner — `internal/runner/{runner.go,core.go,event.go}`

The execution **core**, where correctness lives. It is **transport-agnostic,
proto-free, provider-free**, and holds **zero process-global mutable state** —
which is what makes N cores safe in one process (embedded mode) and one core per
worker process (worker mode) *the same code path*.

- **`TurnRunner` interface** (runner.go): a single method
  `RunTurn(ctx, req, sink, requester, persist) (Result, error)`. Both the
  in-process `Core` and the host-side `workerRunner` satisfy it.
- **`Request`** carries only user-facing turn inputs: `ConversationID`, `Input`,
  `Images`, `WorkDir`, `Gen`. It deliberately holds **no** `llm.Provider` and
  **no** assembled history — the runner resolves the provider and assembles
  history itself, so it works across a process boundary.
- **`Deps`** are the shared services injected once at construction:
  `Providers` (resolver), `Tools`, `Persist` (a narrow `TurnHistory`), `Config`,
  `Perms`, `Agent`, and a **live** `Watchdog` accessor. `TurnHistory` and
  `ToolSvc` are *narrow* interfaces defined in the runner package precisely so
  the runner stays proto-free — the full `persistence.Service` / `tools.Catalog`
  carry proto-typed methods that would drag `pkg/proto` in.
- **`Event` vocabulary** (event.go): a proto-free `Event` struct + `EventKind`
  enum (route-selected, token, progress, tool-use-start/stop, tool-exec-start/
  complete, watchdog, done). The host maps these to `proto.StreamProcessResponse`
  payloads; embedded mode consumes them directly; a worker serializes them over
  its stream. `EventSink.Emit` is fire-and-forget; `PermissionRequester` is a
  request/response func (separate because it round-trips); `PersistFunc` is
  fire-and-forget and host-fenced.
- **`Core.RunTurn`** (core.go) does the whole turn: resolve provider from locus,
  assemble history, persist the user turn up front (crash-resilience), build the
  watchdog gate + gated registry, run `agent.RunToolLoop`, do cross-tier fallback
  only before visible text/tool execution makes replay unsafe. Same-provider
  whole-turn retry is similarly gated and additionally requires a retryable error.
  Recap, compaction and context-usage bookkeeping require a host `Agent`; the
  worker has none, so `Server.workerPostTurn` performs those on the host and
  emits aggregate usage telemetry (not in-process per-call usage). Note
  `buildSystemPrompt` and the directory/git snapshot live here (proto-free copies)
  and use `req.WorkDir` — never `os.Chdir`.

### 3.5 The worker — `internal/worker/`

The worker is a child process that runs the execution of **one** conversation's
turn in isolation and holds **no durable state**. Worker-side files:

- **`worker.go`** — `WorkerServer` implements the `proto.Worker` bidi service.
  `RunTurn` receives `StartTurn`, builds execution `Deps` (local providers + tools
  from the `ConfigSnapshot`, stream-backed permissions, preloaded history), then
  runs the **unchanged** `runner.New(deps).RunTurn(...)` with stream-backed
  sink/requester/persist. It builds providers via `buildWorkerProviders`
  (a minimal `workerResolver`) and tools via `buildWorkerToolSvcWithDiagnostic` (the full
  built-in registry plus proxies for advertised host MCP tools; MCP servers themselves remain host-side).
  Panics in the turn are recovered into a `TurnError`.
- **`proxies.go`** — the stream-backed `Deps`/callback proxies: a single
  serialized `sender` goroutine (gRPC `Send` is not concurrency-safe);
  `streamEventSink` (Emit → `WorkerEvent`), `streamPermissionRequester`
  (id-correlated round-trip), `streamCredentialSource` + `streamTokenSource`
  (id-correlated credential fetch; ChatGPT and Anthropic subscription token
  adapters), `streamSessionProfileController` (host profile read/switch), and
  `streamPersistFunc` + `preloadedHistory` (Assemble/LoadProjectContext return
  the StartTurn-provided values; PersistTurn proxies to the host).
- **`wire.go`** — the codecs: `MarshalMessage`/`UnmarshalMessage`
  (`llm.Message ↔ LLMMessage` via JSON-serializable blocks; transient provider
  extras are excluded), `MarshalEvent`/`UnmarshalEvent` (event subset; see §5),
  `SnapshotConfig`/`ConfigFromSnapshot`.
- **`host.go`** — the host-side `workerRunner` (see §3.6 / §4-worker).
- **`spawn.go`** — host-side process lifecycle: `spawnWorker` finds the `cercano`
  binary (sibling-then-$PATH), starts `cercano worker --socket <path>` with
  `Setpgid` + a pidfile (mirroring `internal/meridian/manager.go`), polls the
  unix socket to ready, and gRPC-dials it. `workerHandle.Kill()` kills the
  process group, closes the conn, removes socket + pidfile — idempotent.
- **`cmd/cercano/main.go`** — the `worker` subcommand (`runWorkerMode`): parse
  `--socket`, serve `proto.Worker` on the unix listener. Not user-invoked; the
  host spawns it.

### 3.6 The `workerRunner` (host side) — `internal/worker/host.go`

`workerRunner` implements `runner.TurnRunner`, so both modes use the same
host front door, with worker-specific setup/bookkeeping around execution. Its `RunTurn`:
pre-assembles history + project context (it owns the store), builds the
`ConfigSnapshot` + permission mode, acquires a pooled worker (or uses an injected dial
in tests), sends `StartTurn`, then runs a **drain loop** mapping worker→host
messages onto the host callbacks it was handed. It is the mirror of the worker's
proxies. §5 details the transport.

---

## 4. A turn's lifecycle — both modes

Both modes use `streamProcessRequestWithToolLoop`. The `TurnRunner` interface
selects execution; worker-specific conversation setup and post-turn bookkeeping
remain host responsibilities.

### 4.1 IN-PROCESS mode (embedded / test / explicit opt-in)

1. **Front door.** Client calls `StreamProcessRequest`. The handler calls
   `beginTurn(stream.Context(), convID)` → `broker.BeginTurn`, which supersedes
   any prior turn on this conversation and returns `(ctx, gen, release)`.
2. **Attach initiator.** The handler `AttachLossless(convID)` — the initiator is
   just the first subscriber. It builds the `sink` (Publish to broker at `gen`),
   the `requester` (host permission round-trip), and the fenced `persist`.
3. **Run.** In a goroutine, `s.turnRunner.RunTurn(ctx, runReq, sink, requester,
   persist)` — here `turnRunner` is an in-process `runner.Core`. `Core.RunTurn`
   resolves the provider, assembles history, runs the tool loop, and emits
   `runner.Event`s via `sink.Emit` → `broker.Publish(convID, gen, ev)`.
4. **Fan-out.** The broker fans each event to every attached subscriber; the
   handler drains the initiator's lossless channel and `sendRunnerEvent`s each to
   the client stream.
5. **Done.** `RunTurn` returns a `Result`; the handler sends the terminal
   response; `release()` retires the turn.

### 4.2 WORKER mode (crash-isolated; Phase 5)

Steps 1, 2, 4, 5 share the front door, broker and callbacks, but the handler
also ensures the worker conversation row and runs host-owned post-turn
bookkeeping. The execution wrapper in step 3 changes: `turnRunner` is a `workerRunner`, and
`workerRunner.RunTurn` (host.go):

1. **Pre-assemble.** `history := persist.AssembleHistory(ctx, convID)`;
   `projectCtx := persist.LoadProjectContext(workDir)` — the host owns the store.
2. **Snapshot.** `snap := SnapshotConfig(cfg, "", w.resolveOpenTiers())`
   resolves effective Local tier IDs on the host. The host adds a session-pinned
   profile if absent from global routing references and attaches model evidence
   when its evidence callback is configured. No credential is supplied in this
   call — the worker fetches on demand; permission mode is read separately.
3. **Acquire.** The per-conversation pool reuses a healthy idle worker with matching managed-policy mode, or spawns `cercano worker --socket …` with Setpgid, pidfile, socket readiness and gRPC dial. (Test path: injected `dial`.)
4. **Start.** Open `client.RunTurn(ctx)` bidi stream; `Send(StartTurn{input,
   images, workDir, gen, config, history, projectContext, permissionMode})`.
5. **Drain loop.** For each `WorkerToHost` message:
   - `WorkerEvent` → `UnmarshalEvent` → `sink.Emit` (→ broker → surfaces).
   - `PermissionRequest` → call host `requester(...)` in a goroutine (a slow human
     must not block the event drain) → send `PermissionResponse`.
   - `CredentialRequest` → `resolveCredential` on the host (keychain / OAuth) →
     send `CredentialResponse`.
   - `PersistTurn` → `UnmarshalMessage` → host `persist(m)` for the main
     conversation (host callback fenced by the captured turn generation). A
     supplied sub-conversation ID instead uses host `TurnHistory.PersistTurn`;
     that branch is best-effort and does not use the main-turn fence.
   - Session-profile requests → host profile read/switch callbacks → correlated
     response. Open-inference requests → host runtime provider.
   - Dispatch-evidence appends → host store write → acknowledgment; unlike
     ordinary turn persistence, the worker waits for this acknowledgment.
   - `TurnDone` → capture `Result`, keep draining until stream EOF, return nil.
   - `TurnError` → return the error.
6. **Meanwhile, in the worker:** the `WorkerServer.RunTurn` handler runs the
   **same `runner.Core`** with stream-backed proxies. Its events, permission asks,
   persist calls, and credential asks all go back over the stream — exactly what
   the host drain loop consumes.
7. **Release.** Deferred cleanup releases the acquired handle to the pool. `TurnDone` followed by stream closure marks it healthy for warm reuse; cancellation observed before completion or stream failure before `TurnDone` leaves it unhealthy and evicts/kills it. The current receive loop treats any receive error after `TurnDone` as completion, not only EOF. A displaced handle cannot release or evict a newer turn’s worker.

The tool loop executes in the child, but host MCP tools, host-managed open
inference and state/control operations still execute on the host. This is a
shared execution core, not proof of identical event streams or of isolation from
failures inside host-executed callbacks.

---

## 5. The worker transport — `proto.Worker` (`source/proto/agent.proto`)

A **separate** service from `Agent` (the worker's failure surface must not touch
the client-facing service). The host is the gRPC **client**; the worker **serves**:

```
service Worker {
  rpc RunTurn (stream HostToWorker) returns (stream WorkerToHost) {}
}

// Core lifecycle subset, not the complete protocol:
HostToWorker { oneof: StartTurn | PermissionResponse | Cancel | CredentialResponse | ... }
WorkerToHost { oneof: WorkerEvent | PermissionRequest | PersistTurn
                    | TurnDone | TurnError | CredentialRequest | ... }
```

`StartTurn` carries `conversation_id`, `input`, `images`, `work_dir`, `gen`,
`ConfigSnapshot`, pre-assembled `history` (`[]LLMMessage`), `project_context`, and
`permission_mode` (the worker's gating mode **must match** the host — never
default, or a strict host would silently auto-run writes in the worker).

**`LLMMessage`** is the wire form of `llm.Message`: `role` + `blocks_json`
(`json.Marshal(m.Blocks)`) + `content`. `blocks_json` is the same encoding the
persistence layer uses. `content` is informational and ignored on decode. This
preserves JSON-serializable block fields, not transient `ProviderExtras`
(`json:"-"`); invalid block JSON fails decoding. **`ConfigSnapshot`** carries the
worker's execution configuration at turn time, including routing assignments
and referenced profiles, host-resolved Local tier IDs, model-profile choices,
compaction knobs and watchdog settings. Host-only Port, LlamaServer and retention
settings are not reconstructed. See `SnapshotConfig` / `ConfigFromSnapshot` for
the current inventory rather than relying on a fixed field count.

`WorkerEvent` is a flat **subset**, not a lossless mirror of `runner.Event`.
The codecs carry route/model, text/progress, tool, watchdog, sub-agent and done
result fields. Task-change and authentication kinds are not mapped by these
codecs; unsupported outgoing kinds become `UNSPECIFIED` and unsupported incoming
kinds become `runner.EventKind(-1)`. Task snapshots, authentication payloads and
runner persistence/epoch/generation/sequence/background metadata are not carried
by `MarshalEvent` / `UnmarshalEvent`. This boundary does not establish how those
features are supplied through other host/control paths.

**Core proxied operations** (a subset; worker asks, host owns the state):

1. **Events** — worker `WorkerEvent` → host `sink.Emit` → broker fan-out.
2. **Permission round-trip** — worker `PermissionRequest{id,…}` blocks on a
   per-id channel; host prompts its client and answers `PermissionResponse{id}`.
3. **Persist** — worker `PersistTurn{message, gen}` is one-way/best-effort, not
   a durable-write acknowledgment. Marshal failures are dropped. Main-turn
   writes use the host callback’s captured-generation fence; sub-agent turns
   carry their own conversation ID and bypass that main-turn fence. Sub-agent
   creation is also best-effort, while dispatch-evidence append is acknowledged.
4. **Credential-on-demand** — worker `CredentialRequest{id, profile_name}`; host
   `resolveCredential` reads the keychain (static-key route: `secrets.Get`) or
   resolves subscription tokens through the shared host credential owner
   (`w.cfg.Credentials().ChatGPT/Anthropic(...).Token`) and answers
   `CredentialResponse{id, token, account}`. Host resolution runs off the drain
   path; typed sanitized failures cross the stream without a token/account.

**The credential proxy — why it matters.** The worker holds **no durable
credential**. Earlier design put a resolved credential in the `ConfigSnapshot`
(the `resolved_credential` field is now deprecated/unread). The proxy supersedes
it: the worker asks the host over the stream. The host owns keychain access and
subscription refresh, but credentials **do enter child memory**: static keys are
fetched during provider construction and retained by the provider; subscription
adapters fetch through the host when their provider requests a token. This does
not guarantee successful refresh or live mid-turn rotation of static keys.
Credential, permission and profile requests use correlated IDs and wait for a
response or context cancellation; stale/unmatched responses are ignored. Their
shared bounded sender can block on enqueue, so cancellation of a response wait
is not a guarantee of nonblocking admission or successful delivery.

**Provider construction.** Each turn rebuilds providers from its snapshot.
`buildWorkerProviders` uses shared `profilechain.Build` independently for Primary
and Secondary destinations, with task-quality model resolution and ordered
backups. ChatGPT Responses and Anthropic subscription routes get stream token
adapters; other non-Bedrock routes fetch a static credential and run
`ValidateStaticCredential`. A nonempty BaseURL alone does not exempt a canonical
vendor endpoint from key requirements. Bedrock skips this stream key fetch.
Cloud providers are vision-guarded with host-supplied evidence. Open inference
uses the host stream proxy for `llama_server` and `mistralrs`, otherwise a direct
Ollama client only when an Ollama URL is configured.

**Persistence boundary.** The core assembles history before persisting the
current user input (avoiding duplication), and persists that input before model
execution when a persistence callback and conversation ID exist. Worker
`preloadedHistory` returns the host-supplied history/context, not a fresh database
read or target-aware history rebuild. Assistant/tool-result callbacks also
forward to the host. Enqueue/stream completion is not durable-write confirmation;
this is crash-resilience intent, not a guarantee that every emitted turn survives
a child/host crash.

---

## 6. Multi-surface attach — `AttachConversation` (Phase 4)

The broker fan-out is what makes one conversation watchable from several surfaces.
`AttachConversation` (server.go ~1930) is a server-streaming RPC:

1. `replay, ch, detach := turnBroker.Attach(convID)` — a **lossy** (drop-on-full)
   subscription, correct for a passive observer.
2. Send the `replay` buffer first (everything published before this attach) via
   `sendRunnerEvent`.
3. Forward live events off `ch` until the channel closes (subscription detached)
   or the client disconnects. Ordinary turn release and supersession do not close
   passive subscriptions: they remain attached across turns until detached.

Because `Attach` snapshots the replay buffer and registers the channel under one
lock, a second surface that attaches **mid-turn** sees the turn so far (replay)
then live events without an attach-boundary duplicate or gap. The bounded passive channel can still drop subsequent events when full. The runner never knows two
surfaces exist — attachment is purely a host concern. A surface can attach whether
or not a runner is currently live; the durable thing is the conversation (store +
broker), the runner is a transient engine behind it.

---

## 7. Crash isolation (Phase 5)

- **Detection.** In `workerRunner.RunTurn`'s drain loop, `stream.Recv()` returns
  EOF or an error. If `TurnDone` was already seen, that's the normal close →
  return the captured `Result`. If **not** (crash mid-turn: panic that killed the
  process, OOM, kill), the worker died before completing → the host returns
  `status.Errorf(codes.Unavailable, "worker turn failed: %v", …)`. Cancellation/deadline errors are preserved; unsupported required enterprise-policy or authentication-recovery protocols fail with `FailedPrecondition`.
- **Containment.** Deferred pool release kills/evicts an unhealthy worker's
  process group and cleans up. Healthy completion instead retains it warm. The error flows back through the *same* host turn
  handler as an in-process error would: the handler's `defer release()` retires
  the broker turn, the generation fence rejects events from superseded/canceled generations.
  Ordinary release clears the active handle but does not advance the generation
  or close passive subscriptions; it alone is not a late-event fence. Do not equate passive broker-channel closure with receiving the initiating RPC’s gRPC status.
- **Blast radius = one conversation.** The host process does not exit. Other
  conversations — their own workers, their own broker turns — are untouched. The
  next submit on the crashed conversation spawns a fresh worker. This is the
  phase's raison d'être and the founding principle made real.
- **Host death** (out of scope for the mid-turn path, noted for completeness):
  workers are the host's children (Setpgid group); clean shutdown kills the group,
  a hard-killed host leaves pidfiles for the next host to reap (Phase 6).

---

## 8. Key invariants

- **Proto-free `runner` + `broker`.** Neither imports `pkg/proto` or
  `internal/server`. `runner` defines *narrow* `TurnHistory`/`ToolSvc` interfaces
  precisely to avoid dragging proto in. This is what lets the same core run
  in-process and in a worker.
- **Zero process-global mutable state in the runner.** `Core` carries all
  execution state in locals/args; nothing global. Directly guarded by
  `internal/runner.TestCore_ConcurrentTurns_NoCrossTalk` — two cores, different
  workdirs/sessions, concurrent under `-race`, asserting zero cross-talk. That
  test *is* embedded mode; it guards the whole design in milliseconds.
- **The generation fence.** At most one live turn per conversation; a superseding
  turn advances the gen. Broker publication and the main-turn persistence callback
  check the captured conversation/generation, dropping stale work. Sub-agent
  persistence uses its own conversation ID and bypasses that main-turn fence (§5).
- **Host owns durable conversation/control state; worker owns transient execution state.** Config, DB (one writer),
  keychain/OAuth, broker, permission decisions all live host-side. The worker
  never opens the DB and never touches the keychain — it proxies.
- **`workerRunner` implements `TurnRunner` → a shared front door.**
  The interface is shared; worker setup, transport and host post-turn bookkeeping
  are additional paths with their own failure surface.
- **`WorkDir` + session threaded via ctx/`Request`, never `os.Chdir`.** The turn
  carries its cwd explicitly (`Request.WorkDir`, used by `buildSystemPrompt` and
  the tool loop) and its session id via `anthropic.WithSessionID(ctx, convID)`.
  The process-global `os.Chdir` is gone.
- **Behavioral parity is a goal, not a transport invariant.** Both modes use
  `runner.Core`, but §5 lists event-codec omissions, persistence guarantees differ
  from acknowledgments, and worker usage telemetry is aggregate. Source comparison
  and historical test claims do not certify identical client-visible streams.

---

## 9. Phase roadmap / status

**Historical implementation notes.**

The table records historical completion labels, not current acceptance results
or certification of every invariant. Current provider/watchdog wiring is described
below; the bounded source-audit evidence and live exclusions are recorded in
[the isolation closure report](../../audit-core-isolation-closure.md). In particular, orphan cleanup is
best-effort (malformed pidfiles are left alone; unknown legacy spawners rely on
the identity guard), not an unconditional no-orphans guarantee. Permission
updates have a worker receiver but no host-side trigger: a running worker uses
the permission values pinned at `StartTurn` until the next turn.

| Phase | Scope | Status |
|---|---|---|
| **1** | cwd isolation — thread `WorkDir`, delete `os.Chdir` | Done |
| **2** | host decomposition — the six `hostsvc` services + thin front door | Done |
| **3** | `TurnRunner` core — proto-free, provider-free, zero global state; concurrent-runner guard test | Done |
| **4** | broker + attach — per-conversation fan-out, replay, `AttachConversation` | Done |
| **5** | worker process — bidi transport, worker binary, host `workerRunner`, credential proxy, basic crash isolation | Done |
| **6** | equivalence (backup failover, watchdog, MCP fallback) + lifecycle (per-conversation reuse, health/restart, idle-reap, orphan-sweep, graceful drain) | Done |

**Phase 5 sub-status (as-built).** The wire protocol (`proto.Worker`, envelopes,
`LLMMessage`, `ConfigSnapshot`, credential messages) and codecs are implemented;
§5 describes their deliberately incomplete mappings. Historical round-trip test
claims are not complete-field or end-to-end acceptance. The worker binary (`worker.go`, `proxies.go`, the `worker`
subcommand) is complete, including the credential proxy (`streamCredentialSource`
/ `streamTokenSource`). The host-side `workerRunner` (`host.go`, `spawn.go`) is
implemented: spawn/dial/drain/crash-detect and host-side credential resolution.
The config toggle is landed: `config.ExecutionMode` defaults to `worker`, and
`Server.SelectExecutionMode()` (called from `runServerMode` after config load)
swaps in `worker.NewWorkerRunner`; the in-process `runner.New(runnerDeps())`
stays the default for embedded mode + the test suite (which construct `Server`
directly and never call the selector). Historical parity/crash tests are not rerun or certified by this source audit;
no spawned-process SIGKILL acceptance was performed.

**Phase 6 source wiring (Part A).** The following implementation paths exist;
they do not independently establish parity or operational safety of the default:
- **Backup-profile failover** — the config snapshot carries both active and backup
  profiles; `buildWorkerProviders` calls shared `profilechain.Build` separately
  for Primary and Secondary, rather than the former `wrapWorkerBackup` path.
  Credential validation and subscription token adapters are described in §5.
- **Watchdog** — `buildWorkerWatchdog` reconstructs protocol supervision from the
  snapshot (nil when disabled). Its OneShot lane uses the shared worker dispatch
  engine with `RoutingTask: TaskWatchdog`; the snapshotted task assignment and
  destination configuration select the route. It is not hardwired to a local
  fast model or the former `buildWorkerEngine`/`watchdogModelFor` path.
- **MCP tools** — MCP servers live host-side, so a conversation carrying MCP tools
  uses worker-side proxies advertised at turn start. `Server.pickTurnRunner`
  selects the worker whenever configured; `hasMCPTools()` does not change runner
  selection. MCP calls execute on the host, outside the child failure boundary.

**Phase 6 as-built — lifecycle (Part B).** Workers are pooled per conversation
(`internal/worker/pool.go`):
- **Warm reuse** — a conversation reuses its worker process across turns when it is
  idle and healthy; otherwise the pool spawns a fresh one (outside the lock).
- **Health-check / restart** — `workerHealthy` gates reuse on process-alive plus a
  live gRPC connection state; an unhealthy or crashed worker is evicted and killed,
  and the next `Acquire` spawns a replacement. A worker crash surfaces to the host
  as `codes.Unavailable`; the host recovers the turn and does not auto-retry.
- **Idle-reap** — a background reaper kills workers idle past a configurable window
  (`config.WorkerIdleTimeout()`: `>0`→seconds, `0`→5-minute default, `<0`→disabled).
- **Orphan-sweep on startup** — `worker.ReapOrphanWorkers()` (`reap.go`) sweeps
  workers left behind by a hard host crash, identity-guarded (process-group +
  `cercano worker` cmdline + spawner-dead) so it never kills an unrelated process,
  mirroring the Meridian supervisor.
- **Graceful drain** — on SIGTERM the host `Shutdown()` drains the pool, killing all
  cached workers (idempotent). Drain and next-startup sweep are cleanup mechanisms,
  not an unconditional no-orphans guarantee; the sweep limitations above apply.

---

## 10. Code pointers

- `internal/server/server.go` — the front door; `streamProcessRequestWithToolLoop`
  (turn handler, ~1978), `AttachConversation` (~1930), `runnerDeps` (~625),
  `beginTurn`/`turnIsCurrent` shims.
- `internal/hostsvc/{config,providers,tools,persistence,permissions,runtimes}/` —
  the six host services.
- `internal/runner/{runner.go,core.go,event.go}` — the execution core.
- `internal/broker/broker.go` — turn fence, fan-out, replay, lossy/lossless subs.
- `internal/worker/{worker.go,proxies.go,wire.go,host.go,spawn.go}` — the worker
  transport, stream proxies, codecs, host runner, and process spawn/kill.
- `internal/worker/{pool.go,reap.go,watchdog.go}` — the per-conversation pool
  (reuse/health/idle-reap/drain), startup orphan-sweep, and worker-side watchdog.
- `internal/server/server.go` — `pickTurnRunner`/`hasMCPTools` (per-turn selection),
  `SelectExecutionMode`, `Shutdown` (pool drain).
- `source/proto/agent.proto` — `service Worker` + envelopes (~1129).
- `cmd/cercano/main.go` — the `worker` subcommand (`runWorkerMode`, ~1614),
  `ReapOrphanWorkers` at startup, SIGTERM→`Shutdown`.
- `internal/meridian/manager.go` — the supervisor pattern spawn/reap reuses.
- `docs/agent/agent-isolation/design.md` + `plan-phase{2,3,4,5,6}.md` — the design
  and per-phase rationale.
