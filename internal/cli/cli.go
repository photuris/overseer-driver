// Package cli wires the overseer-driver subcommands to a
// driver.Driver. Every subcommand prints one JSON object to stdout on
// success; failures go to stderr with a nonzero exit code — never a
// JSON error envelope, so a caller only has to parse stdout when the
// exit code says to.
package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/photuris/overseer-driver/internal/driver"
	"github.com/photuris/overseer-driver/internal/driver/herdr"
	"github.com/photuris/overseer-driver/internal/driver/tmux"
)

// commands maps each subcommand name to its handler.
var commands = map[string]func(ctx context.Context, args []string, stdout, stderr io.Writer) int{
	"spawn":     runSpawn,
	"split":     runSplit,
	"read":      runRead,
	"prompt":    runPrompt,
	"list":      runList,
	"rename":    runRename,
	"interrupt": runInterrupt,
	"status":    runStatus,
	"notify":    runNotify,
}

// Run dispatches args[0] to its subcommand handler and returns the
// process exit code.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "overseer-driver: missing subcommand")

		return 2
	}

	handler, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(stderr, "overseer-driver: unknown subcommand %q\n", args[0])

		return 2
	}

	return handler(ctx, args[1:], stdout, stderr)
}

// harnessFlagSet returns a FlagSet with the --harness flag every
// driver-backed subcommand shares, plus a pointer to its value.
func harnessFlagSet(name string) (*flag.FlagSet, *string) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)

	return fs, fs.String("harness", "", "harness driver to use (tmux, herdr)")
}

// newDriver resolves --harness to a concrete driver.Driver.
// patternsPath is only meaningful for drivers that approximate
// status; unsupported by a driver, it is silently ignored.
func newDriver(harness, patternsPath string) (driver.Driver, error) {
	switch harness {
	case "tmux":
		patterns, err := loadTmuxPatterns(patternsPath)
		if err != nil {
			return nil, err
		}

		return tmux.New(patterns), nil
	case "herdr":
		return herdr.New(), nil
	case "":
		return nil, fmt.Errorf("--harness is required")
	default:
		return nil, fmt.Errorf("no driver for harness %q", harness)
	}
}

func loadTmuxPatterns(path string) (tmux.Patterns, error) {
	if path == "" {
		return tmux.Patterns{}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return tmux.Patterns{}, fmt.Errorf("reading --patterns: %w", err)
	}

	var patterns tmux.Patterns
	if err := json.Unmarshal(data, &patterns); err != nil {
		return tmux.Patterns{}, fmt.Errorf("parsing --patterns: %w", err)
	}

	return patterns, nil
}

func writeJSON(w io.Writer, v any) int {
	enc := json.NewEncoder(w)
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(w, "overseer-driver: encoding output: %v\n", err)

		return 1
	}

	return 0
}

func fail(stderr io.Writer, format string, args ...any) int {
	fmt.Fprintf(stderr, "overseer-driver: "+format+"\n", args...)

	return 1
}
