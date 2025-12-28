# Direction (Clarity V1)

Clarity is a **project management + communication** tool built for humans and their agents.

It is not the intention to recreate Beads feature-for-feature. Beads is useful as a reference for
agent ergonomics and operational UX.

## Mental model

Think **Org Mode + Basecamp**:
- **Outline-native work**: hierarchical items; capture first, organize later.
- **Strong communication**: comments for collaboration, worklog for private execution notes.
- **Replay/history**: decisions and changes are inspectable over time.
- **Calm by default**: avoid notification-driven anxiety; prefer intentional review.

## Interfaces

- **Humans** primarily use the **TUI** (and later a web GUI).
- **Agents** primarily use the **CLI**, with a stable output contract (`data` / `meta` / `_hints`).

## Agenda (core power feature)

The Agenda (Org-style) is expected to become the main navigation and query surface for both humans and agents:
- Cross-project filtering/search over items.
- Views based on status/ownership/assignment, due/schedule, tags, dependencies, and communication signals.
- Saved views (e.g. Today / Next / Waiting / Stuck / Recently discussed).
