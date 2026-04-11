# Kov Agents

This directory contains agent configuration and system prompt templates used by Kov.

## What is an Agent?

In Kov, an "agent" is the core loop that:
1. Takes a user prompt
2. Builds context (project instructions, repo map, conversation history)
3. Streams a request to an LLM with tool schemas
4. Executes returned tool calls (file edits, shell commands, searches)
5. Repeats until the LLM stops requesting tools
6. Verifies changes (runs tests)

## Modes

Each mode configures the agent differently:

| File | Mode | Description |
|------|------|-------------|
| [code.md](./code.md) | code | Full agentic coding (default) |
| [plan.md](./plan.md) | plan | Read-only analysis |
| [think.md](./think.md) | think | Extended reasoning |
| [ask.md](./ask.md) | ask | Quick Q&A |
| [fast.md](./fast.md) | fast | Minimal overhead |
| [research.md](./research.md) | research | Deep investigation |
| [architect.md](./architect.md) | architect | Dual-model planning |
| [review.md](./review.md) | review | Git diff review |

## Customizing

Create a `KOV.md` file in your project root to add project-specific instructions that get injected into every agent prompt. Example:

```markdown
# Project Rules

- Use Go 1.26+
- All tests must pass with -race flag
- Use slog for logging, never fmt.Println
- Follow conventional commits
- Always run go vet before committing
```

## Tool Schemas

The agent has access to these tools:

- **file_read** — Read file contents (with optional line range)
- **file_write** — Create or overwrite files
- **file_edit** — Search-and-replace within files
- **shell_exec** — Execute shell commands (sandboxed)
- **grep_search** — Search for patterns in files
- **glob_search** — Find files matching glob patterns
- **list_dir** — List directory contents
