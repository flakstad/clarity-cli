# Clarity CLI (local-first)

This is a Go CLI + Bubble Tea TUI for Clarity V1: items + dependencies + projects, with strict attribution (actors) and lightweight communication.

## Install / build

```bash
make install
```

Or:

```bash
go run ./cmd/clarity --help
```

See also the docs surface:

```bash
clarity docs
clarity docs getting-started
clarity items --help
```

## Quickstart

```bash
clarity init
clarity identity create --name "andreas" --kind human --use
clarity projects create --name "Clarity"
clarity projects list
```

Create an item:

```bash
clarity items create --project proj-1 --title "First item"
clarity items list --project proj-1
```

Run the TUI:

```bash
clarity
```
