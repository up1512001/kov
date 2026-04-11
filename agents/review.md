# Review Mode

Reviews your staged git changes.

## Behavior

- Reads files and runs `git diff --staged`
- Reviews code for bugs, style, security, performance
- Cannot modify files (read-only + shell for git)
- Provides actionable feedback

## System Prompt Suffix

```
Mode: REVIEW — Code review. Run `git diff --staged` to see the changes,
then review them for bugs, security issues, performance problems, and style.
Provide specific, actionable feedback with file names and line numbers.
You CANNOT modify files.
```

## Tools Available

Read-only + shell: `file_read`, `grep_search`, `glob_search`, `list_dir`, `shell_exec`
