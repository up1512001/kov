# Agent Loop — How Kov Thinks

## The Plan → Execute → Verify Cycle

Kov's agent loop is a structured cycle that ensures reliable code changes:

```
                    ┌─────────┐
                    │  User   │
                    │ Prompt  │
                    └────┬────┘
                         │
                    ┌────▼────┐
                    │  Build  │
                    │ Context │ KOV.md + repo map + history
                    └────┬────┘
                         │
              ┌──────────▼──────────┐
              │   Stream LLM Call   │
              │  (with tool schemas)│
              └──────────┬──────────┘
                         │
                    ┌────▼────┐    No tool calls
                    │ Tool    ├──────────────────► Done
                    │ Calls?  │
                    └────┬────┘
                         │ Yes
                    ┌────▼────┐
                    │ Execute │
                    │  Tools  │ file_read, file_write, shell_exec, etc.
                    └────┬────┘
                         │
                    ┌────▼────┐
                    │Checkpoint│ Save state to SQLite
                    └────┬────┘
                         │
                    ┌────▼────┐
                    │  Loop   │ Check for doom loops
                    │ Detect  │
                    └────┬────┘
                         │
              ┌──────────▼──────────┐
              │  Next Iteration     │
              │  (back to LLM call) │
              └─────────────────────┘
```

## Modes

| Mode | Model | Tools | Thinking | Description |
|------|-------|-------|----------|-------------|
| `code` | default | all | normal | Full agentic coding |
| `plan` | default | read-only | normal | Analysis without edits |
| `think` | default | all | extended (20K) | Deep reasoning |
| `ask` | default | read-only | normal | Quick Q&A |
| `fast` | default | all | none | Minimal overhead |
| `research` | default | all | medium (10K) | Multi-step investigation |
| `architect` | dual | all | high (30K) | Plan with strong model, edit with fast |
| `review` | default | read + shell | normal | Review staged git changes |
| `pipe` | default | all | normal | JSON stdin/stdout for CI |

## Permission System

Tools are categorized by risk:

| Category | Tools | `confirm` | `smart` | `yolo` |
|----------|-------|-----------|---------|--------|
| Read | file_read, grep, glob, list_dir | ✅ auto | ✅ auto | ✅ auto |
| Write | file_write, file_edit | ❓ ask | ✅ auto | ✅ auto |
| Execute | shell_exec | ❓ ask | ❓ ask | ✅ auto |

## Resilience

### State Machine

```
idle → planning → executing → verifying → done
         │           │            │
         └───────────┴────────────┘
                     │
              error_wait / paused
```

### Failover Chain

```
Primary (e.g., Anthropic)
  │ 429/5xx/timeout
  ▼
Fallback (e.g., OpenAI)
  │ 429/5xx/timeout
  ▼
Emergency (Ollama local)
  │ failure
  ▼
Pause → kov resume
```

### Loop Detection

The engine tracks recent tool calls in a sliding window. If the same tool is called more than `threshold` times consecutively, the agent auto-pauses to prevent burning through budget on doom loops.

### Budget Enforcement

Cost is tracked per API call using the known models pricing table. When session cost exceeds `cost.budgetPerSession`, the agent auto-pauses.

## Verification

After the agent finishes tool calls, it runs a verification step:

1. **Auto-detect**: Inspects project files to guess the test command
   - `go.mod` → `go test ./...`
   - `package.json` → `npm run test`
   - `Cargo.toml` → `cargo test`
   - `Makefile` → `make test`
   - etc.

2. **Verify→Fix loop**: If tests fail, the error output is fed back to the LLM for up to `maxFixRetries` attempts.

## Context Assembly

The system prompt includes:

1. **Base instructions** (always present)
2. **Mode suffix** (e.g., "Mode: CODE — Full agentic coding")
3. **KOV.md** (project-specific instructions, if present)
4. **Repo map** (file tree with sizes, respecting .kovignore)
5. **Conversation history** (from SQLite, with auto-compaction)
