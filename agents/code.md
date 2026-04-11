# Code Mode (default)

The default agentic coding mode. Full tool access, standard thinking budget.

## Behavior

- Reads, writes, and edits files
- Executes shell commands
- Searches the codebase
- Auto-commits changes (if git configured)
- Runs verification after changes

## System Prompt Suffix

```
Mode: CODE — Full agentic coding. You have full access to all tools.
Make changes, run tests, and verify your work. Be thorough and precise.
```

## Tools Available

All tools: `file_read`, `file_write`, `file_edit`, `shell_exec`, `grep_search`, `glob_search`, `list_dir`

## Config Override

```yaml
modes:
  code:
    model: "claude-sonnet-4-20250514"
    thinkBudget: "medium"
```
