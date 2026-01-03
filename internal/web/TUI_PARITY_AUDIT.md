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

### Workspaces list (web: `/workspaces`)

| Key | TUI | Web | Status |
|---|---|---|---|
| `j/k` `↑/↓` `Ctrl+N/P` | Move selection | Move focus | ✅ |
| `Enter` | Switch workspace | Switch workspace | ✅ |
| `n` | New workspace | New workspace prompt | ✅ |
| `r` | Rename workspace | Rename focused workspace prompt | ✅ |

## Native outline view

The native outline web mode aims to match the TUI’s outline bindings (move/reorder, indent/outdent, status cycling, collapse, etc.).

Implemented:
- `h/l`, `←/→`, `Ctrl+B/F` navigate parent/child (TUI parity)
- `z/Z` collapse per-item / collapse-all (local, persisted per outline)
- `Alt+H/L` indent/outdent (macOS option key parity)

Known gaps are tracked in the Phase 7 epic items (web parity).

## Action panel entries (context menus)

The goal is: the web action panel should expose the same actions as the TUI for the current view/context, even if the view also has direct keyboard shortcuts.

### Outline view action panel (`/outlines/:id`)

| Key | TUI label | Web label | Status |
|---|---|---|---|
| `enter` | Open item | Open item | ✅ |
| `v` | Cycle view mode | Cycle view mode | ✅ |
| `O` | Outline… | Outline… | ✅ |
| `S` | Edit outline statuses… | Edit outline statuses… | ✅ |
| `z/Z` | Collapse | Toggle collapse / Collapse/expand all | ✅ |
| `y/Y` | Copy ref / show cmd | Copy item ref / Copy show command | ✅ |
| `C/w` | Add comment / worklog | Add comment / Add worklog | ✅ |
| `p/o` | Priority / on hold | Toggle priority / Toggle on hold | ✅ |
| `u` | Assign… | Assign… | ✅ |
| `t` | Tags… | Tags… | ✅ |
| `d/s` | Due / schedule | Set due / Set schedule | ✅ |
| `D` | Edit description | Edit description | ✅ |
| `r` | Archive item | Archive item | ✅ |
| `m` | Move to outline… | Move to outline… | ✅ |

### Item view action panel (`/items/:id`)

| Key | TUI label | Web label | Status |
|---|---|---|---|
| `e` | Edit title | Edit title | ✅ |
| `D` | Edit description | Edit description | ✅ |
| `p/o` | Priority / on hold | Toggle priority / Toggle on hold | ✅ |
| `u` | Assign… | Assign… | ✅ |
| `t` | Tags… | Tags… | ✅ |
| `d/s` | Due / schedule | Set due / Set schedule | ✅ |
| `Space` | Change status | Change status | ✅ |
| `C/w` | Add comment / worklog | Add comment / Add worklog | ✅ |
| `y/Y` | Copy ref / show cmd | Copy item ref / Copy show command | ✅ |
| `m` | Move to outline… | Move to outline… | ✅ |
| `r` | Archive item | Archive item | ✅ |

## Item side panels

| Behavior | TUI | Web | Status |
|---|---|---|---|
| Open side pane | focus “Comments/Worklog/History” row + Enter | focus “Related” row + Enter | ✅ |
| Reply to comment | `R` (reply) | `R` (reply) | ✅ |

## Agenda commands panel

The web currently exposes only a minimal subset of the TUI’s “Agenda Commands…”.

| Key | TUI entry | Web entry | Status |
|---|---|---|---|
| `t` | List all TODO entries | List all TODO entries | ✅ |

## Dialog parity

| Behavior | TUI | Web | Status |
|---|---|---|---|
| Consistent buttons/labels | same across dialogs | varies | ⚠️ |
| Consistent keys | Esc/ctrl+g cancel; ctrl+enter save | varies | ⚠️ |
| Restore focus after close | always restore selection | restores to `data-focus-id` when available | ✅ |
