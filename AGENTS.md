# Repository Guidelines

## Project Structure & Module Organization
Clarity CLI is a Go CLI + Bubble Tea TUI for a **local-first** store.

Primary codepaths:
- CLI entrypoint: `cmd/clarity/`
- Cobra commands (scriptable surface): `internal/cli/`
- Bubble Tea TUI: `internal/tui/`
- Local store + schema: `internal/store/`, `internal/model/`
- Output formatting (JSON/EDN): `internal/format/`
- Embedded docs content: `internal/docs/content/`

## Build, Test, and Development Commands
From `clarity-cli/`:
- Build: `make build`
- Run (CLI/TUI): `make run` (or `go run ./cmd/clarity`)
- Install: `make install` (runs tests first)
- Tests: `make test` (or `go test ./...`)
- Format: `make fmt` (or `gofmt -w .`)
- Tidy deps: `make tidy`

## Local Store & Workspace Resolution
Clarity is workspace-first; storage defaults under `~/.clarity/workspaces/<name>/`.

Useful flags/env:
- `--workspace <name>` / `CLARITY_WORKSPACE`
- `--dir <path>` / `CLARITY_DIR` (bypass workspace resolution; good for fixtures/tests)
- `--actor <actor-id>` / `CLARITY_ACTOR`
- `--format json|edn` / `CLARITY_FORMAT`

## Coding Style & Naming Conventions
- Run `gofmt` on all Go changes (CI/local diffs should be gofmt-clean).
- Prefer small, composable Cobra commands in `internal/cli/` with stable flags and help text.
- Keep CLI output compatible with the documented output contract; avoid breaking changes to shapes/field names.

## Output Contract (Hard Requirement)
Default output is a stable JSON envelope:
- `data` (primary result)
- `meta` (optional: counts/pagination)
- `_hints` (optional: suggested follow-up commands)

Use `--pretty` only for human debugging; do not rely on it in scripts.
See: `clarity docs output-contract`.

## Identity, Comments, and Worklog (Hard Requirement)
All writes are attributed to an **actor**:
- Set once: `clarity identity use <actor-id>`
- Or per command: `clarity --actor <actor-id> ...`

Communication conventions:
- **Worklog** is for private execution notes / tracking work on a task: `clarity worklog add <item-id> --body "..."`
- **Comments** are for communicating with others (questions, clarification, decisions): `clarity comments add <item-id> --body "..."`

## Docs Updates
If you add or change user-visible CLI behavior, update the embedded docs under `internal/docs/content/` (and ensure `clarity docs <topic>` stays accurate).
