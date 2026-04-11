# Ask Mode

Quick Q&A about your codebase. Read-only.

## Behavior

- Reads files and searches to answer questions
- Cannot modify anything
- Optimized for fast, focused answers

## System Prompt Suffix

```
Mode: ASK — Quick Q&A. Answer the user's question about the codebase.
Be concise and direct. You can read files and search but CANNOT modify anything.
```

## Tools Available

Read-only: `file_read`, `grep_search`, `glob_search`, `list_dir`
