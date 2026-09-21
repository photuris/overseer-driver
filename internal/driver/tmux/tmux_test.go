package tmux

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/photuris/overseer-driver/internal/driver"
)

func requireTmux(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
}

// sessionName returns a name unlikely to collide with a real session.
func sessionName(t *testing.T) string {
	t.Helper()

	return fmt.Sprintf("overseer-driver-test-%d", time.Now().UnixNano())
}

func TestSpawnReadPromptRoundTrip(t *testing.T) {
	requireTmux(t)

	ctx := context.Background()
	d := New(Patterns{})
	name := sessionName(t)

	handle, err := d.Spawn(ctx, name, []string{"bash", "--noprofile", "--norc"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	t.Cleanup(func() {
		_ = d.Interrupt(ctx, handle, true)
	})

	if !strings.HasPrefix(string(handle), name+":") {
		t.Errorf("Spawn() handle = %q, want it to start with %q", handle, name+":")
	}

	if err := d.Prompt(ctx, handle, `echo hello world`); err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	out, err := d.Read(ctx, handle, 10, false)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !strings.Contains(out, "hello world") {
		t.Errorf("Read output %q does not contain the echoed text", out)
	}
	if strings.HasSuffix(out, "\n") || strings.HasSuffix(out, "\n\n") {
		t.Errorf("Read output %q has untrimmed trailing blank lines", out)
	}
}

func TestReadAnsiPreservesEscapeCodes(t *testing.T) {
	requireTmux(t)

	ctx := context.Background()
	d := New(Patterns{})
	name := sessionName(t)

	handle, err := d.Spawn(ctx, name, []string{"bash", "--noprofile", "--norc"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	t.Cleanup(func() {
		_ = d.Interrupt(ctx, handle, true)
	})

	if err := d.Prompt(ctx, handle, `printf '\033[2mdim\033[0m\n'`); err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	plain, err := d.Read(ctx, handle, 10, false)
	if err != nil {
		t.Fatalf("Read(ansi=false): %v", err)
	}
	if strings.Contains(plain, "\x1b[") {
		t.Errorf("Read(ansi=false) = %q, want escape codes stripped", plain)
	}

	styled, err := d.Read(ctx, handle, 10, true)
	if err != nil {
		t.Fatalf("Read(ansi=true): %v", err)
	}
	if !strings.Contains(styled, "\x1b[2m") {
		t.Errorf("Read(ansi=true) = %q, want it to contain the dim escape code \\x1b[2m", styled)
	}
}

func TestList(t *testing.T) {
	requireTmux(t)

	ctx := context.Background()
	d := New(Patterns{})
	name := sessionName(t)

	handle, err := d.Spawn(ctx, name, []string{"bash", "--noprofile", "--norc"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	t.Cleanup(func() {
		_ = d.Interrupt(ctx, handle, true)
	})

	handles, err := d.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	found := false
	for _, h := range handles {
		if h == handle {
			found = true
		}
	}
	if !found {
		t.Errorf("List() = %v, want it to contain %q", handles, handle)
	}
}

func TestSplitSharesWindowAndSurvivesSiblingKill(t *testing.T) {
	requireTmux(t)

	ctx := context.Background()
	d := New(Patterns{})
	name := sessionName(t)

	first, err := d.Spawn(ctx, name, []string{"bash", "--noprofile", "--norc"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	t.Cleanup(func() {
		_ = d.Interrupt(ctx, first, true)
	})

	second, err := d.Split(ctx, first, driver.DirectionRight, "second", []string{"bash", "--noprofile", "--norc"})
	if err != nil {
		t.Fatalf("Split: %v", err)
	}

	firstSession := strings.SplitN(string(first), ":", 2)[0]
	secondSession := strings.SplitN(string(second), ":", 2)[0]
	if firstSession != secondSession {
		t.Errorf("Split() handle %q is not in the same session as %q", second, first)
	}
	if second == first {
		t.Errorf("Split() returned the same handle as its target: %q", second)
	}

	handles, err := d.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(handles) < 2 {
		t.Errorf("List() = %v, want at least 2 panes after Split", handles)
	}

	// Killing the second pane must not take the first one with it.
	if err := d.Interrupt(ctx, second, true); err != nil {
		t.Fatalf("Interrupt(second, kill=true): %v", err)
	}

	handles, err = d.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	foundFirst, foundSecond := false, false
	for _, h := range handles {
		if h == first {
			foundFirst = true
		}
		if h == second {
			foundSecond = true
		}
	}
	if !foundFirst {
		t.Errorf("List() = %v, want the sibling pane %q to survive", handles, first)
	}
	if foundSecond {
		t.Errorf("List() = %v, want the killed pane %q to be gone", handles, second)
	}
}

func TestRenameSetsTitleWithoutChangingHandle(t *testing.T) {
	requireTmux(t)

	ctx := context.Background()
	d := New(Patterns{})
	name := sessionName(t)

	handle, err := d.Spawn(ctx, name, []string{"bash", "--noprofile", "--norc"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	t.Cleanup(func() {
		_ = d.Interrupt(ctx, handle, true)
	})

	newHandle, err := d.Rename(ctx, handle, "renamed-title")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if newHandle != handle {
		t.Errorf("Rename() = %q, want the unchanged handle %q", newHandle, handle)
	}

	handles, err := d.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, h := range handles {
		if h == handle {
			found = true
		}
	}
	if !found {
		t.Errorf("List() = %v, want %q still addressable after rename", handles, handle)
	}
}

func TestInterruptKill(t *testing.T) {
	requireTmux(t)

	ctx := context.Background()
	d := New(Patterns{})
	name := sessionName(t)

	handle, err := d.Spawn(ctx, name, []string{"bash", "--noprofile", "--norc"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	if err := d.Interrupt(ctx, handle, true); err != nil {
		t.Fatalf("Interrupt(kill=true): %v", err)
	}

	handles, err := d.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, h := range handles {
		if h == handle {
			t.Errorf("List() still contains %q after a kill interrupt", handle)
		}
	}
}

func TestStatusWithoutPatternsIsUnknown(t *testing.T) {
	requireTmux(t)

	ctx := context.Background()
	d := New(Patterns{})
	name := sessionName(t)

	handle, err := d.Spawn(ctx, name, []string{"bash", "--noprofile", "--norc"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	t.Cleanup(func() {
		_ = d.Interrupt(ctx, handle, true)
	})

	result, err := d.Status(ctx, handle)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if result.Status != driver.StatusUnknown || result.Confidence != "none" {
		t.Errorf("Status() = %+v, want Unknown/none with no patterns configured", result)
	}
}

func TestStatusMatchesConfiguredPatterns(t *testing.T) {
	requireTmux(t)

	ctx := context.Background()
	d := New(Patterns{
		Idle:    []string{`\$\s*$`},
		Blocked: []string{`Trust this folder\?`},
	})
	name := sessionName(t)

	handle, err := d.Spawn(ctx, name, []string{"bash", "--noprofile", "--norc"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	t.Cleanup(func() {
		_ = d.Interrupt(ctx, handle, true)
	})
	time.Sleep(200 * time.Millisecond)

	result, err := d.Status(ctx, handle)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if result.Status != driver.StatusIdle {
		t.Errorf("Status() = %+v, want Idle at a bare shell prompt", result)
	}
}

func TestShellQuoteViaSpawn(t *testing.T) {
	requireTmux(t)

	ctx := context.Background()
	d := New(Patterns{})
	name := sessionName(t)

	// The apostrophe and semicolon here must survive ShellJoin's
	// quoting intact for this to be valid bash at all.
	script := `echo "it's a test"; exec bash --noprofile --norc`
	handle, err := d.Spawn(ctx, name, []string{"bash", "-c", script})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	t.Cleanup(func() {
		_ = d.Interrupt(ctx, handle, true)
	})
	time.Sleep(300 * time.Millisecond)

	out, err := d.Read(ctx, handle, 10, false)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !strings.Contains(out, "it's a test") {
		t.Errorf("Read output %q does not contain the apostrophe-quoted text; quoting broke", out)
	}
}
