// Package tmux implements the driver.Driver interface over the tmux
// terminal multiplexer. tmux has no concept of "agent" — only
// sessions and the text inside them — so Status is a best-effort
// approximation backed by caller-supplied regex patterns, not a
// native lookup. See the overseer skill's resources/tmux.md for the
// reasoning.
//
// Each Spawn creates its own tmux session; a Handle is that session's
// name. tmux's own window/pane splitting (multiple agents sharing one
// window) is a layout concern this package does not implement.
package tmux

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/photuris/overseer-driver/internal/driver"
)

// Patterns are the regexes Status matches an agent's recent output
// against. Blocked is checked before Idle. A zero-value Patterns
// (both nil) makes Status always report StatusUnknown with
// Confidence "none" — the honest answer when nothing has taught it
// what this tool's prompts look like.
type Patterns struct {
	Idle    []string `json:"idle"`
	Blocked []string `json:"blocked"`
}

// Driver drives tmux sessions.
type Driver struct {
	patterns Patterns
}

// New returns a tmux Driver that matches Status against patterns.
func New(patterns Patterns) *Driver {
	return &Driver{patterns: patterns}
}

func (d *Driver) Spawn(ctx context.Context, name string, command []string) (driver.Handle, error) {
	args := []string{"new-session", "-d", "-s", name}
	if len(command) > 0 {
		args = append(args, shellJoin(command))
	}
	if _, err := run(ctx, args...); err != nil {
		return "", fmt.Errorf("tmux spawn %s: %w", name, err)
	}

	return driver.Handle(name), nil
}

func (d *Driver) Read(ctx context.Context, target driver.Handle, lines int) (string, error) {
	out, err := run(ctx, "capture-pane", "-t", string(target), "-p", "-S", fmt.Sprintf("-%d", lines))
	if err != nil {
		return "", fmt.Errorf("tmux read %s: %w", target, err)
	}

	return trimTrailingBlankLines(out), nil
}

func (d *Driver) Prompt(ctx context.Context, target driver.Handle, text string) error {
	if _, err := run(ctx, "send-keys", "-t", string(target), "-l", text); err != nil {
		return fmt.Errorf("tmux prompt %s: %w", target, err)
	}
	if _, err := run(ctx, "send-keys", "-t", string(target), "Enter"); err != nil {
		return fmt.Errorf("tmux prompt %s (submit): %w", target, err)
	}

	return nil
}

func (d *Driver) List(ctx context.Context) ([]driver.Handle, error) {
	out, err := run(ctx, "list-sessions", "-F", "#{session_name}")
	if err != nil {
		if strings.Contains(err.Error(), "no server running") {
			return nil, nil
		}

		return nil, fmt.Errorf("tmux list: %w", err)
	}

	var handles []driver.Handle
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line != "" {
			handles = append(handles, driver.Handle(line))
		}
	}

	return handles, nil
}

func (d *Driver) Rename(ctx context.Context, target driver.Handle, label string) (driver.Handle, error) {
	if _, err := run(ctx, "rename-session", "-t", string(target), label); err != nil {
		return "", fmt.Errorf("tmux rename %s: %w", target, err)
	}

	return driver.Handle(label), nil
}

func (d *Driver) Interrupt(ctx context.Context, target driver.Handle, kill bool) error {
	if kill {
		if _, err := run(ctx, "kill-session", "-t", string(target)); err != nil {
			return fmt.Errorf("tmux kill %s: %w", target, err)
		}

		return nil
	}

	if _, err := run(ctx, "send-keys", "-t", string(target), "C-c"); err != nil {
		return fmt.Errorf("tmux interrupt %s: %w", target, err)
	}

	return nil
}

func (d *Driver) Status(ctx context.Context, target driver.Handle) (driver.StatusResult, error) {
	tail, err := d.Read(ctx, target, 15)
	if err != nil {
		return driver.StatusResult{}, err
	}

	if len(d.patterns.Idle) == 0 && len(d.patterns.Blocked) == 0 {
		return driver.StatusResult{Status: driver.StatusUnknown, Confidence: "none", Tail: tail}, nil
	}

	if matchAny(d.patterns.Blocked, tail) {
		return driver.StatusResult{Status: driver.StatusBlocked, Confidence: "heuristic", Tail: tail}, nil
	}
	if matchAny(d.patterns.Idle, tail) {
		return driver.StatusResult{Status: driver.StatusIdle, Confidence: "heuristic", Tail: tail}, nil
	}

	return driver.StatusResult{Status: driver.StatusWorking, Confidence: "heuristic", Tail: tail}, nil
}

func matchAny(patterns []string, text string) bool {
	for _, p := range patterns {
		if matched, _ := regexp.MatchString(p, text); matched {
			return true
		}
	}

	return false
}

func run(ctx context.Context, args ...string) (string, error) {
	out, err := runOnce(ctx, args...)
	if err != nil && strings.Contains(err.Error(), "server exited unexpectedly") {
		// The tmux server exits when its last session closes; a
		// command issued just after that can race the dying
		// server's socket cleanup. One retry after a brief pause
		// reliably clears it (verified against a real tmux server).
		time.Sleep(200 * time.Millisecond)

		return runOnce(ctx, args...)
	}

	return out, err
}

func runOnce(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "tmux", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}

		return "", fmt.Errorf("%s", msg)
	}

	return stdout.String(), nil
}

// trimTrailingBlankLines removes the blank lines tmux pads
// capture-pane output with when the pane is taller than its content.
func trimTrailingBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	end := len(lines)
	for end > 0 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}

	return strings.Join(lines[:end], "\n")
}

// shellJoin renders command as a single POSIX shell command line,
// since tmux new-session's trailing argument is run through a shell
// as one string, not as a raw argv.
func shellJoin(command []string) string {
	quoted := make([]string, len(command))
	for i, arg := range command {
		quoted[i] = shellQuote(arg)
	}

	return strings.Join(quoted, " ")
}

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_./=-]+$`)

func shellQuote(s string) string {
	if s != "" && shellSafe.MatchString(s) {
		return s
	}

	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
