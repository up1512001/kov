# Kov Architecture

> **Status of this document.** This is a *target architecture* — the design kov is intended to converge on. It was drafted from the public README, the visible repository layout (`cmd/kov`, `internal/`, `agents/`, `.kov.yaml.example`), and the documented feature set. Wherever the current implementation diverges from what's described here, treat this document as the design intent and file an issue with the diff. As sections are verified against the actual code, change "designed to" to "does" so future readers can tell what's shipped from what's planned.

---

## 1. Goals and non-goals

### Goals

Kov exists to make AI coding sessions **survive failures that today's CLIs treat as fatal**. The specific failures it must handle, in priority order:

1. Provider rate limits (HTTP 429), with respect for `Retry-After`.
2. Auth expiry (401/403) with a clean pause-and-resume path.
3. Network drops mid-stream — including the SSE-hang case where the connection is technically open but no bytes are arriving.
4. Process death (`kill -9`, OOM, reboot) between any two operations.
5. User interrupt (Ctrl+C) without losing in-flight tool work.
6. Cost overruns — the agent should stop before spending past a configured budget.
7. Doom loops — the agent repeating the same action with the same result.
8. Context window overflow — compact predictively, before the model errors.

The tool is intended to be a **standalone CLI agent** (not a wrapper around another agent), with one binary, no daemon, and SQLite as the only persistence layer.

### Non-goals

- Not an IDE plugin. The TUI is the primary interface; CI pipe mode is the secondary.
- Not a hosted service. Telemetry, dashboards, and team features are explicitly deferred.
- Not a multi-agent framework. Sub-agents are out of scope for v1; kov runs one conversation at a time.
- Not a replacement for the model. Quality of output depends on the chosen provider; kov focuses on the harness.

---

## 2. Design principles

These are the tiebreakers when a design decision is ambiguous.

**Crash-only.** Every component assumes the process can die at any instant. There is no shutdown sequence required for correctness — the only authoritative state is on disk. A graceful shutdown path may exist for nicer UX, but correctness must not depend on it.

**Single source of truth: SQLite.** No in-memory data structure is authoritative. The agent loop reloads from SQLite at the start of each iteration. RAM is a cache.

**Idempotent provider calls.** Every outbound LLM request gets a UUID that is persisted *before* the network call begins. On restart, the reconciler examines requests in non-terminal state and decides whether they were lost, completed silently, or are still hanging.

**Typed errors, not strings.** Every error a provider can return maps to one of a small set of `ErrorClass` values. String matching on error messages is forbidden outside the per-provider classifier.

**The FSM is the spec.** The resilience engine is a finite state machine. If a behavior cannot be expressed as `(state, event) → (new_state, action)`, it does not belong in the engine — it belongs in the agent loop or the provider layer.

**One binary, no daemon.** Coordination between concurrent kov processes (different repos, different sessions) happens through SQLite WAL, not through a long-running service.

---

## 3. Repository layout

The intended package boundaries. Each directory has a single responsibility; cross-package imports flow downward only.

```
kov/
├── cmd/kov/                  Entry point: flag parsing, mode dispatch, exit codes
│   ├── main.go
│   ├── root.go               Cobra root command
│   └── commands/             Subcommands: resume, status, costs, diagnose
│
├── internal/
│   ├── store/                SQLite layer — the heart of the system
│   │   ├── schema.go         Migrations
│   │   ├── sessions.go
│   │   ├── messages.go
│   │   ├── tasks.go
│   │   ├── checkpoints.go
│   │   ├── requests.go       In-flight LLM request tracking
│   │   └── cost.go
│   │
│   ├── agent/                The loop
│   │   ├── loop.go           plan → execute → verify → checkpoint
│   │   ├── modes.go          code/plan/think/architect/...
│   │   ├── architect.go      Two-phase plan→edit
│   │   ├── compactor.go      Predictive context compaction
│   │   └── verify.go         Test runner integration
│   │
│   ├── provider/             LLM transport
│   │   ├── provider.go       The interface
│   │   ├── anthropic.go
│   │   ├── openai.go
│   │   ├── google.go
│   │   ├── ollama.go
│   │   ├── router.go         Failover chain
│   │   ├── ratelimit.go      Per-provider availability
│   │   └── stream.go         SSE reader with watchdog
│   │
│   ├── resilience/           The FSM and policy
│   │   ├── fsm.go            State machine
│   │   ├── classifier.go     error → ErrorClass
│   │   ├── budget.go         Cost enforcement
│   │   └── loopdetect.go     Doom-loop detection
│   │
│   ├── tools/                Built-in tool registry
│   │   ├── registry.go
│   │   ├── fs.go             read/write/edit
│   │   ├── shell.go          Bash with permission gating
│   │   ├── git.go
│   │   └── permission.go     confirm/auto/deny
│   │
│   ├── mcp/                  MCP client
│   │   ├── client.go
│   │   ├── stdio.go
│   │   └── http.go
│   │
│   ├── context/              Context window management
│   │   ├── tokens.go         Per-provider tokenizer
│   │   ├── window.go         Fill tracking, velocity
│   │   └── prune.go          Smart message pruning
│   │
│   ├── tui/                  Bubble Tea UI
│   ├── pipe/                 Non-interactive JSON stdin/out
│   └── config/               .kov.yaml + KOV.md/AGENTS.md/CLAUDE.md loader
│
├── agents/                   Built-in mode definitions
└── docs/                     This file lives here
```

The two layers that earn the "indestructible" tag are `store/` and `resilience/`. Disproportionate care belongs there.

---

## 4. Data model

SQLite, WAL mode, foreign keys ON, `synchronous=NORMAL`. The schema is the contract; everything else is implementation.

```sql
-- A single user invocation, possibly spanning many turns and resumes
CREATE TABLE sessions (
    id              TEXT PRIMARY KEY,            -- UUID
    repo_path       TEXT NOT NULL,               -- absolute, canonical
    mode            TEXT NOT NULL,               -- code/plan/think/architect/...
    status          TEXT NOT NULL,               -- running/paused/completed/failed
    pause_reason    TEXT,                        -- ratelimit/auth/network/budget/loop/manual
    pid             INTEGER,                     -- owning process; NULL when paused
    pid_started_at  INTEGER,                     -- to detect stale PIDs
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL,
    config_snapshot TEXT NOT NULL                -- resolved config JSON at session start
);

CREATE INDEX idx_sessions_repo_status ON sessions(repo_path, status);

-- Append-only conversation log
CREATE TABLE messages (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id   TEXT NOT NULL REFERENCES sessions(id),
    turn         INTEGER NOT NULL,
    role         TEXT NOT NULL,                  -- user/assistant/tool
    content      TEXT NOT NULL,                  -- JSON: content blocks
    tokens_in    INTEGER,
    tokens_out   INTEGER,
    created_at   INTEGER NOT NULL
);

CREATE INDEX idx_messages_session_turn ON messages(session_id, turn);

-- Sub-tasks the agent decomposed the prompt into
CREATE TABLE tasks (
    id           TEXT PRIMARY KEY,
    session_id   TEXT NOT NULL REFERENCES sessions(id),
    parent_id    TEXT REFERENCES tasks(id),
    description  TEXT NOT NULL,
    status       TEXT NOT NULL,                  -- pending/running/done/failed
    result       TEXT,
    created_at   INTEGER NOT NULL,
    completed_at INTEGER
);

-- The thing that makes resume work
CREATE TABLE checkpoints (
    id                TEXT PRIMARY KEY,
    session_id        TEXT NOT NULL REFERENCES sessions(id),
    turn              INTEGER NOT NULL,
    phase             TEXT NOT NULL,              -- between_turns/streaming/tool_exec/compaction
    active_request_id TEXT,                       -- if mid-LLM call
    active_tool_call  TEXT,                       -- JSON {tool, args, partial_result}
    pending_tasks     TEXT NOT NULL,              -- JSON array of task IDs
    file_snapshot_ref TEXT,                       -- git stash/commit hash
    context_summary   TEXT,                       -- present after compaction
    created_at        INTEGER NOT NULL
);

CREATE INDEX idx_checkpoints_session ON checkpoints(session_id, turn DESC);

-- Outbound LLM requests — the idempotency table
CREATE TABLE requests (
    id            TEXT PRIMARY KEY,               -- UUID, generated BEFORE network
    session_id    TEXT NOT NULL REFERENCES sessions(id),
    turn          INTEGER NOT NULL,
    provider      TEXT NOT NULL,
    model         TEXT NOT NULL,
    request_body  TEXT NOT NULL,                  -- exact JSON sent
    status        TEXT NOT NULL,                  -- sent/streaming/completed/failed/orphaned
    response_body TEXT,                           -- accumulated SSE (if any)
    error_class   TEXT,
    started_at    INTEGER NOT NULL,
    finished_at   INTEGER
);

CREATE INDEX idx_requests_session_status ON requests(session_id, status);

-- Per-event cost log, so /costs is fast and accurate
CREATE TABLE cost_events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id  TEXT NOT NULL REFERENCES sessions(id),
    request_id  TEXT REFERENCES requests(id),
    provider    TEXT NOT NULL,
    model       TEXT NOT NULL,
    tokens_in   INTEGER NOT NULL,
    tokens_out  INTEGER NOT NULL,
    cost_usd    REAL NOT NULL,
    created_at  INTEGER NOT NULL
);

-- Provider availability — shared across kov processes
CREATE TABLE provider_health (
    provider          TEXT PRIMARY KEY,
    available_at      INTEGER,                    -- unix ms; NULL = available now
    consecutive_fails INTEGER NOT NULL DEFAULT 0,
    last_error        TEXT,
    last_updated      INTEGER NOT NULL
);
```

The `requests` table is what makes the indestructible claim earn its keep. The invariant is: **a row is written before any network bytes leave the process**. On restart, any row in `sent` or `streaming` state is an orphan — the reconciler decides what to do with it.

---

## 5. The resilience FSM

The state machine is small enough to draw on a whiteboard.

### States

| State | Meaning |
|---|---|
| `RUNNING` | The agent loop is making progress. |
| `PAUSED_RATELIMIT` | A provider returned 429; waiting for `Retry-After`. |
| `PAUSED_AUTH` | A provider returned 401/403; needs user action. |
| `PAUSED_NETWORK` | No reachable provider; waiting for connectivity. |
| `PAUSED_BUDGET` | Cost budget hit; needs explicit user resume. |
| `PAUSED_LOOP` | Doom loop detected; needs explicit user resume. |
| `PAUSED_MANUAL` | User pressed Ctrl+C. |
| `COMPLETED` | Terminal — task finished successfully. |
| `FAILED` | Terminal — unrecoverable error (typically a bug). |

### Events

```
LLM_OK            successful response
LLM_429           rate limited
LLM_401           auth failure
LLM_5XX           provider server error
LLM_NETWORK       transport failure (ECONNRESET, DNS, etc.)
LLM_HANG          watchdog tripped: no bytes and no pings within timeout
TOOL_OK           tool executed successfully
TOOL_FAIL         tool returned error
VERIFY_OK         test command exited 0
VERIFY_FAIL       test command exited non-zero
BUDGET_HIT        cost crossed configured limit
LOOP_DETECTED     doom-loop detector fired
USER_INTERRUPT    Ctrl+C
USER_RESUME       `kov resume`
PROVIDER_RECOVERED  timer fired; provider available again
TURN_COMPLETE
TASK_COMPLETE
SESSION_COMPLETE
```

### Transitions

| From | Event | To | Action |
|---|---|---|---|
| `RUNNING` | `LLM_429` | `RUNNING` if any provider available, else `PAUSED_RATELIMIT` | mark provider unavailable until `retry_after`; failover if possible |
| `RUNNING` | `LLM_401` | `PAUSED_AUTH` | persist checkpoint; print re-auth instructions |
| `RUNNING` | `LLM_NETWORK` | `RUNNING` if Ollama configured, else `PAUSED_NETWORK` | persist checkpoint; failover to local if available |
| `RUNNING` | `LLM_HANG` | `RUNNING` | watchdog aborted; mark request failed; retry once non-streaming, then failover |
| `RUNNING` | `LLM_5XX` | `RUNNING` | exponential backoff up to N attempts, then failover |
| `RUNNING` | `BUDGET_HIT` | `PAUSED_BUDGET` | persist checkpoint; exit cleanly |
| `RUNNING` | `LOOP_DETECTED` | `PAUSED_LOOP` | persist checkpoint; print last 3 iterations |
| `RUNNING` | `USER_INTERRUPT` | `PAUSED_MANUAL` | finish current tool call if <2s away; else cancel and persist |
| `RUNNING` | `VERIFY_FAIL` | `RUNNING` | feed test output to LLM as next user turn; counts toward loop detection |
| `RUNNING` | `TASK_COMPLETE` | `RUNNING` | mark task done; advance |
| `RUNNING` | `SESSION_COMPLETE` | `COMPLETED` | finalize |
| `PAUSED_RATELIMIT` | `PROVIDER_RECOVERED` | `RUNNING` | resume from checkpoint |
| `PAUSED_AUTH` | `USER_RESUME` | `RUNNING` | reload credentials; resume |
| `PAUSED_NETWORK` | `PROVIDER_RECOVERED` | `RUNNING` | resume |
| `PAUSED_BUDGET` | `USER_RESUME` | `RUNNING` | reset budget counter; resume |
| `PAUSED_LOOP` | `USER_RESUME` | `RUNNING` | clear loop detector; resume |
| `PAUSED_MANUAL` | `USER_RESUME` | `RUNNING` | resume |

The FSM lives in `internal/resilience/fsm.go`. It should be testable with a property-based test: any random sequence of events must reach a terminal state without deadlocking.

---

## 6. Error classification

Every provider error is normalized to one `ErrorClass` before reaching the FSM. String matching belongs only in the per-provider classifier.

```go
// internal/resilience/classifier.go

type ErrorClass int

const (
    ClassUnknown ErrorClass = iota
    ClassRateLimited      // 429, with retry-after
    ClassAuthExpired      // 401, 403
    ClassQuotaExhausted   // billing/credit out — longer backoff
    ClassNetworkDown      // ECONNRESET, EHOSTUNREACH, DNS
    ClassStreamHang       // SSE idle past timeout, no pings
    ClassServerError      // 5xx
    ClassBadRequest       // 400 — usually our bug, do not retry
    ClassContextOverflow  // model says context too large
    ClassToolFailure      // tool exited non-zero or threw
)

type ClassifiedError struct {
    Class      ErrorClass
    Provider   string
    HTTPStatus int
    RetryAfter time.Duration
    Underlying error
    Hint       string  // human-readable for TUI
}

func Classify(provider string, err error, resp *http.Response) ClassifiedError
```

Each provider package implements its own classifier method because Anthropic, OpenAI, and Google return different shapes for the same logical error.

The mapping from `ErrorClass` to FSM event is a single small function, e.g. `ClassRateLimited → LLM_429`.

---

## 7. The streaming watchdog

The leaked Claude Code source revealed that its streaming watchdog has unreachable fallback code that has been broken for months. The kov watchdog is designed to avoid this by tracking two clocks instead of one.

```go
// internal/provider/stream.go

type StreamWatchdog struct {
    IdleTimeout time.Duration  // default 30s — no bytes at all
    PingTimeout time.Duration  // default 60s — no SSE pings
    OnDead      func()         // called on confirmed dead connection
}
```

The two-clock rule:

- `lastDataAt`: any byte received from the SSE stream (data, ping, comment).
- `lastPingAt`: a ping event specifically.

A connection is treated as **dead** when both are exceeded:
- `now - lastDataAt > IdleTimeout` AND
- `now - lastPingAt > PingTimeout`.

If `lastPingAt` is recent but `lastDataAt` is old, the model is still thinking — keep waiting. If both are old, abort the request, mark it failed with `ClassStreamHang`, and emit `LLM_HANG`.

This is the difference between "the CLI hangs forever" and "kov gracefully retries within 60 seconds" and is the single most user-visible reliability win.

---

## 8. The provider router

```go
// internal/provider/router.go

type Router struct {
    chain  []Provider     // ordered by config preference
    health *HealthTracker // backed by provider_health table
    store  *store.Store
}

func (r *Router) Send(ctx context.Context, req Request) (*Response, error) {
    for _, p := range r.chain {
        if !r.health.Available(p.Name()) {
            continue
        }

        // 1. Persist intent BEFORE the network call.
        reqID := r.store.RecordRequest(req, p.Name())

        resp, err := p.Send(ctx, req)
        if err != nil {
            ce := p.Classify(err, resp)
            r.health.RecordFailure(p.Name(), ce)
            r.store.MarkRequestFailed(reqID, ce)

            switch ce.Class {
            case ClassRateLimited, ClassQuotaExhausted,
                 ClassServerError, ClassNetworkDown:
                continue   // try the next provider
            default:
                return nil, ce  // not a failover situation
            }
        }

        r.store.MarkRequestComplete(reqID, resp)
        r.health.RecordSuccess(p.Name())
        return resp, nil
    }
    return nil, ErrAllProvidersExhausted
}
```

The `HealthTracker` reads from the `provider_health` table on every check, so concurrent kov processes share knowledge of which provider is rate-limited. A background goroutine flips entries back to available when their `available_at` passes.

---

## 9. Predictive context management

The agent loop checks window fill at the top of every iteration. When fill exceeds `CompactAt` (default 0.90), kov triggers compaction down to `CompactDownTo` (default 0.70).

```go
// internal/context/window.go

type Window struct {
    MaxTokens     int
    CurrentTokens int
    CompactAt     float64  // 0.90
    CompactDownTo float64  // 0.70

    history []TokenEvent  // ring buffer of recent (delta, timestamp)
}

func (w *Window) Velocity() (turnsRemaining int)
func (w *Window) ShouldCompact() bool
```

Compaction is implemented as a separate, cheaper LLM call (Haiku, Flash, or equivalent). It receives the current message history and returns a structured summary covering completed tasks, decisions made, open questions, and file paths touched. The compactor must produce JSON matching a fixed schema — a free-form summary is not acceptable because it cannot be diffed or audited.

If the compactor itself fails, the FSM treats it as `LLM_NETWORK`. The session pauses; the original messages are not dropped. Compaction is recorded as a checkpoint with `phase=compaction` so resume can detect a partial compaction state.

---

## 10. Doom-loop detection

```go
// internal/resilience/loopdetect.go

type LoopDetector struct {
    Threshold int  // default 3
    history   []LoopSignal
}

type LoopSignal struct {
    ToolName   string
    ArgsHash   string  // SHA-256 of normalized args
    OutputHash string
    Timestamp  time.Time
}
```

Heuristics that emit `LOOP_DETECTED`:

1. The same tool with the same args producing the same output, three times in a row.
2. `VERIFY_FAIL` more than five times in a row (test still red after five attempts).
3. The same file written, then immediately re-read, three or more times.

Thresholds are per-mode. `fast` mode trips earlier than `code` mode. Tune by replaying real session logs.

---

## 11. The agent loop

The loop is small. Most complexity lives in the layers it calls into.

```go
// internal/agent/loop.go

func (a *Agent) Run(ctx context.Context, sessionID string) error {
    for {
        // 0. Reload from disk. RAM is just a cache.
        sess := a.store.LoadSession(sessionID)
        if sess.Status != "running" {
            return nil
        }

        // 1. Resilience preconditions
        if a.budget.Exceeded(sessionID) {
            a.fsm.Emit(BUDGET_HIT)
            continue
        }
        if a.window.ShouldCompact() {
            if err := a.compact(sessionID); err != nil {
                a.fsm.Emit(LLM_NETWORK)
                continue
            }
        }

        // 2. Build request from current state
        msgs := a.store.LoadMessages(sessionID)
        req := a.buildRequest(msgs)

        // 3. Send via the router (handles failover internally)
        resp, err := a.router.Send(ctx, req)
        if err != nil {
            a.fsm.Emit(eventForError(err))
            continue
        }

        // 4. Persist the assistant response
        a.store.AppendMessage(sessionID, resp.Message)
        a.budget.Record(resp.Cost)

        // 5. Execute tool calls
        for _, call := range resp.ToolCalls {
            cpID := a.store.WriteCheckpoint(sessionID, "tool_exec", call)

            result, err := a.tools.Execute(ctx, call)
            if err != nil {
                a.fsm.Emit(TOOL_FAIL)
                a.store.AppendMessage(sessionID, errorMsg(call, err))
                continue
            }

            a.loopDetect.Record(call, result)
            if a.loopDetect.Detect() {
                a.fsm.Emit(LOOP_DETECTED)
                break
            }

            a.store.AppendMessage(sessionID, toolMsg(call, result))
            a.store.ClearCheckpoint(cpID)
        }

        // 6. Verify if no tool calls and verify is enabled
        if len(resp.ToolCalls) == 0 && a.config.Verify.Enabled {
            if err := a.verify.Run(ctx); err != nil {
                a.fsm.Emit(VERIFY_FAIL)
            } else {
                a.fsm.Emit(VERIFY_OK)
            }
        }

        if a.isDone(resp) {
            a.fsm.Emit(SESSION_COMPLETE)
            return nil
        }
    }
}
```

The loop does not contain try/catch sprawl. Every error becomes an FSM event and the FSM decides whether to continue, pause, or terminate.

---

## 12. Resume semantics

`kov resume` is the user-facing payoff for everything above.

```go
// cmd/kov/commands/resume.go

func Resume(repoPath string) error {
    // 1. Find most recent paused session in this repo
    sess := store.FindPausedSession(repoPath)
    if sess == nil {
        return errors.New("no paused session in this directory")
    }

    // 2. Reconcile orphan requests
    for _, o := range store.FindOrphanRequests(sess.ID) {
        if time.Since(o.StartedAt) > 5*time.Minute && o.ResponseBody == "" {
            store.MarkRequestFailed(o.ID, ClassUnknown)
        }
    }

    // 3. Find the latest checkpoint
    cp := store.LatestCheckpoint(sess.ID)

    // 4. If interrupted mid-tool, ask the user
    if cp.Phase == "tool_exec" && cp.ActiveToolCall != "" {
        if !promptUser(cp.ActiveToolCall) {
            store.RollbackToCheckpoint(sess.ID, cp.ID)
        }
    }

    // 5. Mark running and start the loop
    store.UpdateSessionStatus(sess.ID, "running")
    return agent.Run(ctx, sess.ID)
}
```

Important UX detail: on resume, kov prints a one-line summary of what it was doing (`Editing src/auth.ts, was about to run npm test`) and asks for confirmation before continuing. That confirmation prompt is what makes resume feel safe. It is the reason the user trusts kov to handle a `kill -9` differently from how they'd trust any other tool.

---

## 13. Concurrency and process model

A single kov process owns one session. Multiple kov processes can run concurrently in different repositories because:

- SQLite WAL handles concurrent writers, with brief retries on `SQLITE_BUSY`.
- Each session row holds `pid` and `pid_started_at`. A future kov can detect that a `running` session has a dead owner (PID gone, or PID alive but `pid_started_at` doesn't match) and offer to take over.
- The `provider_health` table is shared, so rate-limit knowledge propagates across processes.

Within a single kov process, the goroutine layout is:

| Goroutine | Responsibility |
|---|---|
| main | The agent loop |
| stream | SSE reader, writes to a channel |
| watchdog | Timer firing on idle, signals stream goroutine |
| fsm | Drains an event channel, applies transitions |
| tui (if `--tui`) | Bubble Tea Update/View loop |
| ratelimit-timer | Periodically flips `provider_health` rows back to available |

All cross-goroutine communication uses channels. The only shared mutable state is the `*sql.DB` handle, which has its own internal mutex.

---

## 14. Observability

A `--debug` flag writes structured JSONL to `~/.kov/logs/<session_id>.jsonl`. Every FSM transition, every provider call, and every tool execution gets a line.

```json
{"ts":1714935600000,"session":"abc","event":"fsm.transition","from":"RUNNING","to":"PAUSED_RATELIMIT","reason":"anthropic 429"}
{"ts":1714935601000,"session":"abc","event":"provider.call","provider":"anthropic","model":"claude-sonnet-4","request_id":"r1","duration_ms":4521,"tokens_in":8200,"tokens_out":432}
```

A `kov diagnose <session_id>` subcommand prints a timeline summary from these logs. This is also the evidence trail when a user reports a bug — almost always the log will show what actually happened.

---

## 15. Configuration

Configuration is loaded in this order, with later sources overriding earlier ones:

1. Built-in defaults.
2. `~/.kov/config.yaml` (user-level).
3. `.kov.yaml` in the repository root.
4. Environment variables (`ANTHROPIC_API_KEY`, etc.).
5. CLI flags.

Instructions are loaded from, in order: `KOV.md`, `AGENTS.md`, `CLAUDE.md`. The first one found wins; subsequent files are not merged. This avoids the multi-file ambiguity that has caused issues in other tools.

The resolved configuration is snapshotted into `sessions.config_snapshot` at session start, so resume uses exactly the same configuration the original invocation used — even if the user has since edited their `.kov.yaml`.

---

## 16. Testing strategy

The architecture is designed to be testable in three layers.

**Unit tests** for the pure functions: error classification, FSM transitions, window fill math, loop detection, request building.

**FSM property tests.** Generate random event sequences and assert: (a) no deadlock, (b) every sequence reaches a terminal state, (c) any pause state has a matching resume event.

**Integration tests with a fake provider.** A mock provider that can be scripted to return specific errors at specific points (`return 429 on the third request, then 200`). This is how the rate-limit handling, watchdog, and failover paths get exercised without hitting real APIs.

**Crash tests.** A test harness that runs the agent loop, sends `SIGKILL` at random points, then runs `kov resume` and asserts the session reaches the same final state as a non-crashed run.

---

## 17. Trade-offs and known limitations

**SQLite as the only store.** Fast, simple, transactional, no extra dependency. But: no replication, no team-wide visibility. Acceptable for v1; revisit for team features.

**No streaming output to the user during tool execution.** Tools run to completion before their output is shown. This is a UX cost paid for crash-safety: partial tool output is hard to checkpoint correctly.

**Reconciler is heuristic.** The 5-minute threshold for marking orphan requests as failed is a heuristic, not a guarantee. Anthropic and OpenAI do not currently expose request-status endpoints; if they did, the reconciler could be exact.

**Compaction loses fidelity.** Any compaction loses information by definition. The mitigation is the JSON-schema constraint plus the `phase=compaction` checkpoint, so a user can roll back if the compaction was bad. This does not mean compaction is free.

**No cross-session memory.** Each session starts fresh. Accumulating cross-session knowledge is a separate problem (see claude-mem and similar projects); kov v1 does not solve it.

---

## 18. What's deferred

- Daemon mode (long-running background service).
- Web dashboard.
- Team / hosted features.
- Sub-agents (parallel sub-conversations).
- Cross-CLI sidecar mode (wrapping Claude Code, Cursor, etc.).
- Cross-session memory.

Each of these is a real feature, but each pulls focus from the v1 thesis. They belong on a roadmap, not in v1 scope.

---

## 19. Glossary

- **Harness** — the program around the model that handles tools, streaming, errors, and state. Everything in this document is the harness.
- **Turn** — one user message + the assistant's response, possibly including tool calls.
- **Iteration** — one trip around the agent loop. A single turn can require multiple iterations if the assistant calls tools.
- **Checkpoint** — a row in the `checkpoints` table marking a recoverable state. Distinct from a SQLite checkpoint (WAL mechanic).
- **Orphan request** — a row in `requests` whose owning process died before reaching a terminal status.
- **Failover** — switching to the next provider in the chain when the current one fails.
- **Compaction** — replacing old messages with a structured summary to free context window space.
- **Doom loop** — the agent repeating the same action with the same result. The loop detector exists to break these.
