# Contributing to Kov

We love contributions! Here's how to get started.

## Development Setup

```bash
# Clone the repo
git clone https://github.com/up1512001/kov.git
cd kov

# Install dependencies
go mod download

# Install Task runner (optional, for convenience)
go install github.com/go-task/task/v3/cmd/task@latest

# Build
go build -o bin/kov ./cmd/kov

# Run tests
go test -race ./...
```

## Project Structure

```
kov/
├── cmd/kov/          # CLI entry point
├── internal/
│   ├── agent/        # Core agent loop (plan→execute→verify)
│   ├── app/          # Application orchestrator + CLI commands
│   ├── bus/          # Typed event bus
│   ├── config/       # Configuration (Viper, yaml, env)
│   ├── context/      # KOV.md, .kovignore, repo map
│   ├── db/           # SQLite (WAL mode, crash-proof)
│   ├── git/          # Git integration (snapshot, commit, rollback)
│   ├── mock/         # Mock LLM server for tests
│   ├── modes/        # Mode resolver (code, plan, think, etc.)
│   ├── pipe/         # Pipe mode (stdin/stdout JSON)
│   ├── provider/     # LLM providers (Anthropic, OpenAI, Google, Ollama)
│   ├── resilience/   # State machine, checkpoints, loop detector
│   ├── session/      # Session management (list, resume, delete)
│   ├── tokens/       # Token counting + context compaction
│   ├── tools/        # Tool system (file ops, shell, search)
│   ├── tui/          # Terminal UI (Bubble Tea)
│   └── verify/       # Test runner auto-detection
└── docs/             # Documentation
```

## Making Changes

1. **Fork** the repo and create a feature branch
2. **Write tests** — we target >80% coverage
3. **Run the full suite**: `go test -race ./...`
4. **Keep commits atomic** — one logical change per commit
5. **Open a PR** with a clear description

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Use `slog` for structured logging
- Add doc comments for all exported functions
- Keep imports organized (stdlib, external, internal)
- No CGO dependencies — must cross-compile cleanly

## Testing

```bash
# Run all tests with race detector
go test -race ./...

# Run specific package
go test -v ./internal/tools/...

# Run with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Architecture Principles

1. **Crash-proof**: Every state transition is checkpointed. `kill -9` is a valid test.
2. **Zero CGO**: Pure Go only. Must cross-compile for macOS/Linux/Windows.
3. **Lazy init**: Sub-50ms cold start. Initialize subsystems on first use only.
4. **Event-driven**: Components communicate via the typed event bus, not direct calls.
5. **Provider-agnostic**: The agent loop doesn't know which LLM it's talking to.

## Provider Support

Adding a new provider:
1. Implement the `provider.Provider` interface in `internal/provider/`
2. Add the provider to `internal/app/providers.go`
3. Add config schema to `internal/config/schema.go`
4. Add tests

## Reporting Issues

- Use GitHub Issues
- Include: kov version, OS, Go version, provider
- Attach logs (run with `--verbose`)

## License

MIT — see [LICENSE](LICENSE)
