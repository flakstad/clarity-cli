# Outline as document (design notes — not implemented)

This doc captures the **planned** work for `item-77v`: make a Clarity outline feel closer to an org/markdown document.

## Goals

- **Outline-level description**: an outline has a top-level markdown description, shown above items.
- **Document mode**: the outline view can show the **focused item’s** full markdown description inline (directly under the item).
- **Keybinding-first UX**: outline-level editing should avoid conflicts with item bindings.
- **Local-first persistence**: view preferences should be stored in **local TUI state**, not on the outline entity itself.
- **Import/export planning**: document the planned markdown import/export contract (but do not implement yet).

## Current behavior (verified)

### View mode persistence is local (not stored on the outline)

The TUI stores UI state in a workspace-scoped file `tui_state.json`, including:

- `OutlineViewMode` (per-outline map)
- `ShowPreview`

This is persisted via `internal/store/tui_state.go` and built from the TUI model snapshot in `internal/tui/app.go` (`snapshotTUIState` / `applySavedTUIState`).

Implication: adding new view modes (e.g. `document`) can remain **user-local** and **workspace-local** without changing store/schema.

## Planned TUI UX

### View modes and cycling

We want `v` to be the single place to change “how I’m looking at this outline”, and include what is currently toggled by `o` (split preview), so `o` can be freed for another command.

**Plan**

- `v` cycles a per-outline view mode with **four explicit modes**:
  - `list`
  - `list+preview` (split view)
  - `document`
  - `columns` (existing; “kanban”)

Notes:

- Today `v` toggles `list ↔ columns` and forces `showPreview=false` when entering columns.
- The plan is to expand this into a 4-mode cycle and persist it per-outline in `tui_state.json`.

### Outline description (top-level)

- Outline description is shown **above the first item** in the outline view **regardless of view mode**.
- It is rendered using the same markdown renderer as item descriptions (Glamour).

Columns mode nuance:

- In `columns` view, show the outline description as a **single line under the title** above the columns.
- If the description is long, **truncate** it to fit available width.

Editing:

- From **outline screen** (`viewOutline`): use an outline prefix key:
  - `O d` = “edit outline description” (markdown textarea modal)
  - Also surface this in the action panel (`x` / `?`) for discovery.
- From **outlines list screen** (`viewOutlines`): direct binding:
  - `D` = “edit selected outline description”
  - Also surface in the action panel (`x` / `?`).

### Document mode: inline item description (focused item only)

In document mode:

- Items render as normal outline rows.
- Additionally, the **focused item’s** full markdown description renders **directly below** the focused row.
- If the focused item is **collapsed**, its inline description block is **hidden**.

Implementation strategy (high-level):

- Because list rows are fixed height, inline markdown is represented as additional list rows inserted after the focused item.
- Render markdown with `renderMarkdown(md, width)` and split by newline to produce N rows.
- Cache by `(itemID, item.UpdatedAt, width, mdStyle)` to avoid re-rendering on every selection change.

### Keybinding collisions (why we use prefix keys)

`viewOutline` already binds many single keys to item actions (`e`, `D`, `p`, `n/N`, `m`, `space`, `z/Z`, etc).

Adding outline actions directly would create conflicts, so we use:

- **Prefix key**: `O` for outline-level actions from the outline screen.
- **Direct keys** on the outlines list screen where there are fewer bindings.

## Planned markdown export/import (do not implement yet)

### Export: outline → markdown

Shape:

- `# <outline name>`
- `<outline description markdown>` (body under the H1)
- Items become headings:
  - top-level items: `## <item title>`
  - nested items: `###`, `####`, … based on depth
  - depths beyond `######` are clamped at `######` (and still nested logically by parse rules; exact encoding TBD)
- Each item’s description is markdown body under its heading.

Status:

- Potential encoding (to be confirmed later): `## TODO Title` where the first token is a status label/id.

### Import: markdown → new outline (no sync)

- Import always creates a **new** outline.
- Parse heading levels into hierarchy.
- Outline description is the body under `# ...` up to the first `##`.
- Item description is the body under the item’s heading up to the next heading of same-or-higher level.

## Open decisions (explicitly deferred)

- Outline/item **properties** (key/value) UI and export/import encoding.
- Exact “preview in cycle” representation: whether preview is a distinct mode vs a sub-state of list mode.
