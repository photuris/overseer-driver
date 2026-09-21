package herdr

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/photuris/overseer-driver/internal/driver"
)

// requireLiveHerdr skips unless this process is running inside an
// actual Herdr-managed pane with a workspace to spawn into — these
// tests create and tear down real panes/tabs in that live session.
func requireLiveHerdr(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("herdr"); err != nil {
		t.Skip("herdr not installed")
	}
	if os.Getenv("HERDR_ENV") != "1" {
		t.Skip("not running inside Herdr (HERDR_ENV != 1)")
	}
	if os.Getenv("HERDR_WORKSPACE_ID") == "" {
		t.Skip("HERDR_WORKSPACE_ID not set")
	}
}

func testName(t *testing.T) string {
	t.Helper()

	return fmt.Sprintf("odtest-%d", time.Now().UnixNano())
}

// These tests only exercise the raw-fallback path (a plain "bash" is
// not a recognized agent kind), deliberately — spinning up a real
// coding-agent CLI has real cost and would visibly disrupt whatever
// live Herdr session is running these tests. The known-kind path
// (agent start / agent prompt / native Status) is verified against
// the installed herdr binary's --help and skill docs, not by a live
// spawn; see AGENTS.md.

func TestSpawnReadStatusRawFallback(t *testing.T) {
	requireLiveHerdr(t)

	ctx := context.Background()
	d := New()
	name := testName(t)

	handle, err := d.Spawn(ctx, name, []string{"bash", "--noprofile", "--norc"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	t.Cleanup(func() {
		_ = d.Interrupt(ctx, handle, true)
	})
	time.Sleep(500 * time.Millisecond)

	result, err := d.Status(ctx, handle)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if result.Confidence != "native" {
		t.Errorf("Status().Confidence = %q, want \"native\"", result.Confidence)
	}
	if result.Status != driver.StatusUnknown {
		t.Errorf("Status().Status = %q, want Unknown for a pane with no recognized agent", result.Status)
	}

	out, err := d.Read(ctx, handle, 10, false)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	_ = out // a bare shell prompt has no fixed expected text; reaching here without error is the check
}

func TestReadAnsiPreservesEscapeCodes(t *testing.T) {
	requireLiveHerdr(t)

	ctx := context.Background()
	d := New()
	name := testName(t)

	handle, err := d.Spawn(ctx, name, []string{"bash", "--noprofile", "--norc"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	t.Cleanup(func() {
		_ = d.Interrupt(ctx, handle, true)
	})
	time.Sleep(500 * time.Millisecond)

	// This pane has no recognized agent, so Prompt (which always
	// targets one) doesn't apply here — send text the same way
	// startInPane's raw fallback launches a command, via the
	// package's own run() (this file is in package herdr).
	if _, err := run(ctx, "pane", "run", string(handle), `printf '\033[2mdim\033[0m\n'`); err != nil {
		t.Fatalf("pane run: %v", err)
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

func TestSplitSharesTabAndSurvivesSiblingKill(t *testing.T) {
	requireLiveHerdr(t)

	ctx := context.Background()
	d := New()
	name := testName(t)

	first, err := d.Spawn(ctx, name, []string{"bash", "--noprofile", "--norc"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	t.Cleanup(func() {
		_ = d.Interrupt(ctx, first, true)
	})
	time.Sleep(500 * time.Millisecond)

	second, err := d.Split(ctx, first, driver.DirectionRight, name+"-2", []string{"bash", "--noprofile", "--norc"})
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if second == first {
		t.Errorf("Split() returned the same handle as its target: %q", second)
	}
	time.Sleep(500 * time.Millisecond)

	if err := d.Interrupt(ctx, second, true); err != nil {
		t.Fatalf("Interrupt(second, kill=true): %v", err)
	}

	// The sibling pane must still be addressable after the kill.
	if _, err := d.Status(ctx, first); err != nil {
		t.Errorf("Status(first) after killing its sibling: %v", err)
	}
}

func TestRenameKeepsHandle(t *testing.T) {
	requireLiveHerdr(t)

	ctx := context.Background()
	d := New()
	name := testName(t)

	handle, err := d.Spawn(ctx, name, []string{"bash", "--noprofile", "--norc"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	t.Cleanup(func() {
		_ = d.Interrupt(ctx, handle, true)
	})

	newHandle, err := d.Rename(ctx, handle, "renamed-label")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if newHandle != handle {
		t.Errorf("Rename() = %q, want the unchanged handle %q", newHandle, handle)
	}
}

// TestSpawnReturnsHandleOnStartFailure reproduces the exact failure
// reported from real use: `claude --version` is a known kind
// (command[0] == "claude"), but it prints and exits immediately, so
// `agent start` never sees it become ready and times out. Before this
// was fixed, Spawn discarded the pane's handle on that error, leaving
// a live tab the caller had no way to find or clean up.
func TestSpawnReturnsHandleOnStartFailure(t *testing.T) {
	requireLiveHerdr(t)
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude not installed")
	}

	ctx := context.Background()
	d := New()
	name := testName(t)

	handle, err := d.Spawn(ctx, name, []string{"claude", "--version"})
	if err == nil {
		t.Cleanup(func() { _ = d.Interrupt(ctx, handle, true) })
		t.Fatalf("Spawn(claude --version) succeeded; expected agent start to time out waiting for readiness")
	}
	if handle == "" {
		t.Fatalf("Spawn(claude --version) returned no handle alongside its error %v; the pane exists and is now unreachable", err)
	}
	t.Cleanup(func() {
		_ = d.Interrupt(ctx, handle, true)
	})

	// The handle must still be a real, addressable pane.
	if _, statusErr := d.Status(ctx, handle); statusErr != nil {
		t.Errorf("Status(%q) after a start failure: %v; handle was not actually usable", handle, statusErr)
	}
}

func TestShellQuoteViaSpawn(t *testing.T) {
	requireLiveHerdr(t)

	ctx := context.Background()
	d := New()
	name := testName(t)

	script := `echo "it's a test"; exec bash --noprofile --norc`
	handle, err := d.Spawn(ctx, name, []string{"bash", "-c", script})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	t.Cleanup(func() {
		_ = d.Interrupt(ctx, handle, true)
	})
	time.Sleep(500 * time.Millisecond)

	out, err := d.Read(ctx, handle, 10, false)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !strings.Contains(out, "it's a test") {
		t.Errorf("Read output %q does not contain the apostrophe-quoted text; quoting broke", out)
	}
}
