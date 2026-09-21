package cli

import (
	"context"
	"io"
)

func runList(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs, harness := harnessFlagSet("list")
	if err := fs.Parse(args); err != nil {
		return fail(stderr, "%v", err)
	}

	d, err := newDriver(*harness, "")
	if err != nil {
		return fail(stderr, "%v", err)
	}

	handles, err := d.List(ctx)
	if err != nil {
		return fail(stderr, "%v", err)
	}

	out := make([]string, len(handles))
	for i, h := range handles {
		out[i] = string(h)
	}

	return writeJSON(stdout, map[string]any{"handles": out})
}
