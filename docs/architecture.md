# Kov Architecture

## Overview

Kov is built as a **modular, event-driven Go application** with crash-proof persistence. Every subsystem communicates through a typed event bus, enabling loose coupling and easy testing.

```
┌─────────────────────────────────────────────────────────┐
│                       CLI (Cobra)                       │
│              kov "prompt" / kov resume / etc            │
├─────────────────────────────────────────────────────────┤
│                     App Orchestrator                    │
│         Initializes stack, wires dependencies           │
├──────────┬──────────┬───────────┬───────────────────────┤
│  Agent   │ Session  │   TUI     │    Pipe Mode          │
│  Loop    │ Manager  │ (Bubble   │  (stdin/stdout)       │
│          │          │   Tea)    │                       │
├──────────┴──────────┴───────────┴───────────────────────┤
│                     Event Bus                           │
│           Typed pub/sub, race-free                      │
├──────────┬──────────┬───────────┬───────────────────────┤
│ Provider │  Tool    │ Resilience│    Context            │
│ Router   │ Registry │  Engine   │   (KOV.md, repo map) │
├──────────┼──────────┼───────────┼───────────────────────┤
│ Anthropic│ file_read│   FSM     │  Token Counter        │
│ OpenAI   │ file_write│ Checkpoint│  Compactor           │
│ Google   │ file_edit│  Loop Det │                       │
│ Ollama   │ shell_exec│ Budget   │                       │
│          │ grep/glob│           │                       │
│          │ list_dir │           │                       │
├──────────┴──────────┴───────────┴───────────────────────┤
│                    SQLite (WAL mode)                    │
│     Sessions │ Messages │ Checkpoints │ Cost Events     │
└─────────────────────────────────────────────────────────┘
```

## Core Principles

1. **Crash-proof**: Every state transition is checkpointed to SQLite. The agent survives `kill -9`, power loss, and network failure.

2. **Zero CGO**: Pure Go only. Uses `modernc.org/sqlite` instead of `mattn/go-sqlite3`. Cross-compiles for all platforms without C toolchains.

3. **Lazy initialization**: Sub-50ms cold start. Database, providers, and tools initialize only on first use via `Lazy[T]` generics.

4. **Event-driven**: Components communicate via the typed event bus (`internal/bus`), not direct method calls. This enables logging, metrics, and TUI updates without coupling.

5. **Provider-agnostic**: The agent loop doesn't know which LLM it's talking to. The router handles failover transparently.

## Package Dependency Graph

```
cmd/kov
  └── internal/app
        ├── internal/agent
        │     ├── internal/bus
        │     ├── internal/context
        │     ├── internal/db
        │     ├── internal/provider
        │     ├── internal/resilience
        │     └── internal/tools
        ├── internal/session
        │     ├── internal/agent
        │     ├── internal/bus
        │     └── internal/db
        ├── internal/config
        ├── internal/tui
        └── internal/pipe
```

## Data Flow

### Normal Request
```
1. User: kov "fix the auth bug"
2. App: creates session in SQLite
3. Agent: loads context (KOV.md + repo map + conversation history)
4. Agent: streams LLM request via Router
5. Router: tries primary provider → fallback → local Ollama
6. Agent: receives tool calls → executes via Registry
7. Agent: checkpoints after each tool round
8. Agent: loops until no more tool calls
9. Agent: runs verification (auto-detected tests)
10. Git: auto-commits changes
```

### Crash Recovery
```
1. Process killed mid-task
2. User: kov resume
3. Session Manager: finds last interrupted session
4. Resilience Engine: loads checkpoint from SQLite
5. Agent: rebuilds conversation from messages table
6. Agent: continues from last checkpoint
```

## Key Types

| Type | Package | Purpose |
|------|---------|---------|
| `Agent` | agent | Orchestrates plan→execute→verify |
| `Router` | provider | Failover chain across providers |
| `Engine` | resilience | FSM + checkpoints + loop detection |
| `Registry` | tools | Tool registration + execution |
| `Bus` | bus | Typed event pub/sub |
| `DB` | db | SQLite with write serialization |
| `Manager` | session | Session lifecycle (list/resume/delete) |
| `Compactor` | tokens | Context window management |

## Configuration Cascade

```
CLI flags (highest priority)
  → Environment variables (KOV_*, ANTHROPIC_API_KEY, etc.)
    → Project .kov.yaml
      → User ~/.config/kov/config.yaml
        → Defaults (lowest priority)
```
