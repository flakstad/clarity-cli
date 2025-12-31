# Doctor + reindex

These commands are primarily for the **Git-backed JSONL event log** workflow:

- Canonical history lives in `events/events*.jsonl`
- Local SQLite state is derived and rebuildable

## `clarity doctor`

Validates the workspace event logs and basic invariants.

Examples:

```bash
clarity doctor
clarity doctor --fail
```

`--fail` exits non-zero if errors are found (useful in CI).

## `clarity reindex`

Rebuilds the derived local SQLite state from the JSONL event logs.

Examples:

```bash
clarity reindex
```

Use this after:
- pulling new commits from Git,
- resolving merge conflicts, or
- manual edits to `events/` (not recommended, but supported).
