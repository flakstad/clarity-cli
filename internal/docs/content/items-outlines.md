# Items + outlines

Core model:
- `workspace -> projects -> outlines -> items`
- Items are hierarchical within an outline (indent/outdent).

## Create an item

```bash
# Option A: explicit
clarity items create --project <project-id> --title "Write spec" --description "Markdown supported"

# Option B: set current project once
clarity projects use <project-id>
clarity items create --title "Write spec" --description "Markdown supported"
```

## Status
- Status definitions live on the outline.
- Items store a `status_id` (stable) but CLI accepts status **labels** too.
- Items can always have **no status**: `--status none`.
