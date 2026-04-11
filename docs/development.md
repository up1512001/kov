# Development Guide

## Prerequisites

- Go 1.26+ ([install](https://go.dev/dl/))
- Git
- (Optional) [Task](https://taskfile.dev/) runner
- (Optional) [Air](https://github.com/cosmtrek/air) for hot reload

## Setup

```bash
git clone https://github.com/up1512001/kov.git
cd kov
go mod download
```

## Building

```bash
# Development build
go build -o bin/kov ./cmd/kov

# Production build (smaller binary)
go build -ldflags="-s -w" -o bin/kov ./cmd/kov

# Cross-compile
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/kov-linux ./cmd/kov
```

## Testing

```bash
# Run all tests with race detector
go test -race ./...

# Verbose output
go test -race -v ./...

# Single package
go test -v ./internal/tools/...

# With coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out  # View in browser

# Benchmarks
go test -bench=. -benchmem ./internal/bus/... ./internal/db/...
```

## Project Layout

```
internal/
├── agent/       # Core agent loop — the brain
├── app/         # CLI commands, app wiring
├── bus/         # Event bus (pub/sub)
├── config/      # Configuration loading
├── context/     # KOV.md, repo map, .kovignore
├── db/          # SQLite persistence
├── git/         # Git operations
├── mock/        # Mock LLM server for testing
├── modes/       # Mode resolver
├── pipe/        # Pipe mode (JSON stdin/stdout)
├── provider/    # LLM providers + router
├── resilience/  # State machine, checkpoints
├── session/     # Session management
├── tokens/      # Token counting + compaction
├── tools/       # Tool system
├── tui/         # Terminal UI
└── verify/      # Test runner auto-detection
```

## Adding a New Tool

1. Create the tool function in `internal/tools/`:

```go
func init() {
    // Register in DefaultRegistry
}

func myTool(ctx context.Context, args json.RawMessage) (string, error) {
    var params struct {
        Input string `json:"input"`
    }
    json.Unmarshal(args, &params)
    // ... implementation
    return result, nil
}
```

2. Register it in `internal/tools/registry.go` → `DefaultRegistry()`
3. Add the tool schema (name, description, input_schema)
4. Add tests in `internal/tools/`
5. The agent will auto-discover it via the registry

## Adding a New Provider

1. Implement `provider.Provider` interface in `internal/provider/`:

```go
type Provider interface {
    Name() string
    Chat(ctx context.Context, params ChatParams) (*ChatResponse, error)
    Stream(ctx context.Context, params ChatParams) (<-chan StreamEvent, error)
    Models() []string
    Close() error
}
```

2. Add to `internal/app/providers.go` → `buildProviders()`
3. Add config in `internal/config/schema.go`
4. Add tests

## Adding a New Mode

1. Add case in `internal/modes/modes.go` → `Resolve()`
2. Add system prompt in `internal/agent/agent.go` → `buildSystemPrompt()`
3. Add config struct in `internal/config/schema.go` if needed
4. Add tests

## Code Style

- `gofmt` + `go vet` (enforced by CI)
- `slog` for structured logging (not `log` or `fmt.Println`)
- Doc comments on all exported functions
- Table-driven tests with `t.Run()` sub-tests
- No global state — everything injected via constructors

## Git Workflow

```bash
# Feature branch
git checkout -b feat/my-feature

# Make changes, test
go test -race ./...

# Commit with conventional commits
git commit -m "feat: add new tool for X"
git commit -m "fix: handle edge case in Y"
git commit -m "test: add coverage for Z"

# Push and create PR
git push origin feat/my-feature
gh pr create
```

## Debugging

```bash
# Verbose mode
./bin/kov --verbose "debug this"

# Check database
sqlite3 ~/.local/share/kov/kov.db ".tables"
sqlite3 ~/.local/share/kov/kov.db "SELECT id, mode, state, prompt FROM sessions ORDER BY created_at DESC LIMIT 5;"

# Check config resolution
./bin/kov config
```
