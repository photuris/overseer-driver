// Command overseer-driver is a cross-platform CLI implementing the
// overseer skill's harness-driver interface (spawn, read, prompt,
// list, rename, interrupt, status, notify) as tested code instead of
// prose an LLM re-derives on every call. See the overseer skill's
// SKILL.md and resources/ for the interface these subcommands back.
package main

import (
	"context"
	"os"

	"github.com/photuris/overseer-driver/internal/cli"
)

func main() {
	os.Exit(cli.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}
