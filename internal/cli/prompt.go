package cli

import (
	"context"
	"io"

	"github.com/photuris/overseer-driver/internal/driver"
)

func runPrompt(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs, harness := harnessFlagSet("prompt")
	target := fs.String("target", "", "handle of the agent to prompt")
	text := fs.String("text", "", "text to send")
	if err := fs.Parse(args); err != nil {
		return fail(stderr, "%v", err)
	}

	if *target == "" {
		return fail(stderr, "--target is required")
	}
	if *text == "" {
		return fail(stderr, "--text is required")
	}

	d, err := newDriver(*harness, "")
	if err != nil {
		return fail(stderr, "%v", err)
	}

	if err := d.Prompt(ctx, driver.Handle(*target), *text); err != nil {
		return fail(stderr, "%v", err)
	}

	return writeJSON(stdout, map[string]any{"handle": *target, "sent": true})
}
