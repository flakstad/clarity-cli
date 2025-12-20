# TUI

Run `clarity` with no subcommands to start the interactive TUI.

Current scope (early):
- Navigate projects → outlines → items
- Read item details in a split view (outline on the left, details on the right)
- Auto-refresh when the local store changes (polls file mtimes)
- Manual reload state from disk (so changes made via CLI show up immediately)
- Create items directly from the TUI (sibling and subitem)
- Reorder and restructure items (reorder, indent, outdent)

Key bindings:
- `enter`: select / drill down
- `backspace` or `esc`: go back
- `r`: reload from disk
- `q` or `ctrl+c`: quit
- `tab`: toggle focus (outline/detail)

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

Creating items:
- `n`: create a new sibling after the selected item (outline pane)
- `N`: create a new subitem under the selected item (either pane)
- `+ Add item` row: select it and press `enter` to add an item (handy for empty outlines)

Notes:
- The TUI still leans on the CLI for some features (for example: comments/worklog input, tags, due/schedule).
