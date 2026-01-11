# Items + outlines

Core model:
- `workspace -> projects -> outlines -> items`
- Items are hierarchical within an outline (indent/outdent).

## Outlines are a flexible container (tasks, announcements, docs)

In Clarity V1 we intentionally keep the model small and consistent:
- Projects are the top-level grouping (and can represent a project or a team).
- Within a project you can create **multiple outlines**, named freely.
- An outline can represent tasks *or* other structured content (announcements, docs, notes), using the same `items` primitive.

Status is optional:
- Task-like outlines typically define `statusDefs` and use status cycling.
- Announcement/doc-like outlines often use **no status** on items (`--status none`).

We may later add explicit settings on projects/outlines that change semantics (notifications, default sorting/view, etc), while keeping the underlying entities stable.

## Ordering
Items have:
- `parentId` (hierarchy)
- `rank` (sibling ordering; lexicographic)

The rank is generated and managed by the system (you usually won’t touch it directly).

## Create an item

```bash
# Option A: explicit
clarity items create --project <project-id> --title "Write spec" --description "Markdown supported"

# Option B: set current project once
clarity projects use <project-id>
clarity items create --title "Write spec" --description "Markdown supported"
```

## Copy an item

```bash
# Copy within the same outline (inserted after the source item)
clarity items copy <item-id>

# Copy into a different project/outline
clarity items copy <item-id> --project <project-id> --outline <outline-id>
```

## Find the next item to work on

```bash
clarity items ready
```

By default, `items ready` excludes items that are **on hold**. To include them:

```bash
clarity items ready --include-on-hold
```

## Show an item

```bash
# Canonical
clarity items show <item-id>

# Alias
clarity items get <item-id>

# Direct lookup (beads-style convenience)
clarity <item-id>
```

## Event history

Clarity keeps an append-only event log (`events.jsonl`) which records changes over time.

```bash
# List events for an item (oldest-first)
clarity items events <item-id>

# Limit the number of events returned (0 = all)
clarity items events <item-id> --limit 50
```

## Short aliases (ergonomics)
The canonical mutation commands use `set-*` naming, and there are **short verb aliases**
for interactive use. These aliases are **additive**; scripts can keep using the canonical
commands unchanged.

```bash
clarity items title <item-id> --title "New title"              # alias for set-title
clarity items desc <item-id> --description "Markdown..."       # alias for set-description
clarity items status <item-id> --status doing                  # alias for set-status
clarity items priority <item-id> --on                          # alias for set-priority
clarity items on-hold <item-id> --on                           # alias for set-on-hold
clarity items due <item-id> --at 2025-12-31                    # alias for set-due
clarity items schedule <item-id> --at 2025-12-20T09:00:00Z     # alias for set-schedule
clarity items parent <item-id> --parent none                   # alias for set-parent
clarity items move-to-outline <item-id> --to <outline-id>      # alias for move-outline
```

## Reorder and reparent (CLI)
The CLI intentionally avoids `indent`/`outdent`. Use explicit operations:

```bash
# Reorder among siblings
clarity items move <item-id> --before <sibling-id>
clarity items move <item-id> --after <sibling-id>

# Reparent (and place at end by default)
clarity items set-parent <item-id> --parent <new-parent-id>
clarity items set-parent <item-id> --parent none
```

Note: Ordering is stored as per-sibling-set ranks. If ranks collide, these operations may locally rebalance ranks for a few nearby siblings to ensure the requested placement is always possible and stable.

## Assign an item

```bash
# Assign (transfers ownership to the assignee)
clarity items set-assign <item-id> --assignee <actor-id>

# Short alias
clarity items assign <item-id> --to <actor-id>

# Clear assignment (owner-only; does not transfer ownership)
clarity items set-assign <item-id> --clear
```

## Status
- Status definitions live on the outline.
- Items store a `status_id` (stable) but CLI accepts status **labels** too.
- Items can always have **no status**: `--status none`.

### Customize status values per outline
Each outline can define its own `statusDefs` (labels + which ones count as “end-state”).

```bash
# List statuses for an outline
clarity outlines status list <outline-id>

# Add a status (id is derived from label, e.g. "IN REVIEW" -> "in-review")
clarity outlines status add <outline-id> --label "IN REVIEW"

# Mark a status as an end-state (affects agenda filtering + completion semantics)
clarity outlines status update <outline-id> "IN REVIEW" --end

# Change the cycling/column order (must include all labels exactly once)
clarity outlines status reorder <outline-id> \
  --label "TODO" \
  --label "DOING" \
  --label "IN REVIEW" \
  --label "DONE"

# Remove a status (blocked if any item in the outline uses it)
clarity outlines status remove <outline-id> "IN REVIEW"
```
