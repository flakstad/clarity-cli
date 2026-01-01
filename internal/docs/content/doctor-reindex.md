# Doctor + reindex

These commands are primarily for the **Git-backed JSONL event log** workflow:

- Canonical history lives in `events/events*.jsonl`
- Local SQLite state is derived and rebuildable

## `clarity doctor`

Validates the workspace event logs and basic invariants.

Checks include:
- Git in-progress merge/rebase (writes are blocked until clean)
- Merge markers (`<<<<<<<`, `=======`, `>>>>>>>`) inside `events/*.jsonl`
- Malformed JSON lines
- Replica/workspace id mismatches (vs shard filename + `meta/workspace.json`)
- Duplicate events
- Parent integrity (missing parents, cross-entity parents, self-parent)
- Fork detection (multiple heads for the same entity stream)

Examples:

```bash
clarity doctor
clarity doctor --fail
```

`--fail` exits non-zero if errors are found (useful in CI).

### Forks

`fork_detected` means there are **multiple heads** for the same entity stream (typically caused by two replicas editing the same entity concurrently and later merging in Git).

V1 behavior:
- Clarity continues to allow reads.
- Clarity blocks writes while a Git merge/rebase is in progress; and some operations may also fail when forks are present.

Resolution strategy in V1 is manual: decide what should “win” and edit the event logs accordingly, then run `clarity reindex` and `clarity doctor --fail`.

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
