# TUI

Run `clarity` with no subcommands to start the interactive TUI.

Current scope (early):
- Navigate projects → outlines → items
- Read item details in a split view (outline on the left, details on the right)
- Auto-refresh when the local store changes (polls file mtimes)
- Outline shows progress cookies for items with children (e.g. `[1/2]`)
- Create items directly from the TUI (sibling and subitem)
- Reorder and restructure items (reorder, indent, outdent)

Key bindings:
- `enter`: open selected item (outline view)
- `o`: open selected item (outline view)
- `backspace` or `esc`: go back (from detail → outline; from outline → previous screen)
- `r`: archive selected item (with confirm)
- `c`: add a comment (selected item)
- `w`: add a worklog entry (selected item)
- `q` or `ctrl+c`: quit
- `tab`: toggle focus (outline/detail) (optional)

Outline navigation (outline.js-style):
- `↑/↓`, `j/k`, `ctrl+n/ctrl+p`: previous/next visible item
- `→`, `l`, `ctrl+f`: go into first child (expands if collapsed)
- `←`, `h`, `ctrl+b`: go to parent
- `z`: toggle collapse for selected item
- `Shift+Z`: collapse all ↔ expand all

Outline movement (hold Alt):
- `alt+↑/↓` (or `alt+k/j`, `alt+p/n`): move item up/down among siblings
- `alt+→` (or `alt+l/f`): indent (become child of previous sibling)
- `alt+←` (or `alt+h/b`): outdent (become sibling after parent)

Editing:
- `e`: edit title of the selected item (Enter saves, Esc cancels)

Status:
- `space`: open status picker for selected item (includes `(no status)`)
- `Shift+←/→`: cycle status backward/forward (includes `(no status)`)
  - `(no status)` renders as empty (no placeholder)

Creating items:
- `n`: create a new sibling after the selected item (outline pane)
- `N`: create a new subitem under the selected item (either pane)
- `+ Add item` row: select it and press `enter` to add an item (handy for empty outlines)

Notes:
- The TUI still leans on the CLI for some features (for example: tags, due/schedule, advanced queries).

Comment/worklog editor:
- `ctrl+s`: save
- `tab` / `shift+tab`: focus body/save/cancel, `enter` activates buttons
