# Backup + restore (portable export/import)

Clarity is **local-first** and stores workspace data in a local SQLite database (`clarity.sqlite`).
For peace of mind (and for moving between machines), Clarity supports a **portable backup format**
that is easy to store elsewhere (cloud drive, external disk, git, etc).

## Export a backup

Exports two files into a directory:
- `state.json`: current materialized workspace state (projects/outlines/items/comments/worklog/etc)
- `events.jsonl`: append-only event log (useful for future sync + forensics)

```bash
# Export the current workspace to a folder
clarity workspace export --to /path/to/backup-dir

# Export without events (state only)
clarity workspace export --to /path/to/backup-dir --events=false
```

## Import a backup

Imports `state.json` (and `events.jsonl` if present) into a **new workspace**:

```bash
# Import into a new workspace called "restored"
clarity workspace import restored --from /path/to/backup-dir

# Import using --name (handy for scripts)
clarity workspace import --name restored --from /path/to/backup-dir

# Replace an existing workspace (DANGEROUS)
clarity workspace import restored --from /path/to/backup-dir --force

# Import and immediately make it current
clarity workspace import restored --from /path/to/backup-dir --use
```

## Notes + caveats

- The exported files are designed for backup/restore and inspection; they are **not** meant to be edited by hand.
- Git works much better with these exported text files than with raw SQLite binaries.
- If you store backups in a cloud-synced folder, treat it as **backup**, not a multi-writer sync system.
