package cli

import (
	"context"
	"flag"
	"io"

	"github.com/photuris/overseer-driver/internal/notify"
)

func runNotify(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("notify", flag.ContinueOnError)
	title := fs.String("title", "Overseer", "notification title")
	message := fs.String("message", "", "notification body")
	if err := fs.Parse(args); err != nil {
		return fail(stderr, "%v", err)
	}

	if *message == "" {
		return fail(stderr, "--message is required")
	}

	result, err := notify.Send(ctx, *title, *message)
	if err != nil {
		return fail(stderr, "%v", err)
	}

	return writeJSON(stdout, result)
}
