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
clarity workspace current
clarity workspace use <name>
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

# Direct item lookup (shortcut for `clarity items show <item-id>`)
clarity <item-id>
```

## If you want an isolated store

```bash
clarity --dir ./.clarity init
clarity --dir ./.clarity identity create --kind human --name "me" --use
```
