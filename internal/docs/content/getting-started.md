# Getting started (local-first)

Clarity is a local-first project management CLI + TUI built for humans and their agents.

## Minimal setup

```bash
clarity init
clarity identity create --kind human --name "andreas" --use
clarity projects create --name "Clarity" --use
clarity outlines create --project <project-id>
```

## Workspaces

```bash
clarity init
clarity workspace current
clarity workspace use <name>
```

Clarity is **workspace-first** and will use the implicit **`default`** workspace unless you explicitly select a different one.

For most users (and for agents), you should **stick to the default workspace** unless you were specifically instructed to use a different workspace.

### Registering Git-backed workspaces

If you have a Git repo directory that you want Clarity to treat as a workspace, register it:

```bash
clarity workspace add <name> --dir /path/to/repo --use
```

### Archiving (hiding) unused workspaces

Hide workspaces you don’t want to see in pickers/lists:

```bash
clarity workspace archive <name>
clarity workspace list --include-archived
clarity workspace unarchive <name>
```

### Deleting legacy workspaces (dangerous)

If you have legacy workspaces under `~/.clarity/workspaces/<name>` that you want to permanently remove:

```bash
clarity workspace delete <name> --yes
```

If a workspace with the same name is registered to a different directory (e.g. a Git repo),
use `--force-legacy` to delete only the legacy directory:

```bash
clarity workspace delete <name> --yes --force-legacy
```

### Migrating legacy SQLite event log workspaces

Clarity’s Git sync is built around the JSONL event log (`events/events*.jsonl`) as the canonical history.

If you have an older workspace created during the SQLite event log phase, migrate it into the Git-backed JSONL v1 layout:

```bash
clarity workspace migrate --from /path/to/old --to /path/to/new
clarity --dir /path/to/new reindex
clarity --dir /path/to/new doctor --fail
```

Optionally initialize a new repo and create the first commit:

```bash
clarity workspace migrate --from /path/to/old --to /path/to/new --git-init --git-commit --message "clarity: migrate"
```

Optionally register the migrated workspace (and make it current):

```bash
clarity workspace migrate --from /path/to/old --to /path/to/new --register --name "Team workspace" --use
```

## Projects (set context)

```bash
clarity projects list
clarity projects use <project-id>
clarity projects current
```

## Start tracking work

```bash
clarity items create --title "First item"
clarity items ready

# Include items that are on hold:
clarity items ready --include-on-hold

# Direct item lookup (shortcut for `clarity items show <item-id>`)
clarity <item-id>
```

## If you want an isolated store

```bash
## Use this for fixtures/tests or truly isolated experiments only.
clarity --dir ./.clarity init
clarity --dir ./.clarity identity create --kind human --name "me" --use
```

## Doctor + reindex (Git-backed workflows)

If your workspace uses JSONL events as the canonical history (e.g. `events/events*.jsonl`), these commands help keep the derived state healthy:

```bash
clarity doctor --fail
clarity reindex
```
