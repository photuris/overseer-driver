package cli

import (
	"context"
	"io"
)

func runSpawn(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs, harness := harnessFlagSet("spawn")
	name := fs.String("name", "", "name to give the spawned agent")
	if err := fs.Parse(args); err != nil {
		return fail(stderr, "%v", err)
	}
	command := fs.Args()

	if *name == "" {
		return fail(stderr, "--name is required")
	}

	d, err := newDriver(*harness, "")
	if err != nil {
		return fail(stderr, "%v", err)
	}

	handle, err := d.Spawn(ctx, *name, command)

	return reportStart(stdout, stderr, handle, err)
}
