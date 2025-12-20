# Items + outlines

Core model:
- `workspace -> projects -> outlines -> items`
- Items are hierarchical within an outline (indent/outdent).

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

## Status
- Status definitions live on the outline.
- Items store a `status_id` (stable) but CLI accepts status **labels** too.
- Items can always have **no status**: `--status none`.
