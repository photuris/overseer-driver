# overseer-driver

Cross-platform CLI driver for the [overseer skill][overseer]'s
multi-agent orchestration interface. Deterministic mechanics — spawn,
split, read, prompt, list, rename, interrupt, notify — live here as
tested Go, instead of prose an LLM has to correctly re-derive on every
call. `status` stays best-effort where a harness has no native
agent-awareness (see below).

[overseer]: https://github.com/photuris/overseer

## Status

Early. Two harnesses implemented:

| Harness | Status |
|---------|--------|
| tmux | implemented |
| Herdr | implemented |

## Install

```
go install github.com/photuris/overseer-driver/cmd/overseer-driver@latest
```

Without Go: download the archive for your OS/arch from the
[latest release](https://github.com/photuris/overseer-driver/releases/latest),
extract it, and put `overseer-driver` on your `PATH`.

## Usage

Every driver-backed subcommand takes `--harness <name>` (`tmux` or
`herdr`) and prints one JSON object to stdout on success. Failures go
to stderr with a nonzero exit code — a caller only needs to parse
stdout when the exit code says to.

```
overseer-driver spawn     --harness tmux --name impl-003 -- claude --model opus
overseer-driver split     --harness tmux --target impl-003 --name impl-004 --direction right -- claude --model opus
overseer-driver read      --harness tmux --target impl-003 --lines 30
overseer-driver prompt    --harness tmux --target impl-003 --text "Task ready: .overseer/tasks/003.md"
overseer-driver list      --harness tmux
overseer-driver rename    --harness tmux --target impl-003 --label impl-003-retry
overseer-driver interrupt --harness tmux --target impl-003 [--kill]
overseer-driver status    --harness tmux --target impl-003 [--patterns patterns.json]
overseer-driver notify    --message "..." [--title "Overseer"] [--sound request]
```

`notify` has no `--harness` — it reaches the user, not an agent. It
prefers Herdr's own notification when running inside Herdr
(`HERDR_ENV=1`), falling back to an OS-level notifier (`notify-send`
on Linux, `osascript` on macOS) otherwise. No Windows notifier yet;
`notify` reports `{"sent":false,"method":"none"}` there rather than
claiming to work untested. `--sound` is Herdr-specific
(`none`/`done`/`request`) and ignored by the OS-level fallback. Herdr
can report success while showing nothing at all if the user has
notifications turned off — `sent` reflects whether it actually showed,
not just whether the command succeeded.

### `prompt` does not wait

`prompt` only sends text and submits it — it does not block until the
agent goes idle. Waiting is a judgment call layered on an approximate
`status`, and conflating "send" (deterministic) with "wait"
(heuristic) risks hanging on a bad pattern. Callers poll `read` or
`status` themselves.

### `status`: native vs. approximate

Herdr tracks agent lifecycle state itself and reports it directly —
`status` calls always come back `"confidence":"native"` for it. tmux
has no concept of "agent," only panes and their text, so its `status`
is pattern-matching against a caller-supplied config, not a lookup.
Without `--patterns`, it always reports `{"status":"unknown",
"confidence":"none"}` — the honest answer when nothing has taught it
what a given tool's prompts look like. `--patterns` points at a JSON
file:

```json
{
  "idle": ["\\$\\s*$"],
  "blocked": ["Trust this folder\\?"]
}
```

Never trust a `"heuristic"` result (tmux) as much as a `"native"` one
(Herdr) — the overseer skill's health-watch logic treats them
differently for exactly this reason.

## Handles

A `Handle` is always a full, stable address in both drivers — never
just a name a rename could invalidate.

- **tmux**: `session:window.pane` (e.g. `impl-003:1.2`), the same
  format `spawn`, `split`, and `list` all report. `rename` sets the
  pane's title only; the handle it returns is unchanged.
- **Herdr**: a pane ID (e.g. `w1:p2`), Herdr's own stable public
  identifier — valid as a target whether or not the pane holds a
  Herdr-recognized agent. `rename` sets the pane's label only; the
  handle is unchanged there too.

`interrupt --kill` closes only the target's own pane, never a whole
session/tab — a session or tab can hold sibling panes from other
agents spawned via `split`.

## `spawn` vs. `split`

`spawn` isolates a new agent in its own session (tmux) or tab (Herdr).
`split` adds one alongside an existing pane instead, in the same
window/tab — this is the `driver.Layouter` interface; not every driver
implements it, and `split` reports "no native layout" for one that
doesn't. Herdr's `agent start` only knows a fixed list of agent kinds
(`claude`, `codex`, `gemini`, …, see `herdr agent start --help`); a
command that doesn't match one runs as a raw, unrecognized process
instead (a shell alias wrapping a known tool, for example) — that
pane won't support `prompt` or native `status`, since Herdr has
nothing recognized in it to talk to or report on.

## Development

```
go build ./...
go vet ./...
go test ./...          # driver tests skip if their real binary/env isn't available
```

tmux tests skip if `tmux` isn't on `PATH`. Herdr tests additionally
skip outside `HERDR_ENV=1` with `HERDR_WORKSPACE_ID` set, and only
exercise the raw-fallback path — they create and tear down real
panes/tabs in whatever live Herdr session runs them, so they
deliberately never spawn a real paid agent CLI. See AGENTS.md for the
known-kind path's verification story.

No external dependencies — stdlib only, deliberately, to keep
cross-compilation and the trust surface small.

## License

Not yet decided; this repo is private. Revisit before any public
release.
