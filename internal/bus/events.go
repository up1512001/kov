// Package bus provides a typed, in-process event bus for loose coupling
// between kov subsystems. Components communicate via events rather than
// direct function calls, enabling open-source contributors to add features
// without modifying existing code.
package bus

// Event is the marker interface for all events in the system.
// Every event type must implement this interface.
type Event interface {
	eventMarker()
}

// --- Streaming Events ---

// TokenReceived is emitted when a new token arrives from the LLM stream.
// The TUI subscribes to this for real-time display.
type TokenReceived struct {
	SessionID string
	Token     string
	Provider  string
	Model     string
}

func (TokenReceived) eventMarker() {}

// StreamStarted signals the beginning of an LLM response stream.
type StreamStarted struct {
	SessionID string
	Provider  string
	Model     string
}

func (StreamStarted) eventMarker() {}

// StreamCompleted signals the end of an LLM response stream.
type StreamCompleted struct {
	SessionID    string
	InputTokens  int
	OutputTokens int
	Cost         float64
}

func (StreamCompleted) eventMarker() {}

// --- Task Events ---

// TaskStarted is emitted when execution of a sub-task begins.
type TaskStarted struct {
	SessionID string
	TaskID    string
	Seq       int
	Total     int
	Desc      string
}

func (TaskStarted) eventMarker() {}

// TaskCompleted is emitted when a sub-task finishes successfully.
type TaskCompleted struct {
	SessionID string
	TaskID    string
	Result    string
}

func (TaskCompleted) eventMarker() {}

// TaskFailed is emitted when a sub-task fails.
type TaskFailed struct {
	SessionID string
	TaskID    string
	Error     error
}

func (TaskFailed) eventMarker() {}

// --- Resilience Events ---

// FailureDetected is emitted when the resilience engine detects a failure.
type FailureDetected struct {
	SessionID string
	Error     error
	Action    string // retry, failover, pause, abort
}

func (FailureDetected) eventMarker() {}

// CheckpointSaved is emitted after a checkpoint is persisted to SQLite.
type CheckpointSaved struct {
	SessionID string
	TaskID    string
}

func (CheckpointSaved) eventMarker() {}

// ProviderFailover is emitted when the router switches to a fallback provider.
type ProviderFailover struct {
	SessionID string
	From      string
	To        string
}

func (ProviderFailover) eventMarker() {}

// LoopDetected is emitted when the loop detector finds a doom loop.
type LoopDetected struct {
	SessionID  string
	ToolName   string
	Repetitions int
}

func (LoopDetected) eventMarker() {}

// --- Cost Events ---

// CostUpdated tracks spending for budget enforcement.
type CostUpdated struct {
	SessionID  string
	Provider   string
	Model      string
	DeltaCost  float64
	TotalCost  float64
	BudgetUsed float64 // percentage (0-100)
}

func (CostUpdated) eventMarker() {}

// BudgetExceeded is emitted when a session exceeds its cost budget.
type BudgetExceeded struct {
	SessionID string
	Budget    float64
	Spent     float64
}

func (BudgetExceeded) eventMarker() {}

// --- Verification Events ---

// VerificationStarted is emitted when auto-verify begins.
type VerificationStarted struct {
	SessionID string
	Command   string
}

func (VerificationStarted) eventMarker() {}

// VerificationPassed is emitted when auto-verify succeeds.
type VerificationPassed struct {
	SessionID string
}

func (VerificationPassed) eventMarker() {}

// VerificationFailed is emitted when auto-verify fails.
type VerificationFailed struct {
	SessionID string
	Error     error
	Output    string
}

func (VerificationFailed) eventMarker() {}

// --- Session Events ---

// SessionStarted is emitted when a new session begins.
type SessionStarted struct {
	SessionID string
	Mode      string
	Provider  string
	Model     string
}

func (SessionStarted) eventMarker() {}

// SessionResumed is emitted when a session is recovered from a checkpoint.
type SessionResumed struct {
	SessionID string
	TaskID    string // the task being resumed
}

func (SessionResumed) eventMarker() {}

// SessionEnded is emitted when a session completes or is stopped.
type SessionEnded struct {
	SessionID  string
	TotalCost  float64
	TasksTotal int
	TasksDone  int
}

func (SessionEnded) eventMarker() {}

// --- Tool Events ---

// ToolCallStarted is emitted before a tool is executed.
type ToolCallStarted struct {
	SessionID string
	ToolName  string
}

func (ToolCallStarted) eventMarker() {}

// ToolCallCompleted is emitted after a tool finishes.
type ToolCallCompleted struct {
	SessionID string
	ToolName  string
	Duration  int64 // milliseconds
}

func (ToolCallCompleted) eventMarker() {}

// PermissionRequired is emitted when a tool needs user approval.
type PermissionRequired struct {
	SessionID   string
	ToolName    string
	Description string
	ResponseCh  chan bool // send true to approve, false to deny
}

func (PermissionRequired) eventMarker() {}
