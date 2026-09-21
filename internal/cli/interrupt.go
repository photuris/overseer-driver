package cli

import (
	"context"
	"io"

	"github.com/photuris/overseer-driver/internal/driver"
)

func runInterrupt(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs, harness := harnessFlagSet("interrupt")
	target := fs.String("target", "", "handle of the agent to interrupt")
	kill := fs.Bool("kill", false, "force a hard stop instead of a graceful interrupt")
	if err := fs.Parse(args); err != nil {
		return fail(stderr, "%v", err)
	}

	if *target == "" {
		return fail(stderr, "--target is required")
	}

	d, err := newDriver(*harness, "")
	if err != nil {
		return fail(stderr, "%v", err)
	}

	if err := d.Interrupt(ctx, driver.Handle(*target), *kill); err != nil {
		return fail(stderr, "%v", err)
	}

	return writeJSON(stdout, map[string]any{"handle": *target, "killed": *kill})
}
