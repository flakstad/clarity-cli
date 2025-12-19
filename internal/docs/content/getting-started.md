# Getting started (local-first)

Clarity is a local-first project management CLI + TUI built for humans and their agents.

## Minimal setup

```bash
clarity init
clarity identity create --kind human --name "andreas" --use
clarity projects create --name "Clarity"
clarity outlines create --project <project-id>
```

## Start tracking work

```bash
clarity items create --project <project-id> --title "First item"
clarity items list --project <project-id>
```

## If you want an isolated store

```bash
clarity --dir ./.clarity init
clarity --dir ./.clarity identity create --kind human --name "me" --use
```
