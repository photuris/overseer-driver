// Package herdr implements the driver.Driver interface (and
// driver.Layouter) over Herdr, a terminal workspace manager with
// native agent-awareness. Unlike tmux, Herdr recognizes specific
// coding-agent CLIs running in a pane and reports their lifecycle
// state directly (Status here is always Confidence "native"), and
// its own addressing already accepts a pane ID as a target for both
// pane- and agent-level commands, so a Handle here is always a pane
// ID ("w1:p2") — stable across Rename, and valid whether or not the
// pane holds a Herdr-recognized agent.
//
// Herdr's agent start only knows how to launch one of its own fixed
// list of supported agent kinds (see knownKinds), not an arbitrary
// command — Spawn and Split use it when command[0] names one of
// those kinds, and fall back to the general-purpose pane run
// otherwise (a shell alias or wrapper around a known kind, for
// example). Herdr's own recognition of what's running in a pane
// looks to be screen-based, not tied to which of these two paths
// launched it: a real agent CLI started via the raw fallback has
// been observed getting picked up by Status/Prompt too, once Herdr's
// detection catches up to it, not just the known-kind path — this
// package does not force or wait for that detection, though, so
// don't assume it landed; check Status first. What the fallback
// reliably cannot do is start a Herdr-unrecognized program (a script,
// a plain shell) as something Status/Prompt can address as an agent.
package herdr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/photuris/overseer-driver/internal/driver"
	"github.com/photuris/overseer-driver/internal/textutil"
)

// knownKinds are the --kind values herdr agent start accepts, taken
// verbatim from `herdr agent start --help` on herdr 0.8.2. Anything
// else is launched as a raw pane process instead of a recognized
// agent.
var knownKinds = map[string]bool{
	"pi": true, "claude": true, "codex": true, "gemini": true, "cursor": true,
	"devin": true, "agy": true, "cline": true, "omp": true, "mastracode": true,
	"opencode": true, "copilot": true, "kimi": true, "kiro": true, "droid": true,
	"amp": true, "grok": true, "hermes": true, "kilo": true, "qodercli": true,
	"qwen": true, "maki": true,
}

// Driver drives Herdr workspaces, tabs, and panes.
type Driver struct{}

// New returns a Herdr Driver.
func New() *Driver {
	return &Driver{}
}

func (d *Driver) Spawn(ctx context.Context, name string, command []string) (driver.Handle, error) {
	if len(command) == 0 {
		return "", fmt.Errorf("herdr spawn %s: command is required", name)
	}

	workspace := os.Getenv("HERDR_WORKSPACE_ID")
	if workspace == "" {
		return "", fmt.Errorf("herdr spawn %s: HERDR_WORKSPACE_ID is not set; not running inside a Herdr pane", name)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("herdr spawn %s: %w", name, err)
	}

	var created struct {
		Result struct {
			RootPane struct {
				PaneID string `json:"pane_id"`
			} `json:"root_pane"`
		} `json:"result"`
	}
	if err := runJSON(ctx, &created, "tab", "create", "--workspace", workspace, "--cwd", cwd, "--label", name, "--no-focus"); err != nil {
		// No pane was created at all: no handle to return.
		return "", fmt.Errorf("herdr spawn %s: %w", name, err)
	}
	paneID := created.Result.RootPane.PaneID

	handle, err := d.startInPane(ctx, paneID, name, command)
	if err != nil {
		// The pane exists even though starting failed (e.g. agent
		// start timed out waiting for readiness) — return it so the
		// caller can inspect or clean it up. See driver.Driver.Spawn.
		return handle, fmt.Errorf("herdr spawn %s: %w", name, err)
	}

	return handle, nil
}

// Split implements driver.Layouter: it adds name/command as a new
// pane alongside target, in target's own tab, instead of opening a
// new tab the way Spawn does.
func (d *Driver) Split(ctx context.Context, target driver.Handle, direction driver.Direction, name string, command []string) (driver.Handle, error) {
	if len(command) == 0 {
		return "", fmt.Errorf("herdr split %s: command is required", target)
	}

	dir := "right"
	if direction == driver.DirectionDown {
		dir = "down"
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("herdr split %s: %w", target, err)
	}

	var split struct {
		Result struct {
			Pane struct {
				PaneID string `json:"pane_id"`
			} `json:"pane"`
		} `json:"result"`
	}
	if err := runJSON(ctx, &split, "pane", "split", "--pane", string(target), "--direction", dir, "--cwd", cwd, "--no-focus"); err != nil {
		// No pane was created at all: no handle to return.
		return "", fmt.Errorf("herdr split %s: %w", target, err)
	}
	paneID := split.Result.Pane.PaneID

	handle, err := d.startInPane(ctx, paneID, name, command)
	if err != nil {
		// The pane exists even though starting failed — return it so
		// the caller can inspect or clean it up. See driver.Driver.Spawn.
		return handle, fmt.Errorf("herdr split %s: %w", target, err)
	}

	return handle, nil
}

// startInPane launches command in the already-created pane paneID, as
// a recognized agent kind when command[0] is one, or as a raw process
// otherwise. It always returns paneID as the Handle, even on error —
// the pane itself exists regardless of whether the launch inside it
// finished; only the error return distinguishes "started" from "the
// pane exists but starting failed" (e.g. agent start timing out
// waiting for readiness, which folder-trust and other startup dialogs
// trigger). See driver.Driver.Spawn.
func (d *Driver) startInPane(ctx context.Context, paneID, name string, command []string) (driver.Handle, error) {
	handle := driver.Handle(paneID)

	if knownKinds[command[0]] {
		args := []string{"agent", "start", name, "--kind", command[0], "--pane", paneID}
		if len(command) > 1 {
			args = append(args, "--")
			args = append(args, command[1:]...)
		}
		if _, err := run(ctx, args...); err != nil {
			return handle, err
		}

		return handle, nil
	}

	if _, err := run(ctx, "pane", "run", paneID, textutil.ShellJoin(command)); err != nil {
		return handle, err
	}
	if _, err := run(ctx, "pane", "rename", paneID, name); err != nil {
		return handle, fmt.Errorf("(label): %w", err)
	}

	return handle, nil
}

func (d *Driver) Read(ctx context.Context, target driver.Handle, lines int, ansi bool) (string, error) {
	// --source visible is required here: on herdr 0.8.2, --lines
	// combined with the default "recent" source (or
	// "recent-unwrapped") silently returns empty output instead of
	// an error — verified directly against a live pane. Only
	// --source visible actually honors --lines.
	args := []string{"pane", "read", string(target), "--source", "visible", "--lines", fmt.Sprintf("%d", lines)}
	if ansi {
		args = append(args, "--format", "ansi")
	}
	out, err := run(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("herdr read %s: %w", target, err)
	}

	return textutil.TrimTrailingBlankLines(out), nil
}

// Prompt submits text to the agent recognized in target's pane. It
// only works once Herdr recognizes an agent there — reliably true
// right after the knownKinds path in Spawn/Split, and observed true
// for a real agent CLI started via the raw fallback too (Herdr's
// detection looks screen-based, not tied to launch path) — but never
// true for a raw fallback pane running something Herdr doesn't
// recognize at all. Check Status first if unsure.
func (d *Driver) Prompt(ctx context.Context, target driver.Handle, text string) error {
	if _, err := run(ctx, "agent", "prompt", string(target), text); err != nil {
		return fmt.Errorf("herdr prompt %s: %w", target, err)
	}

	return nil
}

func (d *Driver) List(ctx context.Context) ([]driver.Handle, error) {
	var listed struct {
		Result struct {
			Agents []struct {
				PaneID string `json:"pane_id"`
			} `json:"agents"`
		} `json:"result"`
	}
	if err := runJSON(ctx, &listed, "agent", "list"); err != nil {
		return nil, fmt.Errorf("herdr list: %w", err)
	}

	handles := make([]driver.Handle, len(listed.Result.Agents))
	for i, a := range listed.Result.Agents {
		handles[i] = driver.Handle(a.PaneID)
	}

	return handles, nil
}

// Rename sets target's display label. A pane ID is Herdr's stable
// public identifier and never changes, so the returned Handle is
// always target, unchanged.
func (d *Driver) Rename(ctx context.Context, target driver.Handle, label string) (driver.Handle, error) {
	if _, err := run(ctx, "pane", "rename", string(target), label); err != nil {
		return "", fmt.Errorf("herdr rename %s: %w", target, err)
	}

	return target, nil
}

// Interrupt, with kill, closes target's own pane (pane close) — it
// never touches its tab, which may hold sibling panes from other
// agents. Without kill, it sends ctrl+c to the recognized agent.
func (d *Driver) Interrupt(ctx context.Context, target driver.Handle, kill bool) error {
	if kill {
		if _, err := run(ctx, "pane", "close", string(target)); err != nil {
			return fmt.Errorf("herdr kill %s: %w", target, err)
		}

		return nil
	}

	if _, err := run(ctx, "agent", "send-keys", string(target), "ctrl+c"); err != nil {
		return fmt.Errorf("herdr interrupt %s: %w", target, err)
	}

	return nil
}

// Status reads target's lifecycle state directly from Herdr — always
// Confidence "native". Herdr's own done and idle states both mean
// "ready for input" (done differs only in whether the user has seen
// it in the UI yet), so both map to StatusIdle here; that UI-facing
// distinction has no meaning for an overseer polling automatically.
// Tail is left empty: the point of a native status is not needing to
// double-check it against raw output.
func (d *Driver) Status(ctx context.Context, target driver.Handle) (driver.StatusResult, error) {
	var got struct {
		Result struct {
			Pane struct {
				AgentStatus string `json:"agent_status"`
			} `json:"pane"`
		} `json:"result"`
	}
	if err := runJSON(ctx, &got, "pane", "get", string(target)); err != nil {
		return driver.StatusResult{}, fmt.Errorf("herdr status %s: %w", target, err)
	}

	return driver.StatusResult{Status: mapStatus(got.Result.Pane.AgentStatus), Confidence: "native"}, nil
}

func mapStatus(s string) driver.Status {
	switch s {
	case "working":
		return driver.StatusWorking
	case "idle", "done":
		return driver.StatusIdle
	case "blocked":
		return driver.StatusBlocked
	default:
		return driver.StatusUnknown
	}
}

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "herdr", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var envelope errorEnvelope
		if jsonErr := json.Unmarshal(stderr.Bytes(), &envelope); jsonErr == nil && envelope.Error.Message != "" {
			return "", fmt.Errorf("%s: %s", envelope.Error.Code, envelope.Error.Message)
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}

		return "", fmt.Errorf("%s", msg)
	}

	return stdout.String(), nil
}

func runJSON(ctx context.Context, v any, args ...string) error {
	out, err := run(ctx, args...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(out), v); err != nil {
		return fmt.Errorf("parsing herdr output: %w", err)
	}

	return nil
}
