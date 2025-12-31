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

## `clarity sync pull`

Pulls remote changes using rebase.

This command refuses to run when:
- a merge/rebase is already in progress, or
- there are tracked local changes (untracked files are ignored).

Examples:

```bash
clarity sync pull
```

After pulling, rebuild derived state:

```bash
clarity reindex
clarity doctor --fail
```

## `clarity sync push`

Stages canonical workspace paths, commits, optionally pulls/rebases, then pushes.

If pushing fails due to a non-fast-forward update, Clarity retries once by pulling with `--rebase` and pushing again.

Examples:

```bash
clarity sync push
clarity sync push --message "clarity: weekly updates"
clarity sync push --pull=false
```

## `clarity sync resolve`

Shows conflict status and suggested resolution steps.

Clarity blocks writes while a Git merge/rebase is in progress.
