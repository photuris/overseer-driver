// Package driver defines the abstract interface every harness backend
// implements. It mirrors the primitive vocabulary in the overseer
// skill's SKILL.md: spawn, status, read, prompt, list, rename,
// interrupt. notify lives outside this interface (internal/notify)
// because it is harness-independent.
package driver

import "context"

// Status is an agent's coarse execution state.
type Status string

const (
	StatusWorking Status = "working"
	StatusIdle    Status = "idle"
	StatusBlocked Status = "blocked"
	StatusUnknown Status = "unknown"
)

// Handle identifies one spawned agent within a harness. Its exact
// shape is driver-defined (a tmux session name, a Herdr pane id).
type Handle string

// StatusResult reports Status plus how much to trust it. A driver
// with no native agent-awareness sets Confidence to "heuristic" or
// "none" and includes Tail so the caller can judge for itself.
type StatusResult struct {
	Status     Status `json:"status"`
	Confidence string `json:"confidence"`
	Tail       string `json:"tail,omitempty"`
}

// Driver launches and controls agents on one multi-agent harness.
type Driver interface {
	// Spawn launches an agent named name running command and returns
	// its handle.
	Spawn(ctx context.Context, name string, command []string) (Handle, error)

	// Read returns the agent's last lines of output, most recent
	// last, with trailing blank lines trimmed.
	Read(ctx context.Context, target Handle, lines int) (string, error)

	// Prompt sends text to the agent, followed by a submit action
	// (e.g. Enter). It does not wait for the agent to go idle;
	// callers poll Read or Status for that.
	Prompt(ctx context.Context, target Handle, text string) error

	// List returns the handles of all currently active agents on
	// this harness, not just ones this process spawned.
	List(ctx context.Context) ([]Handle, error)

	// Rename relabels an agent and returns its new handle. Some
	// harnesses change the handle's identity on rename; callers must
	// use the returned handle afterward.
	Rename(ctx context.Context, target Handle, label string) (Handle, error)

	// Interrupt asks the agent to stop. If kill is true, it forces a
	// hard stop instead of a graceful interrupt signal.
	Interrupt(ctx context.Context, target Handle, kill bool) error

	// Status reports the agent's execution state. Drivers without
	// native agent-awareness approximate it; see StatusResult.
	Status(ctx context.Context, target Handle) (StatusResult, error)
}
