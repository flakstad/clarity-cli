# TUI

Run `clarity` with no subcommands to start the interactive TUI.

Current scope (early):
- Centered layout by default (projects/outlines/outline/item views)
- Breadcrumb at the top showing where you are (projects > project > outline > item)
- Go to projects → outlines → items
- Optional preview pane for item details (outline on the left, preview on the right)
- Full-screen item view (single pane)
- Auto-refresh when the local store changes (polls file mtimes)
- Outline shows progress cookies for items with children (e.g. `1/2`)
- Create items directly from the TUI (sibling and subitem)
- Reorder and restructure items (reorder, indent, outdent)

Key bindings:
- `enter`: open selected item (single-pane item view)
- `v`: cycle outline view mode (`list` → `list+preview` → `columns`)
- `O`: open outline actions menu (from outline screen; includes rename + description)
- `D` (on outlines screen): edit selected outline description
- `backspace` or `esc`: go back (from item view → outline; from outline → previous screen)
- `x` (or `?`): open the actions menu (context-aware; includes item actions from outline focus, details pane, and item view)
- `r`: archive selected item (with confirm)
- `m`: move selected/open item to another outline (can be in a different project)
- `y`: copy selected item ID to clipboard
- `Y`: copy `clarity items show <id>` to clipboard
- `c`: add a comment (selected item)
- `w`: add a worklog entry (selected item)
- `o`: toggle on-hold (selected/open item)
- `d`: set/clear due date (selected/open item)
- `s`: set/clear schedule date (selected/open item)
- `g`: open the Go to menu (shows available destinations, including `j` jump-to-item)
  - `j`: jump to an item by id (accepts `item-vth` or just `vth`)
  - `A`: archived (browse archived content; items open read-only)
- `q` or `ctrl+c`: quit
- `tab`: toggle focus (outline/preview) (optional; only when preview is visible)

Item view:
- `tab` / `shift+tab`: cycle focus across fields (title/status/priority/description/children/…)
- When **Children** is focused: `↑/↓` selects a child; `enter` opens the selected child

Due/schedule modal:
- Date is required (`YYYY-MM-DD`), time is optional (`HH:MM`)
- Focus is shown per field (`YYYY`, `MM`, `DD`, `HH`, `MM`)
- `tab` / `shift+tab`: change focus
- `enter` / `ctrl+s`: save
- `ctrl+c`: clear
- `h` / `l` (or `←` / `→`): previous/next field
- `j` / `k` (or `↓` / `↑`): decrement/increment the focused field
- `t` (or space on the toggle): enable/disable time fields
- `esc` / `ctrl+g`: cancel

Outline navigation (outline.js-style):
- `↑/↓`, `j/k`, `ctrl+n/ctrl+p`: previous/next visible item
- `→`, `l`, `ctrl+f`: go into first child (expands if collapsed)
- `←`, `h`, `ctrl+b`: go to parent
- `g`, `home`, `<`: go to start
- `G`, `end`, `>`: go to end
- `z`: toggle collapse for selected item
- `Shift+Z`: collapse all ↔ expand all

Outline movement (hold Alt):
- `alt+↑/↓` (or `alt+k/j`, `alt+p/n`): move item up/down among siblings
- `alt+→` (or `alt+l/f`): indent (become child of previous sibling)
- `alt+←` (or `alt+h/b`): outdent (become sibling after parent)

Note: Reordering always moves exactly one slot. If sibling ranks collide, Clarity may locally rebalance ranks for a few adjacent items to keep ordering stable (no janky jumps).

Editing:
- `e`: edit title of the selected item (Enter or Ctrl+S saves, Esc or Ctrl+G cancels)
- `e` (on outlines screen): rename selected outline (Enter or Ctrl+S saves, Esc or Ctrl+G cancels)

Status:
- `space`: open status picker for selected item (includes `(no status)`)
- `Shift+←/→`: cycle status backward/forward (includes `(no status)`)
  - `(no status)` renders as empty (no placeholder)
- If a status requires a note, Clarity prompts for the note before applying the change.

Creating items:
- `n`: create a new sibling after the selected item (outline pane)
- `N`: create a new subitem under the selected item (either pane)
- `+ Add item` row: select it and press `enter` to add an item (handy for empty outlines)

Notes:
- The TUI still leans on the CLI for some features (for example: tags, due/schedule, advanced queries).

Comment/worklog editor:
- `ctrl+s`: save
- `ctrl+o`: open in `$VISUAL`/`$EDITOR`
- `ctrl+g`: close (cancel)
- `tab` / `shift+tab`: focus body/save/cancel, `enter` activates buttons

## Git auto-commit (experimental)

For Git-backed workspaces, the TUI can optionally stage+commit canonical workspace changes after you stop editing for a short while (debounced).

Enable with:
- `CLARITY_AUTOCOMMIT=1` (or `CLARITY_GIT_AUTOCOMMIT=1`)

Notes:
- Commits include canonical paths only (`events/`, `meta/workspace.json`, `resources/`).
- This does not push to a remote; use `clarity sync push` (or `git push`) for that.
