# overseer-driver

Cross-platform CLI driver for the [overseer skill][overseer]'s
multi-agent orchestration interface. Deterministic mechanics — spawn,
read, prompt, list, rename, interrupt, notify — live here as tested
Go, instead of prose an LLM has to correctly re-derive on every call.
`status` stays best-effort where a harness has no native
agent-awareness (see below).

[overseer]: https://github.com/photuris/agent-skills/tree/main/shared/overseer

## Status

Early / private. One harness implemented:

| Harness | Status |
|---------|--------|
| tmux | implemented |
| Herdr | not yet ported |

## Install

```
go install github.com/photuris/overseer-driver/cmd/overseer-driver@latest
```

Prebuilt binaries: none published yet (this repo is still private;
see the overseer skill's plan for public release).

## Usage

Every subcommand takes `--harness <name>` and prints one JSON object
to stdout on success. Failures go to stderr with a nonzero exit code —
a caller only needs to parse stdout when the exit code says to.

```
overseer-driver spawn     --harness tmux --name impl-003 -- claude --model opus
overseer-driver read      --harness tmux --target impl-003 --lines 30
overseer-driver prompt    --harness tmux --target impl-003 --text "Task ready: .overseer/tasks/003.md"
overseer-driver list      --harness tmux
overseer-driver rename    --harness tmux --target impl-003 --label impl-003-retry
overseer-driver interrupt --harness tmux --target impl-003 [--kill]
overseer-driver status    --harness tmux --target impl-003 [--patterns patterns.json]
overseer-driver notify    --message "..." [--title "Overseer"]
```

`notify` has no `--harness` — it reaches the user via the OS
(`notify-send` on Linux, `osascript` on macOS), not through the
multi-agent tool. No Windows notifier yet; `notify` reports
`{"sent":false,"method":"none"}` there rather than claiming to work
untested.

### `prompt` does not wait

`prompt` only sends text and submits it — it does not block until the
agent goes idle. Waiting is a judgment call layered on an approximate
`status`, and conflating "send" (deterministic) with "wait"
(heuristic) risks hanging on a bad pattern. Callers poll `read` or
`status` themselves.

### `status` and patterns

tmux has no concept of "agent," only panes and their text, so `status`
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

`confidence` in the response is `"none"` (no config), `"heuristic"`
(matched against config), or would be `"native"` for a harness that
can report status directly (Herdr, once ported) — never trust a
`"heuristic"` result as much as the health-watch logic in the overseer
skill trusts a `"native"` one.

## Handles

Each `spawn` creates its own tmux session; the handle is that
session's name. Multiple agents sharing one tmux window (split panes)
is a layout concern this CLI does not implement yet — one session per
agent, addressed by name, for now.

## Development

```
go build ./...
go vet ./...
go test ./...          # tmux tests are skipped if tmux isn't on PATH
```

No external dependencies — stdlib only, deliberately, to keep
cross-compilation and the trust surface small.

## License

Not yet decided; this repo is private. Revisit before any public
release.
