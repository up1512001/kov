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

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go)](https://go.dev)
[![Release](https://img.shields.io/github/v/release/utsavkovy/kov)](https://github.com/utsavkovy/kov/releases)

[Website](https://trykov.dev) · [Documentation](https://trykov.dev/docs) · [Discord](https://discord.gg/kov)

</div>

---

**Kov** is an open-source AI coding CLI that makes your coding sessions **indestructible**.

Rate limit hit? Auth expired? Network dropped? Process killed? Kov recovers automatically and picks up exactly where you left off.

## Why Kov?

Every AI coding CLI today — Claude Code, Gemini CLI, Codex — crashes and loses all context when something goes wrong mid-task. You lose 20+ minutes of AI reasoning, partial file changes are left broken, and you start over.

**Kov solves this.** It's the only tool that:

- 🔄 **Auto-recovers** from rate limits (429), auth expiry (401), network drops, and crashes
- 💾 **Checkpoints** every sub-task to SQLite — survives `kill -9` and reboots
- 🔀 **Auto-failovers** from cloud to local (Ollama) when providers go down
- 💰 **Enforces cost budgets** — auto-stops before overspending
- 🔁 **Detects doom loops** — auto-pauses when the AI is going in circles
- ✅ **Verifies every change** — auto-runs tests, never fakes success

## Quick Start

```bash
# Install
curl -fsSL trykov.dev/install.sh | sh

# Or with Homebrew
brew install utsavkovy/tap/kov

# Or from source
go install github.com/utsavkovy/kov/cmd/kov@latest
```

```bash
# Set your API key
export ANTHROPIC_API_KEY=sk-ant-...

# Start coding
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
```

## Features

### 🛡️ Indestructible Sessions

| Failure | Other tools | Kov |
|---------|------------|-----|
| Rate limit (429) | 💀 Crash | ⏳ Auto-wait → retry |
| Auth expired (401) | 💀 Crash | ⏸️ Pause → re-auth → `kov resume` |
| Network drop | 💀 Crash | 🔀 Failover to local AI |
| `kill -9` | 💀 Lost forever | 💾 `kov resume` → exact recovery |
| Ctrl+C | 💀 Hard kill | ✅ Graceful checkpoint |
| Budget exceeded | 💸 Silent overspend | 🛑 Auto-stop at limit |
| Doom loop | ♾️ Burns $40 | 🔁 Auto-detect → pause |

### 🤖 9 Agent Modes

- **`code`** — Full agentic coding (default)
- **`plan`** — Read-only analysis, generates plan.md
- **`think`** — Extended reasoning with thinking models
- **`ask`** — Quick Q&A about your codebase
- **`fast`** — Minimal overhead, direct edits
- **`research`** — Multi-step deep codebase investigation
- **`architect`** — Two-pass: plan with strong model, edit with fast model
- **`review`** — Reviews your staged git changes
- **`pipe`** — Non-interactive JSON stdin/stdout for CI/CD

### 🔌 Works with All Models

| Provider | Models | Setup |
|----------|--------|-------|
| Anthropic | Claude Sonnet 4, Opus 4 | `ANTHROPIC_API_KEY` |
| OpenAI | GPT-4.1, o3, o4-mini | `OPENAI_API_KEY` |
| Google | Gemini 2.5 Pro, Flash | `GEMINI_API_KEY` |
| Ollama | Any local model | `ollama serve` |

BYOK (Bring Your Own Key) — you pay your provider directly. Zero markup.

### ⚡ Performance

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
```

See [`.kov.yaml.example`](.kov.yaml.example) for all options.

## How It Works

```
User prompt → LLM decomposes into atomic tasks → SQLite queue
→ Execute task → Verify (run tests) → Git commit → Checkpoint → Next task
→ If failure: classify error → retry / failover / pause
→ On resume: load checkpoint → retry failed task → continue
```

## License

MIT — see [LICENSE](LICENSE).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.
Built with ❤️ by [Utsav](https://github.com/utsavkovy)
