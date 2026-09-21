// Package notify surfaces a message outside the current session via
// whatever OS-level notifier is available. It is harness-independent:
// no multi-agent tool primitive maps to it, since notify is about
// reaching the user, not an agent.
package notify

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
)

// Result reports how (or whether) a notification was sent.
type Result struct {
	Sent   bool   `json:"sent"`
	Method string `json:"method"`
}

// Send attempts to show title/body as a desktop notification. It
// never returns an error for "no notifier available" — that is a
// normal outcome the caller must handle by falling back to telling
// the user directly, not a failure to report.
func Send(ctx context.Context, title, body string) (Result, error) {
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
