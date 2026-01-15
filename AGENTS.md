# Repository Guidelines

## Project Structure & Module Organization
Clarity CLI is a Go CLI + Bubble Tea TUI for a **local-first** store.

## Product Philosophy & Direction
Clarity aims to be **calm project management**: outline-native work + communication with clear history, designed to reduce noise and support focus.

V1 in this repo is intentionally **local-first and terminal-first**:
- Humans primarily use the TUI (`clarity` opens Bubble Tea).
- Agents/scripting primarily use the CLI (stable command surface, stable output envelope, `_hints` for progressive disclosure).

Longer-term direction includes calm communication and replay/history; keep these in mind when designing data models, CLI surfaces, and UX defaults.

Read for context:
- `docs/clarity/VISION.md` (product vision + principles; start here)
- `internal/docs/content/direction.md` (current CLI-era direction notes)
- `internal/docs/content/output-contract.md` (hard requirement: stable output envelope)

Primary codepaths:
- CLI entrypoint: `cmd/clarity/`
- Cobra commands (scriptable surface): `internal/cli/`
- Bubble Tea TUI: `internal/tui/`
- Local store + schema: `internal/store/`, `internal/model/`
- Output formatting (JSON/EDN): `internal/format/`
- Git-backed workspace sync/autocommit: `internal/gitrepo/`
- Embedded docs content: `internal/docs/content/`

Out of scope for V1 (may exist as experiments; do not expand scope unless explicitly asked):
- `internal/web/`, `internal/webtui/`

## Build, Test, and Development Commands
From `clarity-cli/`:
- Build: `make build`
- Run (CLI/TUI): `make run` (or `go run ./cmd/clarity`)
- Install: `make install` (runs unit + integration tests first)
- Tests: `make test` (or `go test ./...`)
- Integration tests: `make it` (runs `scripts/cli_integration.sh`)
- Format: `make fmt` (or `gofmt -w .`)
- Tidy deps: `make tidy`

## Concurrent Agent Workflow (Hard Requirement)
When multiple agents work in the same repo concurrently, **never** switch branches or do destructive git operations (e.g. `git reset --hard`, `git clean -fdx`) in the shared checkout. Instead, each agent must work in its **own git worktree + branch**.

Reference:
- Workflow details: `docs/agents/worktrees.md`
- Base template (optional): `docs/agents/AGENTS.base.md`

Use this pattern:

```bash
# Prefer a stable per-agent id when available.
AGENT_ID="${CLARITY_AGENT_SESSION:-${AGENT_SESSION:-agent}}"
ITEM_ID="${CLARITY_ITEM_ID:-item-task}" # typically item-xxx
ITEM_SUFFIX="${ITEM_ID#item-}"          # prefer using just xxx in branch names

BRANCH="agent/${AGENT_ID}/${ITEM_SUFFIX}"
WT_BASE="${WORKTREE_BASE:-$PWD/.worktrees}"
WORKTREE_DIR="$WT_BASE/${AGENT_ID}-${ITEM_ID}"

mkdir -p "$(dirname "$WORKTREE_DIR")"
git worktree add -b "$BRANCH" "$WORKTREE_DIR" HEAD
cd "$WORKTREE_DIR"
```

Rules:
- Do all edits/tests/formatting inside the worktree directory.
- Treat the shared checkout as read-only.
- Test early and often; set up a tight feedback loop (run targeted tests/builds after each small change, then broader checks before handoff).
- Agents may commit freely to their own branch/worktree; prefer small, standalone commits and commit frequently.
- When finished, open a PR or hand off the branch name; do not merge to `main` unless explicitly asked.
- Humans merge from the shared checkout on `main` by merging the remote agent branch (example: `git merge --no-ff origin/agent/<agent-id>/<item-suffix>`).
- Cleanup (only after handoff): `git worktree remove "$WORKTREE_DIR"` (optional) and delete the branch if requested.

## Local Store & Workspace Resolution
Clarity is workspace-first; workspaces are resolved in this order:
1) `--dir` / `CLARITY_DIR` (advanced override; points at a workspace root that contains `.clarity/`, or at a legacy `.clarity/` dir itself)
2) `--workspace` / `CLARITY_WORKSPACE` (resolved via `~/.clarity/config.json` workspace registry when present, otherwise falls back to legacy `~/.clarity/workspaces/<name>/`)
3) `~/.clarity/config.json` `currentWorkspace`
4) implicit default workspace: `default`

Notes:
- `CLARITY_CONFIG_DIR` overrides `~/.clarity` (useful for tests/fixtures to avoid touching the real home dir).
- If `CLARITY_DIR` is set in the environment but `--workspace` is explicitly provided (without `--dir`), `--workspace` wins.
- Prefer registering non-legacy workspaces (e.g. Git-backed) via `clarity workspace add <name> --dir <path>`.

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

## Capturing unrelated issues (default for agents)
If you discover an issue or follow-up that is **unrelated** to the current task you’re working on, do **not** expand scope.

Instead, immediately file a new item and include where it came from:

```bash
clarity items create --title "..." --description "..." --filed-from <current-item-id>
```

## Docs Updates
If you add or change user-visible CLI behavior, update the embedded docs under `internal/docs/content/` (and ensure `clarity docs <topic>` stays accurate).
