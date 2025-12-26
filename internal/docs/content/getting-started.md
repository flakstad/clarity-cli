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
