package cli

import (
	"context"
	"io"

	"github.com/photuris/overseer-driver/internal/driver"
)

func runSplit(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs, harness := harnessFlagSet("split")
	target := fs.String("target", "", "handle to split alongside")
	name := fs.String("name", "", "label for the new pane")
	direction := fs.String("direction", "right", `"right" or "down"`)
	if err := fs.Parse(args); err != nil {
		return fail(stderr, "%v", err)
	}
	command := fs.Args()

	if *target == "" {
		return fail(stderr, "--target is required")
	}

	d, err := newDriver(*harness, "")
	if err != nil {
		return fail(stderr, "%v", err)
	}

	layouter, ok := d.(driver.Layouter)
	if !ok {
		return fail(stderr, "harness %q has no native layout to split", *harness)
	}

	var dir driver.Direction
	switch *direction {
	case "right":
		dir = driver.DirectionRight
	case "down":
		dir = driver.DirectionDown
	default:
		return fail(stderr, `--direction must be "right" or "down"`)
	}

	handle, err := layouter.Split(ctx, driver.Handle(*target), dir, *name, command)
	if err != nil {
		return fail(stderr, "%v", err)
	}

	return writeJSON(stdout, map[string]any{"handle": string(handle)})
}
