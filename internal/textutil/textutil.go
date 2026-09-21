// Package textutil holds small text-processing helpers shared by
// driver backends that shell out to a terminal tool: trimming padded
// terminal captures and safely quoting a command for a shell that
// will interpret it as one line.
package textutil

import (
	"regexp"
	"strings"
)

// TrimTrailingBlankLines removes trailing blank lines a terminal
// capture pads output with when the pane/window is taller than its
// content.
func TrimTrailingBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	end := len(lines)
	for end > 0 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}

	return strings.Join(lines[:end], "\n")
}

// ShellJoin renders command as a single POSIX shell command line, for
// tools that run a trailing argument through a shell as one string
// rather than as a raw argv (tmux's new-session, Herdr's pane run).
func ShellJoin(command []string) string {
	quoted := make([]string, len(command))
	for i, arg := range command {
		quoted[i] = shellQuote(arg)
	}

	return strings.Join(quoted, " ")
}

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_./=-]+$`)

func shellQuote(s string) string {
	if s != "" && shellSafe.MatchString(s) {
		return s
	}

	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
