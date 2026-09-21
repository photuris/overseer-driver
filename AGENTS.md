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
- **`status` degrades honestly.** No `--patterns` configured means
  `{"status":"unknown","confidence":"none"}`, not a guess.
- **One tmux session per agent.** `Handle` is the session name.
  Multi-agent-per-window layouts are unimplemented, not broken by
  design — a future `layout` subcommand territory, not `spawn`'s.
- **The tmux server-exit race.** Killing the last tmux session exits
  the server; a command issued immediately after can hit a stale
  socket and fail with "server exited unexpectedly". `run()` retries
  once after a short pause — verified against a real tmux server, not
  guessed. Don't remove it without re-verifying the race is gone.

## Testing

`internal/driver/tmux` tests shell out to a real `tmux` binary and
skip (`t.Skip`) if it's not on `PATH`. They are integration tests, not
unit tests with a fake — the value here is catching real tmux syntax
drift, which a mock can't do.
