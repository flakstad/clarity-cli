# Git sync

Clarity v1 workspaces can be **Git-backed**:

- Canonical history: `events/events*.jsonl` (committed)
- Derived state: `.clarity/index.sqlite` (local, rebuildable)

## `clarity sync status`

Shows the Git working tree status for the current workspace directory.

Examples:

```bash
clarity sync status
clarity --dir /path/to/workspace sync status
```

Use this when:
- debugging why writes are blocked (future behavior), or
- checking ahead/behind status before/after a pull.
