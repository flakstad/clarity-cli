# Clarity identity + agent sessions (skill reference)

This skill assumes **all writes are attributed to an actor**:
- `human`: a real user
- `agent`: belongs to a human user

## Selecting an actor (required for writes)
- Set once: `clarity identity use <actor-id>`
- Or per command: `clarity --actor <actor-id> ...`

## Agent workflow (recommended for autonomous agents)

Use a dedicated agent identity and explicitly claim items you work on.

- **Stable identity per session (recommended)**:
  - Set a session key once: `CLARITY_AGENT_SESSION=<session>`
  - Start work (ensures identity + claims item): `clarity agent start <item-id>`

- **New identity per run (default)**:
  - Omit `CLARITY_AGENT_SESSION`
  - Start work: `clarity agent start <item-id>`

### Session key conventions

Session keys are opaque strings chosen by the agent/tool, but Clarity applies one normalization for readability:

- If `<session>` looks like `<prefix>-YYYY-MM-DD` (example: `cursor-2025-12-20`),
  Clarity deterministically normalizes it to `<prefix>-<3 letters>` (example: `cursor-xvf`).
  This preserves the property: **same input session ⇒ same agent identity**.

If you want full control, just pass a short stable key yourself (example: `cursor-xvf`, `codex-abc`, etc.).

## Avoiding agent collisions
- `clarity items ready` shows **unassigned** items by default
- `clarity items claim` / `clarity agent start` refuse to take an already-assigned item
- Use `--take-assigned` to explicitly take an already-assigned item
