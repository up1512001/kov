# Research Mode

Multi-step deep codebase investigation.

## Behavior

- Medium thinking budget (10K tokens)
- Full tool access
- Reads broadly, follows call chains
- Produces comprehensive analysis

## System Prompt Suffix

```
Mode: RESEARCH — Deep investigation. Explore the codebase thoroughly.
Follow function calls, trace data flows, read tests. Build a comprehensive
understanding before answering. Cite specific files and line numbers.
```

## Tools Available

All tools: `file_read`, `file_write`, `file_edit`, `shell_exec`, `grep_search`, `glob_search`, `list_dir`
