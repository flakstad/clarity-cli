---
name: clarity-cli
description: Use Clarity as the default system of record for task management, work logging, and project communication; agents should create/claim items, update status, and record progress via comments/worklog using the `clarity` CLI.
---

## Purpose
This skill equips an agent to help a human operate **Clarity**: a local-first project management tool for humans and their agents.

Use Clarity to:
- capture and organize work as **items** in **outlines** and **projects**
- model blocking work with **dependencies**
- communicate via **comments** and private **worklog**
- keep strict attribution via **identities** (actors)

## Preconditions (local-only)
- `clarity` is installed and available on `PATH` (or use a direct binary path).
- **Default workspace is the norm**: assume all work happens in the implicit `default` workspace unless the user explicitly tells you otherwise.
- Only use `--workspace <name>` or `--dir <path>` when the user explicitly asks for a specific workspace or an isolated store.

## How to discover capabilities (progressive disclosure)
- High-level help: `clarity --help`
- Feature-level help: `clarity <command> --help`
- Direct item lookup: `clarity <item-id>` (equivalent to `clarity items show <item-id>`)
- Find the next thing to work on: `clarity items ready`
- Long-form docs (on demand): `clarity docs` and `clarity docs <topic>`
- Output convention: `clarity docs output-contract`
- Follow-up discovery: many commands return `_hints` with suggested next commands

## Output contract
- Default output is a stable JSON envelope: `{ "data": ... }` (optionally `meta` and `_hints`)
- Formats: `--format json|edn` (or `CLARITY_FORMAT`)
- Use `--pretty` only for human debugging; do not rely on it in scripts/agents

## Shell quoting for `--body` / `--description` (avoid backtick hiccups)
When passing freeform text to `--body` / `--description`, avoid unescaped shell metacharacters (especially in `zsh`):
- Avoid Markdown inline code backticks (`` `like this` ``) in command arguments; backticks trigger command substitution in many shells.
- Prefer plain text (e.g. `clarity worklog add ... --body "Deployed X; next: Y"`).

Safe patterns:

```bash
# Option A: single quotes (safest for literal backticks and $() text)
clarity worklog add <item-id> --body 'Ran `go test ./...`; fixed failing test'

# Option B: here-doc (best for long multi-line updates)
clarity comments add <item-id> --body "$(cat <<'EOF'
Implemented X.
Next: Y.
EOF
)"
```

## Identity model (hard requirement)
All writes are attributed to an **actor**:
- Actors are either `human` or `agent`
- Agents must be connected to a human user (`--user <human-actor-id>`)

You must set an active actor before doing writes:
- `clarity identity use <actor-id>`, or
- pass `--actor <actor-id>` to each command

Ownership rules are enforced by the CLI (V1). If you can’t edit, comment instead.

## Agent workflow (recommended for autonomous agents)
If you are an autonomous agent (Cursor/Codex/etc), you should operate under your own `agent` identity and explicitly claim items you work on.

Two supported patterns:

- **Stable identity per session (recommended)**:
  - Set a session key once (e.g. `CLARITY_AGENT_SESSION=codex-abc`)
  - Run `clarity agent start <item-id>` to ensure identity + claim item

- **New identity per run (sufficient default)**:
  - Omit the session key; Clarity will generate one automatically
  - Run `clarity agent start <item-id>`

Session key notes:
- `CLARITY_AGENT_SESSION` can be any short string that stays stable for your tool/session (example: `codex-abc`).
- If you use a long “date-like” suffix (e.g. `cursor-2025-12-20`), Clarity may normalize it to a shorter stable key for readability; the important property is “same input ⇒ same agent identity”.

Minimal loop:

```bash
# 1) Find work
clarity items ready

# 2) Start work (ensure identity + claim ownership)
clarity agent start <item-id>

# If another agent already claimed it, you must be explicit:
clarity agent start <item-id> --take-assigned

# 3) Move item to "in progress" (status ids vary per outline; "doing" is the default)
clarity items set-status <item-id> --status doing

# 4) Do work and record updates (prefer worklog for private notes, comments for collaboration)
clarity worklog add <item-id> --body "Implemented X; next: Y"
clarity comments add <item-id> --body "FYI: shipped X; open question: Y"

# 5) When finished, mark it done (end-state status id varies per outline; "done" is the default)
# Tip: `clarity items show <item-id>` will include `_hints` with the recommended end-state for that outline.
clarity items set-status <item-id> --status <done-status>
```

## Capturing unrelated issues (hard requirement for autonomous agents)
When you discover an issue or follow-up that is **not necessary to complete the current item**, you must:
- **Create a new item immediately**
- **Include where it came from** (the current item id, plus any relevant file/command/trace)
- **Return to the current item** (do not expand scope)

Use `--filed-from` to standardize the “where it came from” metadata:

```bash
clarity items create --title "..." --description "..." --filed-from <current-item-id>
```

Environment variables (optional conveniences):
- `CLARITY_AGENT_SESSION`: stable identity within a session; omit for new identity per run
- `CLARITY_AGENT_NAME`: display name used when creating agent identities
- `CLARITY_AGENT_USER`: parent human actor id (if not resolvable from current actor)

## Workspace-first usage (where data lives)
Workspace resolution is workspace-first and supports both registered and legacy workspaces:
- Registered workspaces live in `~/.clarity/config.json` and can point anywhere on disk (useful for Git-backed workspaces).
- Legacy workspaces live under `~/.clarity/workspaces/<name>/`.
- `CLARITY_CONFIG_DIR` overrides `~/.clarity` (useful for isolated runs).
- `--dir` / `CLARITY_DIR` is an advanced override for pointing at a specific workspace root (or a legacy `.clarity/` directory).
- If `CLARITY_DIR` is set but `--workspace` is explicitly provided (without `--dir`), `--workspace` wins.

Typical (recommended) workflow (default workspace):

```bash
clarity init
```

If (and only if) you are explicitly told to use a different workspace:

```bash
clarity workspace use <name>
```

## Bootstrap (minimum to start tracking work)

```bash
clarity init
clarity identity create --kind human --name "<your-name>" --use
clarity projects create --name "Clarity" --use
clarity outlines create --project <project-id>
```

## First actions to try
- Create an item: `clarity items create --project <project-id> --title "..." --description "..."`
- Pick the next item to work on: `clarity items ready` then open it with `clarity <item-id>`
- Add a blocker: `clarity deps add <item-id> --blocks <item-id>`
- Comment: `clarity comments add <item-id> --body "..."`
- Check status: `clarity status`

For everything else, rely on `clarity docs` and `clarity <command> --help`.
