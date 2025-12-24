# Notifications plan (derived from local events)

This document captures the current plan and assumptions for “on-demand communication notifications” as a **Notifications page**: a feed of **events** for **outline items you follow**.

No implementation is included here—this is a design note based on what we’ve discussed so far and what exists in the repo today.

## Context and constraints (from project docs)

- **Local-first**: Clarity stores data locally (workspace-first), typically under `~/.clarity/workspaces/<name>/`.
- **Core model is stable**: `workspace -> projects -> outlines -> items`. Outlines are intentionally flexible containers (tasks, announcements, docs/notes).
- **Communication primitives exist**:
  - **Comments**: for communicating with others.
  - **Worklog**: for private execution notes.
- **Scriptable CLI contract**: default output is a stable JSON envelope with optional `meta` and `_hints` (progressive disclosure).

## Existing foundation in the codebase

Clarity already records an append-only **event log** for core actions, including (examples):

- `item.create`
- `item.set_status`
- `comment.add`
- `worklog.add`

The current store includes an event log backend (SQLite-backed by default, with legacy JSONL compatibility). This makes it feasible to query recent events efficiently and build a “notifications feed” without inventing a new write-heavy system first.

## Product shape (consumer-first)

Primary consumer: a **Notifications page** where “you see your notifications”.

Definition (current): **Notifications are events** that happened on items in outlines you **follow**.

Initial event types envisioned:
- **New item created**
- **Status changes**
- **New comments**

## Proposed minimal model

### 1) Follow state (persisted)

Persist “who follows what” as first-class state.

Minimal record shape:
- `actorId`
- `targetKind`: `"outline"` or `"item"`
- `targetId`
- `createdAt`

Notes:
- Start with `outline` follows as the primary feature (matches “outline items you follow”).
- Item-level follows can be added later (useful for “track just this one thing”).

### 2) Notifications feed (derived view)

Do **not** precompute and store notifications as rows initially. Instead, derive a feed from the event log:

High-level algorithm:
1. Read the most recent events (bounded, newest-first).
2. Map each event to an `itemId` (straightforward for `item.*`; for `comment.add`, payload includes/points to the item).
3. Map `itemId -> outlineId` via the items table/state.
4. Include the event in the feed if the current actor follows:
   - the item’s `outlineId` (outline-follow), or
   - the specific `itemId` (item-follow), if supported.

Suggested feed row shape (for TUI/CLI output):
- `ts`
- `type` (event type)
- `actorId` (who caused it)
- `outlineId`
- `itemId`
- `title` (item title at time of rendering; best-effort)
- `summary` (human-readable short summary; derived from event type + payload)

### 3) Unread / read state (minimal)

Keep unread state simple:
- Store a per-actor “last seen” cursor:
  - e.g. `notifications_last_seen_event_id` or a timestamp.
- Unread = feed events newer than that cursor.
- “Mark all read” advances the cursor.

This avoids per-notification mutation and keeps the model local-first and low complexity.

## CLI/TUI surface (conceptual; not implemented)

- TUI: add a Notifications page/view that lists the derived feed, with filters and “mark all read”.
- CLI (optional): `clarity notifications list` / `--unread` for scriptable workflows, returning the stable JSON envelope.

## Open decisions (not resolved yet)

These are the main points to clarify before implementation:

1. **Follow granularity**:
   - outlines only?
   - also per-item follows?
   - does outline-follow automatically include all items (likely yes)?
2. **Unread semantics**:
   - unread since last open (global cursor), or
   - per-outline cursor, or
   - per-notification mark read/dismiss?
3. **Event coverage**:
   - only: create/status/comment (initial),
   - or also: assignment/priority/on-hold/move/reparent/archive?
4. **Actor scope**:
   - notifications scoped to the current actor identity, or
   - unified under the owning human user (agent + human share one inbox)?
5. **Performance bounds**:
   - default event window size for the feed (N events),
   - pagination behavior for CLI (limit/offset + `_hints`).

## Why this fits Clarity’s direction

- **Local-first**: everything is computed from local state and the local event log.
- **Small core model**: adds a minimal follow state; keeps notifications as a view over events.
- **Auditable**: the source of truth remains the append-only event log (already central to Clarity).
- **Progressive disclosure**: the feed can stay lightweight and link out to item detail/history, comments, etc.
