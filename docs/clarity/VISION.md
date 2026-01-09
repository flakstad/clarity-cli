# Clarity V1 (CLI + TUI) — Vision and Principles

Clarity is a local-first, terminal-first project management and communication tool designed to increase clarity, focus, and stillness.

V1 is intentionally **just the Go CLI + Bubble Tea TUI** with a local store. No server.

## What we are building

Clarity is closer to **Org Mode + Basecamp** than to a dashboard-heavy PM tool:

- **Outline-native work**: hierarchical items as the core primitive.
- **Communication-first**: tasks, decisions, and discussion live together with a clear history.
- **Calm by default**: reduce notification pressure; support intentional review instead of constant activity.
- **Agents and humans**: humans primarily use the TUI; agents primarily use the CLI.

## Core principles (product)

- **Clarity over complexity**: features must reduce cognitive load.
- **Quiet defaults**: avoid engagement-driven design; prefer review/digest patterns over interruption.
- **Linear communication**: keep discussion focused and easy to follow.
- **Replayable history**: keep a trustworthy audit trail of changes (“why did we do this?”).
- **Capture first, organize later**: make it easy to record work quickly, then refine structure.
- **Ownership with accountability**: avoid ambiguity about who can change what, and make attribution explicit.

## Non-goals (for V1)

- No server, no web UI, no sync service.
- No surveillance features (time tracking, productivity scoring, “activity feeds” designed to create anxiety).
- No competitor feature-parity checklist; additions must serve calm and clarity.

## Builder constraints (how this shows up in the CLI/TUI)

- **Stable scriptable CLI**: default output is a stable envelope; keep fields and shapes compatible.
- **Progressive disclosure**: commands should provide `_hints` to guide the next action.
- **Explicit attribution**: all writes are attributed to an actor; identity and permissions are first-class.
- **Separation of concerns**:
  - **comments**: communicate with other people
  - **worklog**: private execution notes / agent traces

## Related docs (in this repo)

- `internal/docs/content/output-contract.md` (output envelope contract)
- `internal/docs/content/getting-started.md` (how to use the CLI)
- `skills/clarity-cli/SKILL.md` (how external agents should use `clarity`)
