# Clarity CLI (local-first)

This is a Go CLI + Bubble Tea TUI for Clarity V1: items + dependencies + projects, with strict attribution (actors) and lightweight communication.

## Model: projects + outlines (one flexible container)

In Clarity V1 we keep the grouping entity named **Projects**.

Within a project, **Outlines** are the primary container you add and name freely:
- Task outlines (with statuses like TODO/DOING/DONE)
- Announcement / message outlines (often with no statuses)
- Docs / notes outlines (often with no statuses)

We may later introduce explicit settings on projects/outlines to change semantics (notifications, default sorting, etc), but the **data model stays centered on outlines + items**.

## Install / build

```bash
make install # runs unit tests first
```

Or:

```bash
go run ./cmd/clarity --help
```

See also the docs surface:

```bash
clarity docs
clarity docs getting-started
clarity docs backup
clarity items --help
```

## Quickstart

```bash
clarity init
clarity identity create --name "andreas" --kind human --use
clarity projects create --name "Clarity" --use
clarity projects list
```

Create an item:

```bash
clarity items create --title "First item"
clarity items list --project proj-1
```

Run the TUI:

```bash
clarity
```

## Backup / restore (portable export/import)

Clarity stores data locally in SQLite, but you can export a portable backup as text files for safekeeping.

```bash
# Export a backup (writes state.json + events.jsonl)
clarity workspace export --to /path/to/backup-dir

# Restore into a new workspace
clarity workspace import --name restored --from /path/to/backup-dir --use
```

For details: `clarity docs backup`.

## Ordering model
Items are ordered by:
- `parentId` (hierarchy)
- `rank` (lexicographic sibling ordering)

For CLI reordering/reparenting:
- `clarity items move ...`
- `clarity items set-parent ...`
