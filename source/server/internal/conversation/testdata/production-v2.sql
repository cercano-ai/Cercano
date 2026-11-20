-- Frozen production v2 layout (before execution bindings). Do not regenerate from current initializers.
CREATE TABLE autonomy_runs (
    run_id            TEXT PRIMARY KEY,
    conversation_id   TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    state             TEXT NOT NULL DEFAULT 'proposed',
    source_kind       TEXT NOT NULL DEFAULT '',
    source_plan_path  TEXT NOT NULL DEFAULT '',
    source_spec_path  TEXT NOT NULL DEFAULT '',
    brief_json        TEXT NOT NULL DEFAULT '',
    revisions_json    TEXT NOT NULL DEFAULT '',
    decisions_json    TEXT NOT NULL DEFAULT '',
    review_json       TEXT NOT NULL DEFAULT '',
    -- blocker_json records an explicit report_autonomous_blocker stop: the run
    -- is still "running" but waits for the user's next explicit message.
    blocker_json      TEXT NOT NULL DEFAULT '',
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL
);
CREATE TABLE coexistence_metadata (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1),
 contract TEXT NOT NULL, schema_version INTEGER NOT NULL
 );
CREATE TABLE conversation_chat_routes (
    conversation_id TEXT PRIMARY KEY REFERENCES conversations(id) ON DELETE CASCADE,
    profile TEXT NOT NULL,
    model TEXT NOT NULL
);
CREATE TABLE conversation_compaction (
    conversation_id   TEXT PRIMARY KEY REFERENCES conversations(id) ON DELETE CASCADE,
    frozen_through    INTEGER NOT NULL DEFAULT 0,
    segment_summaries TEXT    NOT NULL DEFAULT '',
    consolidated      TEXT    NOT NULL DEFAULT '',
    compacted_tokens  INTEGER NOT NULL DEFAULT 0,
    updated_at        INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE conversation_context_usage (
    provider TEXT NOT NULL DEFAULT '',
    runtime_instance_id TEXT NOT NULL DEFAULT '',
    conversation_id     TEXT PRIMARY KEY REFERENCES conversations(id) ON DELETE CASCADE,
    tokens_used         INTEGER NOT NULL DEFAULT 0,
    raw_tokens          INTEGER NOT NULL DEFAULT 0,
    message_tokens      INTEGER NOT NULL DEFAULT 0,
    system_tokens       INTEGER NOT NULL DEFAULT 0,
    tool_schema_tokens  INTEGER NOT NULL DEFAULT 0,
    output_reserve      INTEGER NOT NULL DEFAULT 0,
    estimated_request   INTEGER NOT NULL DEFAULT 0,
    context_window      INTEGER NOT NULL DEFAULT 0,
    window_known        INTEGER NOT NULL DEFAULT 0,
    model               TEXT    NOT NULL DEFAULT '',
    source              TEXT    NOT NULL DEFAULT '',
    computed_at         INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE conversation_handovers (handover TEXT PRIMARY KEY, conversation_id TEXT NOT NULL, destination TEXT NOT NULL, epoch TEXT NOT NULL);
CREATE TABLE conversation_ownership (
 conversation_id TEXT PRIMARY KEY, owner TEXT NOT NULL, epoch TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('owned','released','reserved')),
 destination TEXT NOT NULL DEFAULT '', handover TEXT NOT NULL DEFAULT ''
);
CREATE TABLE conversation_request_usage (
    provider TEXT NOT NULL DEFAULT '',
    runtime_instance_id TEXT NOT NULL DEFAULT '',
    conversation_id     TEXT PRIMARY KEY REFERENCES conversations(id) ON DELETE CASCADE,
    tokens_used         INTEGER NOT NULL DEFAULT 0,
    raw_tokens          INTEGER NOT NULL DEFAULT 0,
    message_tokens      INTEGER NOT NULL DEFAULT 0,
    system_tokens       INTEGER NOT NULL DEFAULT 0,
    tool_schema_tokens  INTEGER NOT NULL DEFAULT 0,
    output_reserve      INTEGER NOT NULL DEFAULT 0,
    estimated_request   INTEGER NOT NULL DEFAULT 0,
    context_window      INTEGER NOT NULL DEFAULT 0,
    window_known        INTEGER NOT NULL DEFAULT 0,
    model               TEXT    NOT NULL DEFAULT '',
    source              TEXT    NOT NULL DEFAULT '',
    computed_at         INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE conversations (
    id           TEXT PRIMARY KEY,
    title        TEXT NOT NULL DEFAULT '',
    project_dir  TEXT NOT NULL DEFAULT '',
    dev_work_dir TEXT NOT NULL DEFAULT '',
    model        TEXT NOT NULL DEFAULT '',
    title_source TEXT NOT NULL DEFAULT 'user',
    started_at   INTEGER NOT NULL,
    last_turn_at INTEGER NOT NULL,
    recap            TEXT NOT NULL DEFAULT '',
    recap_updated_at INTEGER NOT NULL DEFAULT 0,
    -- kind: 'main' for user-facing conversations, 'subagent' for persisted
    -- dispatch tool loops (hidden from List). parent_id links a subagent loop
    -- to the conversation whose dispatch spawned it.
    kind      TEXT NOT NULL DEFAULT 'main',
    parent_id TEXT NOT NULL DEFAULT '',
    -- precursor_id: temporal succession, distinct from parent_id's sub-agent
    -- containment. When a long session rolls over, a new 'main' conversation is
    -- created whose only inherited context is a handoff turn, and precursor_id
    -- points at the session it succeeded. The old conversation is left intact on
    -- disk; walk precursor_id backward (A<-B<-C) to reconstruct the lineage.
    precursor_id TEXT NOT NULL DEFAULT '',
    -- granted_tools: for subagent rows, the comma-joined tool names the
    -- dispatch loop was granted. Tool names are identifiers with no commas, so
    -- a plain join round-trips cleanly. Lets a resumed CLI reopen each
    -- sub-agent tab with the same tool set it showed live.
    granted_tools TEXT NOT NULL DEFAULT '',
    -- dismissed: 1 when a sub-agent tab was closed or swept in the CLI, so a
    -- resumed CLI skips reopening it (ListChildren filters dismissed = 0).
    dismissed INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE delegated_tasks (
 id TEXT PRIMARY KEY, parent_id TEXT NOT NULL, status TEXT NOT NULL,
 task TEXT NOT NULL, work_dir TEXT NOT NULL, child_id TEXT NOT NULL DEFAULT '',
 result_json TEXT NOT NULL DEFAULT 'null', error TEXT NOT NULL DEFAULT '',
 created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
 notification_pending INTEGER NOT NULL DEFAULT 0, wake_claimed INTEGER NOT NULL DEFAULT 0
 );
CREATE TABLE dispatch_events (
    conversation_id TEXT    NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    seq             INTEGER NOT NULL CHECK (seq > 0),
    kind            TEXT    NOT NULL,
    iteration       INTEGER NOT NULL DEFAULT 0,
    created_at      INTEGER NOT NULL,
    payload         TEXT    NOT NULL,
    PRIMARY KEY (conversation_id, seq)
);
CREATE TABLE turns (
    id              TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role            TEXT NOT NULL,           -- 'user' | 'assistant' | 'system'
    content         TEXT NOT NULL,
    tokens_in       INTEGER NOT NULL DEFAULT 0,
    tokens_out      INTEGER NOT NULL DEFAULT 0,
    latency_ms      INTEGER NOT NULL DEFAULT 0,
    created_at      INTEGER NOT NULL
, content_json TEXT NOT NULL DEFAULT '');
CREATE INDEX delegated_tasks_parent ON delegated_tasks(parent_id, created_at);
CREATE INDEX idx_autonomy_runs_conversation_updated
    ON autonomy_runs(conversation_id, updated_at DESC, created_at DESC, run_id DESC);
CREATE UNIQUE INDEX idx_autonomy_runs_one_active
    ON autonomy_runs(conversation_id)
    WHERE state IN ('running', 'review_pending');
CREATE INDEX idx_conv_last_turn ON conversations(last_turn_at DESC);
CREATE INDEX idx_conv_project ON conversations(project_dir, last_turn_at DESC);
CREATE INDEX idx_turns_conv ON turns(conversation_id, created_at);
INSERT INTO coexistence_metadata VALUES(1,'cercano-development-owned-v2',2);
PRAGMA user_version=2;

INSERT INTO conversations(id,project_dir,dev_work_dir,started_at,last_turn_at) VALUES('legacy','/project','/development',11,22);
INSERT INTO turns(id,conversation_id,role,content,content_json,tokens_in,tokens_out,latency_ms,created_at) VALUES('turn','legacy','assistant','history','[{"type":"text","text":"history"}]',12,34,56,22);
INSERT INTO delegated_tasks VALUES('task','legacy','running','keep working','/task-root','child','{"kept":true}','',11,22,1,1);
INSERT INTO conversation_ownership VALUES('legacy','old-owner','epoch-7','owned','','');
INSERT INTO conversation_handovers VALUES('handover','legacy','next-owner','epoch-8');
