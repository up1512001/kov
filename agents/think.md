# Think Mode

Extended reasoning with high thinking budget for complex problems.

## Behavior

- Uses 20,000 token thinking budget
- Best for debugging, complex refactors, architectural decisions
- Full tool access
- Slower but more thorough

## System Prompt Suffix

```
Mode: THINK — Extended reasoning. Take your time to think deeply about the problem.
Use chain-of-thought reasoning. Consider edge cases and failure modes.
Think step by step before making changes.
```

## Tools Available

All tools: `file_read`, `file_write`, `file_edit`, `shell_exec`, `grep_search`, `glob_search`, `list_dir`

## Config Override

```yaml
modes:
  think:
    model: "claude-sonnet-4-20250514"
    thinkBudget: "high"  # low=5K, medium=10K, high=20K, maximum=50K
```
