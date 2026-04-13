<div align="center">

```
  ██╗  ██╗ ██████╗ ██╗   ██╗
  ██║ ██╔╝██╔═══██╗██║   ██║
  █████╔╝ ██║   ██║██║   ██║
  ██╔═██╗ ██║   ██║╚██╗ ██╔╝
  ██║  ██╗╚██████╔╝ ╚████╔╝
  ╚═╝  ╚═╝ ╚═════╝   ╚═══╝
```

### Indestructible AI coding in your terminal

*kov (ков)* — from the Slavic root meaning **"to forge."** Kov forges your code, unbreakably.

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev)
[![Release](https://img.shields.io/github/v/release/up1512001/kov)](https://github.com/up1512001/kov/releases)

[Website](https://trykov.dev) · [Documentation](https://trykov.dev/docs)

</div>

---

**Kov** is an open-source AI coding CLI that makes your coding sessions **indestructible**.

Rate limit hit? Auth expired? Network dropped? Process killed? Kov recovers automatically and picks up exactly where you left off.

## Why Kov?

Every AI coding CLI today — Claude Code, Gemini CLI, Codex — crashes and loses all context when something goes wrong mid-task. You lose 20+ minutes of AI reasoning, partial file changes are left broken, and you start over.

**Kov solves this.** It's the only tool that:

- **Auto-recovers** from rate limits (429), auth expiry (401), network drops, and crashes
- **Checkpoints** every sub-task to SQLite — survives `kill -9` and reboots
- **Auto-failovers** from cloud to local (Ollama) when providers go down
- **Predictive context management** — compacts at 90% window fill, not at overflow
- **Enforces cost budgets** — auto-stops before overspending
- **Detects doom loops** — auto-pauses when the AI is going in circles
- **Verifies every change** — auto-runs tests, never fakes success

## Quick Start

```bash
# Install with Homebrew (macOS / Linux)
brew install up1512001/tap/kov

# Or from source
go install github.com/up1512001/kov/cmd/kov@latest

# Or download a binary from GitHub Releases
# https://github.com/up1512001/kov/releases
```

```bash
# Set your API key
export ANTHROPIC_API_KEY=sk-ant-...

# Start interactive REPL
kov

# Single-shot mode
kov "refactor auth to use JWT with refresh tokens"

# Resume after interruption
kov resume

# Different modes
kov --mode plan "analyze this repo's architecture"
kov --mode think "debug this flaky test"
kov --mode fast "fix the typo in README"
kov --mode research "how does the auth flow work?"
kov --mode architect "add GraphQL layer"
kov --mode review  # review staged changes

# CI/CD pipe mode
echo '{"prompt":"fix the bug"}' | kov --mode pipe
```

## Features

### Indestructible Sessions

| Failure | Other tools | Kov |
|---------|------------|-----|
| Rate limit (429) | Crash | Auto-wait, retry, or failover to another provider |
| Auth expired (401) | Crash | Pause, re-auth, `kov resume` |
| Network drop | Crash | Failover to local AI |
| `kill -9` | Lost forever | `kov resume` — exact recovery from checkpoint |
| Ctrl+C | Hard kill | Graceful checkpoint |
| Budget exceeded | Silent overspend | Auto-stop at limit |
| Doom loop | Burns $40 | Auto-detect, pause |
| Context window 90% full | Overflow crash | Proactive compaction with headroom |

### Predictive Context Management

Unlike every other tool that waits until the context window overflows:

- **Tracks window fill** in real-time — shown in the status bar (green/yellow/red)
- **Proactive compaction at 90%** — compacts down to 70% to leave headroom
- **Velocity estimation** — predicts how many iterations before window fills
- **Smart message pruning** — keeps system prompt + first user message + recent history

### Smart Rate Limit Handling

The biggest gap in every competitor — kov handles rate limits intelligently:

- **Respects Retry-After headers** — waits exactly as long as the API says
- **Auto-failover** — if provider A is rate-limited, seamlessly continues on provider B
- **Real-time TUI updates** — shows which provider is rate-limited and when it clears
- **Per-provider tracking** — knows which providers are available right now

### 9 Agent Modes

| Mode | Description | Tools |
|------|-------------|-------|
| `code` | Full agentic coding (default) | All |
| `plan` | Read-only analysis, generates plan | Read-only |
| `think` | Extended reasoning with thinking models | All |
| `ask` | Quick Q&A about your codebase | Read-only |
| `fast` | Minimal overhead, direct edits | All |
| `research` | Multi-step deep codebase investigation | All |
| `architect` | Two-phase: plan with strong model, execute with fast model | All |
| `review` | Reviews your staged git changes | Read + shell |
| `pipe` | Non-interactive JSON stdin/stdout for CI/CD | All |

**Architect mode** is genuinely two-phase — Phase 1 uses your plan model (read-only, extended thinking) to produce a structured plan, Phase 2 uses your edit model to execute it. No other tool does this.

### Works with All Models

| Provider | Models | Setup |
|----------|--------|-------|
| Anthropic | Claude Sonnet 4, Opus 4 | `ANTHROPIC_API_KEY` |
| OpenAI | GPT-4.1, o3, o4-mini | `OPENAI_API_KEY` |
| Google | Gemini 2.5 Pro, Flash | `GEMINI_API_KEY` |
| Ollama | Any local model | `ollama serve` |
| MCP Servers | Any MCP-compatible tool server | Config in `.kov.yaml` |

BYOK (Bring Your Own Key) — you pay your provider directly. Zero markup.

### Standards Support

- **AGENTS.md** — reads the OpenAI-originated agent instructions standard (60K+ repos)
- **KOV.md / CLAUDE.md** — project-specific instructions
- **MCP (Model Context Protocol)** — connect any MCP tool server

### Performance

- **<50ms** cold start (Go binary, zero deps)
- **<15MB** idle memory (DB-backed history, zero-copy streaming)
- **Single binary** — no runtime, no Node.js, no Python
- **Zero CGO** — cross-compiles everywhere

## Configuration

Create `.kov.yaml` in your project:

```yaml
model: claude-sonnet-4-20250514
provider: anthropic

resilience:
  checkpoint: true
  failover: true
  loopDetection:
    threshold: 3

verify:
  enabled: true
  command: "npm test"

cost:
  budgetPerSession: 10.00

permissions: confirm

# Architect mode can use different models for each phase
modes:
  architect:
    planModel: claude-opus-4-20250514    # strong model for analysis
    editModel: claude-sonnet-4-20250514  # fast model for edits

# MCP tool servers
mcpServers:
  filesystem:
    command: "npx"
    args: ["-y", "@modelcontextprotocol/server-filesystem", "/path/to/dir"]
```

See [`.kov.yaml.example`](.kov.yaml.example) for all options.

## How It Works

```
User prompt → LLM plans approach → Execute with tools → Verify (run tests)
→ Checkpoint to SQLite → Next iteration
→ If 429: respect Retry-After → retry or failover to next provider
→ If context 90% full: proactive compaction → continue
→ If failure: classify error → retry / failover / pause
→ On resume: load checkpoint → retry failed task → continue
```

## Architecture

```
┌──────────────────────────────────────────────┐
│  TUI (Bubble Tea)  or  Pipe (JSON stdin/out) │
├──────────────────────────────────────────────┤
│  Agent Loop (plan → execute → verify)        │
│  ├── Architect Mode (two-phase plan→edit)    │
│  ├── Context Manager (predictive compaction) │
│  └── Tool Registry (built-in + MCP)         │
├──────────────────────────────────────────────┤
│  Provider Router (failover chain + health)   │
│  ├── Rate limit tracking per provider        │
│  ├── Retry with Retry-After / backoff        │
│  └── Auto-failover on exhaustion             │
├──────────────────────────────────────────────┤
│  Resilience Engine (FSM + checkpoints)       │
│  ├── Error classification (429/401/5xx/etc)  │
│  ├── Cost budget enforcement                 │
│  └── Loop detection                          │
├──────────────────────────────────────────────┤
│  SQLite (WAL mode) — sessions, messages,     │
│  tasks, checkpoints, cost events             │
└──────────────────────────────────────────────┘
```

## License

MIT — see [LICENSE](LICENSE).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

Built by [Utsav](https://github.com/up1512001)
