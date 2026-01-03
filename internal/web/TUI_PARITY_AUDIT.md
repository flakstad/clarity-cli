# TUI ↔ Web Parity Audit (Keybindings + Panels)

This document is the running “source of truth” for keyboard parity between the Bubble Tea TUI and the Datastar-backed web UI.

## Global keys

| Key | TUI meaning | Web meaning | Status |
|---|---|---|---|
| `x` / `?` | Open action panel (context) | Open action panel (context) | ✅ |
| `g` | Open “Go to” panel | Open “Go to” panel | ✅ |
| `a` | Open “Agenda Commands” (except in outline/item where `a` is assign) | Same | ✅ |
| `A` | Open “Agenda Commands” (always) | Same | ✅ |
| `c` | Open “Capture” panel | Opens capture modal | ⚠️ (different UI, same intent) |

## Action panel behavior

| Behavior | TUI | Web | Status |
|---|---|---|---|
| Stack navigation | `Esc`/`Backspace` pops; root closes | Same | ✅ |
| Close immediately | `Ctrl+G` | `Ctrl+G` | ✅ |
| Selection | `j/k`, arrows, `Ctrl+N/P`, Tab | `j/k`, arrows, `Ctrl+N/P` | ⚠️ (Tab intentionally not stolen) |
| Typed selection | press the entry key | same | ✅ |

## Go to panel entries

| Key | TUI entry | Web entry | Status |
|---|---|---|---|
| `p` | Projects | Projects | ✅ |
| `s` | Sync… (submenu) | Sync… (submenu) | ✅ |
| `j` | Jump to item by id… | Jump to item by id… | ✅ |
| `o` | Outlines (current project) | Outlines (current project) | ✅ (when project context exists) |
| `l` | Outline (current) | Outline (current) | ✅ (when outline context exists) |
| `i` | Item (open) | Item (open) | ✅ (only on item page) |
| `A` | Archived | Archived | ✅ |
| `W` | Workspaces… | Workspaces… | ✅ |
| `1..5` | Recent items | Recent items | ✅ |

## Lists (projects, outlines, agenda rows)

| Key | TUI | Web | Status |
|---|---|---|---|
| `j/k` | Move selection | Move focus | ✅ |
| `↑/↓` | Move selection | Move focus | ✅ |
| `Ctrl+N/P` | Move selection | Move focus | ✅ |
| `Enter` | Open | Open | ✅ |

### Projects list (web: `/projects`)

| Key | TUI | Web | Status |
|---|---|---|---|
| `n` | New project | New project prompt | ✅ |
| `e` | Rename project | Rename focused project prompt | ✅ |
| `r` | Archive project | Archive focused project | ✅ |

### Outlines list (web: `/projects/:id`)

| Key | TUI | Web | Status |
|---|---|---|---|
| `n` | New outline | New outline prompt | ✅ |
| `e` | Rename outline | Rename focused outline prompt | ✅ |
| `D` | Edit outline description | Edit focused outline description prompt | ✅ |
| `r` | Archive outline | Archive focused outline | ✅ |
| `S` | Edit outline statuses | Outline statuses modal | ✅ |

## Native outline view

The native outline web mode aims to match the TUI’s outline bindings (move/reorder, indent/outdent, status cycling, collapse, etc.).

Implemented:
- `h/l`, `←/→`, `Ctrl+B/F` navigate parent/child (TUI parity)
- `z/Z` collapse per-item / collapse-all (local, persisted per outline)
- `Alt+H/L` indent/outdent (macOS option key parity)

Known gaps are tracked in the Phase 7 epic items (web parity).
