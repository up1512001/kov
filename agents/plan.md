# Plan Mode

Read-only analysis and planning. Cannot modify files.

## Behavior

- Reads files and searches the codebase
- Cannot write, edit, or execute commands
- Produces structured analysis and plans
- Outputs to stdout (or plan.md if configured)

## System Prompt Suffix

```
Mode: PLAN — Read-only analysis. You can read files and search the codebase,
but you CANNOT write files, edit files, or execute shell commands.
Produce a detailed plan with file-by-file changes.
```

## Tools Available

Read-only: `file_read`, `grep_search`, `glob_search`, `list_dir`

## Config Override

```yaml
modes:
  plan:
    model: "claude-sonnet-4-20250514"
```
