# overseer-driver — agent instructions

This is the companion CLI to the `overseer` skill in the
[agent-skills][agent-skills] repo. That skill's `SKILL.md` defines the
abstract interface this CLI implements (spawn/read/prompt/list/
rename/interrupt/status/notify); `resources/<harness>.md` files there
document what a driver's mechanics look like in prose, for harnesses
this CLI hasn't been ported to yet or for an LLM reasoning about a
harness directly. Keep the two in sync: a change to one side's
contract (a flag, a JSON field, a primitive's semantics) is a change
to both.

[agent-skills]: https://github.com/photuris/agent-skills

## Conventions

Follows this account's `go-style` skill: `gofmt`/`go vet` clean,
`cmd/<name>/main.go` + `internal/` layout (no `pkg/`), doc comments on
every exported name, accept interfaces / return structs, no `init()`
or package-level mutable state, early returns, stdlib `testing` with
table-driven cases where the shape fits.

Deliberately dependency-free (stdlib only): no cobra, no external YAML
lib (patterns config is JSON, not YAML, for this reason). Keep it that
way unless something stdlib genuinely cannot do reasonably — this
buys simple cross-compilation and a small trust surface for anyone
running a downloaded binary.

## Design decisions worth knowing before changing things

- **`prompt` does not wait.** Sending text is deterministic; waiting
  for idle depends on the approximate `status`. Conflating the two
  risks a call hanging on a bad pattern. Callers poll.
- **`status` degrades honestly.** tmux with no `--patterns` configured
  means `{"status":"unknown","confidence":"none"}`, not a guess. Herdr
  always reports `confidence: "native"` — it tracks agent state itself
  (`agent_status` in `pane get`/`agent list`), no pattern matching.
- **A `Handle` is always a full address, in both drivers.** tmux:
  `session:window.pane` (from `new-session -P -F`/`split-window -P
  -F`, same format `list-panes -a` reports). Herdr: a pane ID
  (`w1:p2`) — Herdr's own docs confirm pane IDs work as targets for
  both pane- and agent-level commands, and are stable across rename
  (a session/pane *title* changes; the address never does).
- **`Rename` never changes the handle, in either driver.** tmux uses
  `select-pane -T` (a cosmetic title, not `rename-session`, which
  would rename the whole session out from under any sibling panes).
  Herdr uses `pane rename` (a label) for the same reason. Both return
  the input handle unchanged — a real API difference from the first
  tmux implementation, which renamed the session and returned a new
  handle; fixed when `Split` made that unsafe (see below).
- **`Interrupt(kill=true)` closes only the target's own pane**
  (`tmux kill-pane` / `herdr pane close`), never the whole
  session/tab. A session or tab can hold sibling panes spawned by
  `Split` — killing at that level would take them out too. Verified
  directly: split two panes, killed one, confirmed the other survived
  in both drivers.
- **`Split` (driver.Layouter) adds a pane to an existing window/tab**
  instead of isolating every agent in its own session/tab the way
  `Spawn` does. Only tmux and Herdr implement it so far; a driver that
  has no visual layout concept simply doesn't implement the interface,
  and the CLI's `split` subcommand reports "no native layout" rather
  than pretending to support it.
- **Herdr's `--lines` is broken with the default read source.** On
  herdr 0.8.2, `pane read --lines N` silently returns empty output
  with the default `--source recent` *or* `--source
  recent-unwrapped` — only `--source visible` actually honors
  `--lines`. Found by an 8/8 failure rate in a tight spawn+read stress
  loop; fixed by always passing `--source visible` explicitly in
  `Read`. Confirmed live, not assumed from docs — don't drop that flag
  without re-testing against a real herdr session. (An earlier fixed
  150ms "pane settle" delay looked like it might be the fix too; it
  wasn't — removed after the real cause was found and confirmed 0/10
  clean without it.)
- **Herdr's `agent start` only knows a fixed `--kind` list**, not an
  arbitrary command. `Spawn`/`Split` use it when `command[0]` matches
  one of those kinds; anything else (a shell alias/wrapper) falls back
  to `pane run` + `pane rename`, which loses native `Status`/`Prompt`
  — there's no recognized agent in that pane for Herdr to report on
  or submit to. This mirrors what the overseer skill's
  `resources/herdr.md` already documented in prose before this CLI
  existed.
- **The tmux server-exit race.** Killing the last tmux session exits
  the server; a command issued immediately after can hit a stale
  socket and fail with "server exited unexpectedly". `run()` retries
  once after a short pause — verified against a real tmux server, not
  guessed.
- **`notify` prefers Herdr's own notification** when `HERDR_ENV=1`,
  falling back to an OS-level notifier otherwise. Herdr can return
  success while showing nothing (`{"shown":false,"reason":"disabled"}`
  when the user has notifications off) — `Result.Sent` reflects
  `shown`, not the command's exit code; don't just check the error.

## Testing

`internal/driver/tmux` and `internal/driver/herdr` tests shell out to
the real binary and skip (`t.Skip`) when it's not on `PATH` (Herdr
tests also skip outside `HERDR_ENV=1`/without `HERDR_WORKSPACE_ID`).
They are integration tests, not unit tests with a fake — the value
here is catching real CLI syntax and behavior drift (like the
`--lines` bug above), which a mock can't do.

The Herdr tests only exercise the raw-fallback path (a plain `bash` is
not a recognized agent kind) — spinning up a real coding-agent CLI
just to test against has real cost and visibly disrupts whatever live
Herdr session is running the tests. The known-kind path (`agent
start`/`agent prompt`/native `Status` on a real agent) is verified
against the installed `herdr --help` output and `herdr --skill` docs,
plus manual one-off testing against real already-running agents for
the read-only calls (`list`, `status`, `read`) — not by an automated
live spawn.
