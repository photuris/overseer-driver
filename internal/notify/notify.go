// Package notify surfaces a message outside the current session via
// whatever notifier is available: Herdr's own, when running inside
// it, or an OS-level one otherwise. It is harness-independent in the
// driver.Driver sense — no multi-agent primitive maps to it, since
// notify is about reaching the user, not an agent — but it does
// prefer Herdr's richer, in-context notification when available.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// Result reports how (or whether) a notification was sent.
type Result struct {
	Sent   bool   `json:"sent"`
	Method string `json:"method"`
	Reason string `json:"reason,omitempty"`
}

// Send attempts to show title/body as a notification, trying Herdr
// first when running inside it, then an OS-level notifier. sound is
// Herdr-specific ("none", "done", or "request"); empty leaves it to
// Herdr's own default, and OS-level notifiers ignore it. It never
// returns an error for "nothing available or nothing shown" — that
// is a normal outcome the caller must handle by falling back to
// telling the user directly, not a failure to report.
func Send(ctx context.Context, title, body, sound string) (Result, error) {
	if os.Getenv("HERDR_ENV") == "1" {
		if result, ok, err := sendHerdr(ctx, title, body, sound); ok {
			return result, err
		}
	}

	switch runtime.GOOS {
	case "linux":
		if path, err := exec.LookPath("notify-send"); err == nil {
			if err := exec.CommandContext(ctx, path, title, body).Run(); err != nil {
				return Result{}, fmt.Errorf("notify-send: %w", err)
			}

			return Result{Sent: true, Method: "notify-send"}, nil
		}
	case "darwin":
		if path, err := exec.LookPath("osascript"); err == nil {
			script := fmt.Sprintf("display notification %q with title %q", body, title)
			if err := exec.CommandContext(ctx, path, "-e", script).Run(); err != nil {
				return Result{}, fmt.Errorf("osascript: %w", err)
			}

			return Result{Sent: true, Method: "osascript"}, nil
		}
	}

	return Result{Sent: false, Method: "none"}, nil
}

// sendHerdr tries `herdr notification show`. Its second return value
// is false when herdr isn't on PATH at all, so the caller falls
// through to an OS-level notifier instead of reporting a failure.
func sendHerdr(ctx context.Context, title, body, sound string) (Result, bool, error) {
	path, err := exec.LookPath("herdr")
	if err != nil {
		return Result{}, false, nil
	}

	args := []string{"notification", "show", title, "--body", body}
	if sound != "" {
		args = append(args, "--sound", sound)
	}
	cmd := exec.CommandContext(ctx, path, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if msg == "" {
			msg = err.Error()
		}

		return Result{}, true, fmt.Errorf("herdr notification show: %s", msg)
	}

	// The command can succeed while showing nothing at all — Herdr
	// respects the user's own notification settings (e.g. disabled),
	// verified directly: {"result":{"shown":false,"reason":"disabled"}}.
	var shown struct {
		Result struct {
			Shown  bool   `json:"shown"`
			Reason string `json:"reason"`
		} `json:"result"`
	}
	if jsonErr := json.Unmarshal(stdout.Bytes(), &shown); jsonErr != nil {
		return Result{}, true, fmt.Errorf("parsing herdr notification output: %w", jsonErr)
	}

	return Result{Sent: shown.Result.Shown, Method: "herdr", Reason: shown.Result.Reason}, true, nil
}
