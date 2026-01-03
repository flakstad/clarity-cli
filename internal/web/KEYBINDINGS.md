# Web Keybindings Strategy (V1)

Goal: make the web UI feel like the TUI: fast, predictable, and keyboard-first, while keeping the app fully usable with a mouse and without JavaScript.

## Principles

- **Roving focus**: lists expose focusable “rows” (`tabindex="0"`, `data-kb-item`, `data-focus-id`), and keybindings operate on the currently focused row.
- **Do not steal standard browser keys**: avoid binding `Tab`/`Shift+Tab`, and avoid overriding typing behavior inside inputs/textarea.
- **Context first**: the same key can mean different things depending on focus (e.g. `j/k` inside an outline vs. a generic list).
- **TUI parity first**: prefer the same keys and panel structure as the Bubble Tea TUI, even when we could do something more “webby”.
- **Optimistic UI for “cursor/movement” interactions**: reorder/indent/outdent mutate the DOM immediately, then persist async (debounced) to reduce git churn.
- **Server remains source of truth**: SSE + Datastar keep pages convergent; optimistic UI should converge or show an error.

## Implementation pattern

All key handling is routed through a single `document.addEventListener('keydown', ...)` handler in `internal/web/static/app.js`:

- Highest priority: modal dialogs (e.g. status picker) consume keys while open.
- Next: ignore keys while typing in form controls (`input`, `textarea`, `select`).
- Next: page/context-specific shortcuts based on focus/DOM markers:
  - native outline rows: `data-outline-row`
  - outline component: `#outline` / `<clarity-outline>`
  - generic lists: `data-kb-item`

## Adding new shortcuts

1. Prefer a **context marker** (`data-*`) over checking URLs or brittle DOM structure.
2. Keep the shortcut consistent with the TUI when possible.
3. Only call `preventDefault()` when the shortcut is actually handled.
4. When adding a mutation shortcut, decide:
   - optimistic + async persist (good for move/reorder)
   - synchronous persist (good for destructive actions)
5. Add/adjust the on-page hint text and/or `?` help overlay.

## Global bindings (today)

- `x` / `?` open **Actions** (action panel root)
- `g` open **Go to** (action panel: nav)
- `a` open **Agenda Commands** (action panel: agenda) — except in outline/item where `a` is assignment
- `A` open **Agenda Commands** (always available; useful when `a` is assignment)
- `c` open **Capture** (modal; TUI parity is action-panel capture, but web currently uses a modal)

## Projects/outlines lists

- List navigation: `j/k`, `↑/↓`, `Ctrl+N/P` move focus; `Enter` opens focused row
- Projects (`/projects`): `n` new project, `e` rename focused, `r` archive focused
- Outlines (`/projects/:id`): `n` new outline, `e` rename focused, `D` edit description, `r` archive focused

## Outline view (native)

- `v` cycle view mode: `list` → `list+preview` → `columns`
- `h/l` or `←/→` or `Ctrl+B/F` navigate parent/child

## Item view

- `e` edit title, `D` edit description
- `Space` change status, `a` assign, `t` tags
- `d/s` due/schedule, `p/o` priority/on-hold
- `C` comment, `w` worklog, `m` move outline, `r` archive
- `y/Y` copy ref / copy show command

## Outline statuses (web)

- Open from action panel: `x` then `S` (or `x` then `S` from outlines list)
- Modal keys: `a` add, `r` rename, `e` toggle end-state, `n` toggle requires-note, `d` delete, `Ctrl+J/K` reorder, `Esc` close

## Agenda view

- `j/k`, `↑/↓`, `Ctrl+N/P` move focus; `Enter` opens item
- `/` focuses the filter input
- `z/Z` collapse selected / collapse-all (local, persisted)
- `h/l` or `←/→` or `Ctrl+B/F` navigate parent/child
- Owner-only mutations: `e` edit title, `Space` change status, `r` archive

## Action panel (web)

The web action panel mirrors the TUI “stack” behavior:

- `Esc` / `Backspace` pops to previous panel; at root, closes
- `Ctrl+G` closes immediately
- `j/k` or `↑/↓` or `Ctrl+N/P` navigates entries
- Pressing an entry’s key executes it (no Enter required)
