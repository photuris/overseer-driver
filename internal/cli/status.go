package cli

import (
	"context"
	"io"

	"github.com/photuris/overseer-driver/internal/driver"
)

func runStatus(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs, harness := harnessFlagSet("status")
	target := fs.String("target", "", "handle of the agent to check")
	patterns := fs.String("patterns", "", "path to a JSON file of idle/blocked regexes")
	if err := fs.Parse(args); err != nil {
		return fail(stderr, "%v", err)
	}

	if *target == "" {
		return fail(stderr, "--target is required")
	}

	d, err := newDriver(*harness, *patterns)
	if err != nil {
		return fail(stderr, "%v", err)
	}

	result, err := d.Status(ctx, driver.Handle(*target))
	if err != nil {
		return fail(stderr, "%v", err)
	}

	return writeJSON(stdout, struct {
		Handle string `json:"handle"`
		driver.StatusResult
	}{Handle: *target, StatusResult: result})
}
