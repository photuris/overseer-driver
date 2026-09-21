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

	if err := d.Prompt(ctx, handle, `echo hello world`); err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	out, err := d.Read(ctx, handle, 10)
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

func TestRename(t *testing.T) {
	requireTmux(t)

	ctx := context.Background()
	d := New(Patterns{})
	name := sessionName(t)

	handle, err := d.Spawn(ctx, name, []string{"bash", "--noprofile", "--norc"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	newLabel := name + "-renamed"
	newHandle, err := d.Rename(ctx, handle, newLabel)
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	t.Cleanup(func() {
		_ = d.Interrupt(ctx, newHandle, true)
	})

	if string(newHandle) != newLabel {
		t.Errorf("Rename() = %q, want %q", newHandle, newLabel)
	}

	handles, err := d.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, h := range handles {
		if h == handle {
			t.Errorf("List() still contains the pre-rename handle %q", handle)
		}
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

func TestShellQuote(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"claude"}, "claude"},
		{[]string{"claude", "--model", "opus"}, "claude --model opus"},
		{[]string{"echo", "hello world"}, "echo 'hello world'"},
		{[]string{"echo", "it's"}, `echo 'it'\''s'`},
	}
	for _, c := range cases {
		if got := shellJoin(c.in); got != c.want {
			t.Errorf("shellJoin(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
