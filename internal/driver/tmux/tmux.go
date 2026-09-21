// Package tmux implements the driver.Driver interface (and
// driver.Layouter) over the tmux terminal multiplexer. tmux has no
// concept of "agent" — only sessions, windows, panes, and the text
// inside them — so Status is a best-effort approximation backed by
// caller-supplied regex patterns, not a native lookup. See the
// overseer skill's resources/tmux.md for the reasoning.
//
// A Handle is always a full "session:window.pane" address, the same
// format Spawn and Split both return, so every pane is addressable
// the same way whether it's alone in its own session or one of
// several sharing a window.
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
	"github.com/photuris/overseer-driver/internal/textutil"
)

// paneFormat is the tmux format string every command that creates or
// lists a pane uses, so every Handle this package hands out has the
// same shape.
const paneFormat = "#{session_name}:#{window_index}.#{pane_index}"

// Patterns are the regexes Status matches an agent's recent output
// against. Blocked is checked before Idle. A zero-value Patterns
// (both nil) makes Status always report StatusUnknown with
// Confidence "none" — the honest answer when nothing has taught it
// what this tool's prompts look like.
type Patterns struct {
	Idle    []string `json:"idle"`
	Blocked []string `json:"blocked"`
}

// Driver drives tmux sessions, windows, and panes.
type Driver struct {
	patterns Patterns
}

// New returns a tmux Driver that matches Status against patterns.
func New(patterns Patterns) *Driver {
	return &Driver{patterns: patterns}
}

func (d *Driver) Spawn(ctx context.Context, name string, command []string) (driver.Handle, error) {
	args := []string{"new-session", "-d", "-s", name, "-P", "-F", paneFormat}
	if len(command) > 0 {
		args = append(args, textutil.ShellJoin(command))
	}
	out, err := run(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("tmux spawn %s: %w", name, err)
	}

	return driver.Handle(strings.TrimSpace(out)), nil
}

// Split implements driver.Layouter: it adds name/command as a new
// pane next to target, in the same window, instead of isolating it
// in its own session.
func (d *Driver) Split(ctx context.Context, target driver.Handle, direction driver.Direction, name string, command []string) (driver.Handle, error) {
	flag := "-h"
	if direction == driver.DirectionDown {
		flag = "-v"
	}

	args := []string{"split-window", "-t", string(target), flag, "-P", "-F", paneFormat}
	if len(command) > 0 {
		args = append(args, textutil.ShellJoin(command))
	}
	out, err := run(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("tmux split %s: %w", target, err)
	}
	newPane := driver.Handle(strings.TrimSpace(out))

	if name != "" {
		if _, err := run(ctx, "select-pane", "-t", string(newPane), "-T", name); err != nil {
			return "", fmt.Errorf("tmux split %s (title): %w", target, err)
		}
	}

	return newPane, nil
}

func (d *Driver) Read(ctx context.Context, target driver.Handle, lines int, ansi bool) (string, error) {
	args := []string{"capture-pane", "-t", string(target), "-p", "-S", fmt.Sprintf("-%d", lines)}
	if ansi {
		// -e keeps styling escape codes; -J preserves trailing spaces
		// and joins wrapped lines, which -e needs to render right.
		// Verified: the closing reset for a dim/^[[2m run can land at
		// the start of the *next* captured line rather than the end
		// of the styled one — a caller matching dim text should not
		// assume the reset appears on the same line.
		args = append(args, "-e", "-J")
	}
	out, err := run(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("tmux read %s: %w", target, err)
	}

	return textutil.TrimTrailingBlankLines(out), nil
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
	out, err := run(ctx, "list-panes", "-a", "-F", paneFormat)
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

// Rename sets target's pane title. tmux panes have no separately
// addressable name the way a session does, so this only changes a
// cosmetic label — the returned Handle is always target, unchanged.
func (d *Driver) Rename(ctx context.Context, target driver.Handle, label string) (driver.Handle, error) {
	if _, err := run(ctx, "select-pane", "-t", string(target), "-T", label); err != nil {
		return "", fmt.Errorf("tmux rename %s: %w", target, err)
	}

	return target, nil
}

// Interrupt, with kill, closes only target's own pane
// (tmux kill-pane), never the whole session — a session or window
// may hold sibling panes from other agents that must be left alone.
func (d *Driver) Interrupt(ctx context.Context, target driver.Handle, kill bool) error {
	if kill {
		if _, err := run(ctx, "kill-pane", "-t", string(target)); err != nil {
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
	tail, err := d.Read(ctx, target, 15, false)
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
