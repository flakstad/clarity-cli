---
name: clarity-cli
description: Use the local-first Clarity CLI (project management for humans and their agents) via a stable, scriptable command surface.
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
- Storage lives under `~/.clarity/workspaces/<name>/` (by default: `~/.clarity/workspaces/default/`).
- Only use `--workspace <name>` or `--dir <path>` when the user explicitly asks for a specific workspace or an isolated store (fixtures/tests).

## How to discover capabilities (progressive disclosure)
- High-level help: `clarity --help`
- Feature-level help: `clarity <command> --help`
- Item lookup shortcut: `clarity <item-id>` (equivalent to `clarity items show <item-id>`)
- Find the next thing to work on: `clarity items ready`
- Long-form docs (on demand): `clarity docs` and `clarity docs <topic>`
- Output convention: `clarity docs output-contract`

## Output contract
- Default output is a stable JSON envelope: `{ "data": ... }`
- Some commands also return `meta` and `_hints` for progressive disclosure
- Use `--pretty` for readability while debugging

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

For session key conventions and ownership details, see:
- `skills/clarity-cli/identity.md`

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

## Workspace-first usage
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
clarity identity create --kind human --name "andreas" --use
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
