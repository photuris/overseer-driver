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

	out, err := d.Read(ctx, handle, 10)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	_ = out // a bare shell prompt has no fixed expected text; reaching here without error is the check
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

	out, err := d.Read(ctx, handle, 10)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !strings.Contains(out, "it's a test") {
		t.Errorf("Read output %q does not contain the apostrophe-quoted text; quoting broke", out)
	}
}
