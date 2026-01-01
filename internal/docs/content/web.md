# Web UI (v1, self-host)

Clarity is local-first. The v1 web UI is designed to run **on your machine** and operate on the same workspace directories and Git repos as the CLI/TUI.

## Run

```bash
clarity web --addr 127.0.0.1:3333
```

Or explicitly select a workspace:

```bash
clarity --workspace "Flakstad Software" web --addr :3333
```

## Auth (v1)

Today there are two modes:
- `--auth none` (default): no browser login; use a fixed actor by starting with `--actor`, or run read-only.
- `--auth dev`: local/dev mode that lets you pick a human actor in the browser at `/login` (sets a cookie).

## Current status

The web UI is **experimental** and currently focuses on:
- Read-only views for confidence and debugging
- Showing Git status so teams can see when they’re behind/dirty/conflicted

If you start the server with `--read-only=false`, a minimal write path is enabled:
- post comments from the item detail page (requires an actor; start with `--actor` or set a current actor via `clarity identity use`)

Current routes (read-only):
- `/` (home)
- `/agenda`
- `/sync`
- `/projects`
- `/projects/<project-id>`
- `/outlines/<outline-id>`
- `/items/<item-id>`

Dev auth routes:
- `/login`

Writes, login, and multi-user semantics are planned under the `Pivot` epic.
