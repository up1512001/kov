# Architect Mode

Dual-model orchestration: plan with a strong model, execute with a fast model.

## Behavior

1. **Phase 1 — Plan**: Uses the planning model (e.g., Claude Opus) to analyze the codebase and produce a detailed implementation plan
2. **Phase 2 — Execute**: Uses the editing model (e.g., Claude Sonnet) to implement the plan step-by-step

## System Prompt Suffix

### Planning Phase
```
Mode: ARCHITECT (Planning Phase) — You are the planning model.
Analyze the codebase and produce a detailed, step-by-step implementation plan.
List every file to change, what to change, and why. Be extremely thorough.
Do NOT make any changes yourself.
```

### Execution Phase
```
Mode: ARCHITECT (Execution Phase) — You are the execution model.
Follow the provided plan exactly. Implement each step one at a time.
Do not deviate from the plan unless you find a clear error.
```

## Tools Available

All tools in both phases.

## Config Override

```yaml
modes:
  architect:
    planModel: "claude-opus-4-20250514"
    editModel: "claude-sonnet-4-20250514"
```
