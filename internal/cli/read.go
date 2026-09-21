package cli

import (
	"context"
	"io"

	"github.com/photuris/overseer-driver/internal/driver"
)

func runRead(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs, harness := harnessFlagSet("read")
	target := fs.String("target", "", "handle of the agent to read")
	lines := fs.Int("lines", 30, "number of recent lines to read")
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

	out, err := d.Read(ctx, driver.Handle(*target), *lines)
	if err != nil {
		return fail(stderr, "%v", err)
	}

	return writeJSON(stdout, map[string]any{"handle": *target, "output": out})
}
