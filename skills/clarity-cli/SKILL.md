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
- You can access a Clarity workspace (default local location: `~/.clarity/workspaces/<name>/`) or pass `--dir <path>`.

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

## Workspace-first usage
Typical (recommended) workflow:

```bash
clarity init
clarity workspace init default
clarity workspace use default
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
