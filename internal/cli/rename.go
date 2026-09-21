package cli

import (
	"context"
	"io"

	"github.com/photuris/overseer-driver/internal/driver"
)

func runRename(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs, harness := harnessFlagSet("rename")
	target := fs.String("target", "", "handle of the agent to rename")
	label := fs.String("label", "", "new label")
	if err := fs.Parse(args); err != nil {
		return fail(stderr, "%v", err)
	}

	if *target == "" {
		return fail(stderr, "--target is required")
	}
	if *label == "" {
		return fail(stderr, "--label is required")
	}

	d, err := newDriver(*harness, "")
	if err != nil {
		return fail(stderr, "%v", err)
	}

	newHandle, err := d.Rename(ctx, driver.Handle(*target), *label)
	if err != nil {
		return fail(stderr, "%v", err)
	}

	return writeJSON(stdout, map[string]any{"handle": string(newHandle)})
}
