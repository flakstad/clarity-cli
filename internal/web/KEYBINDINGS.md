# Web Keybindings Strategy (V1)

Goal: make the web UI feel like the TUI: fast, predictable, and keyboard-first, while keeping the app fully usable with a mouse and without JavaScript.

## Principles

- **Roving focus**: lists expose focusable “rows” (`tabindex="0"`, `data-kb-item`, `data-focus-id`), and keybindings operate on the currently focused row.
- **Do not steal standard browser keys**: avoid binding `Tab`/`Shift+Tab`, and avoid overriding typing behavior inside inputs/textarea.
- **Context first**: the same key can mean different things depending on focus (e.g. `j/k` inside an outline vs. a generic list).
- **Global bindings are small and consistent**:
  - `?` = help overlay
  - `g h/p/a/s` = navigation “go to”
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
