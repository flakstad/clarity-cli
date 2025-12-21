# TUI

Run `clarity` with no subcommands to start the interactive TUI.

Current scope (early):
- Centered layout by default (projects/outlines/outline/item views)
- Breadcrumb at the top showing where you are (projects > project > outline > item)
- Navigate projects → outlines → items
- Optional preview pane for item details (outline on the left, preview on the right)
- Full-screen item view (single pane)
- Auto-refresh when the local store changes (polls file mtimes)
- Item details include a recent history section (from the local `events.jsonl` log)
- Outline shows progress cookies for items with children (e.g. `1/2`)
- Create items directly from the TUI (sibling and subitem)
- Reorder and restructure items (reorder, indent, outdent)
- On quit, remembers your last screen/selection (per workspace) and restores it on next launch

Key bindings:
- `x` / `?`: open the action panel (shows available commands for the current context)
- `esc` / `backspace`: go back within the action panel; from the root, closes the panel
- `ctrl+g`: close the action panel immediately
- `g`: open the Navigate menu (action panel)
- `a`: open *Agenda Commands* (then press `t` to list all TODO entries)
- `c`: open the Capture menu (action panel) (coming soon)
- `v`: toggle experimental column view (outline screen only)
- `/`: filter outline items (type to filter; `enter` applies; `esc` clears)
- `enter`: open selected item (single-pane item view)
- `o`: toggle preview pane (split view; auto-collapses on narrow terminals `<80` cols)
- `backspace` or `esc`: go back (from item view → outline; from outline → previous screen)
- `r`: archive selected item / outline / project (with confirm; depends on screen)
- `y`: copy selected item ID to clipboard
- `Y`: copy `clarity items show <id>` to clipboard
- `C`: add a comment (selected item)
- `w`: add a worklog entry (selected item)
- `q` or `ctrl+c`: quit
- `tab`: toggle focus (outline/preview) (only when preview is visible)

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
- `shift+↑/↓`: move item up/down among siblings (fallback; works in macOS Terminal.app where ctrl+↑/↓ may not get delivered)
- `ctrl+k/j`: move item up/down among siblings (fallback; works in macOS Terminal.app where `shift+↑/↓` is indistinguishable from plain arrows)

Editing:
- `e`: edit title of the selected item (Enter saves, Esc cancels)
- `Shift+D` (`D`): edit description of the selected item (multiline Markdown; `ctrl+s` saves)
- `e` (on outlines screen): rename selected outline (Enter saves, Esc cancels)
- `e` (on projects screen): rename selected project (Enter saves, Esc cancels)

Status:
- `space`: open status picker for selected item (includes `(no status)`)
- `Shift+←/→`: cycle status backward/forward (includes `(no status)`)
  - `(no status)` renders as empty (no placeholder)

Creating items:
- `n`: create a new sibling after the selected item (outline pane)
- `N`: create a new subitem under the selected item (either pane)
- `+ Add item` row: select it and press `enter` to add an item (handy for empty outlines)

Creating projects/outlines:
- `n` (on projects screen): create a new project
- `n` (on outlines screen): create a new outline (name optional)

Notes:
- The TUI still leans on the CLI for some features (for example: tags, due/schedule, advanced queries).
- While resizing your terminal window, Clarity may briefly show a `Resizing…` overlay to avoid transient layout artifacts.

Theme detection:
- Clarity uses Lip Gloss “adaptive colors” to support both light and dark terminals.
- If your terminal reports the wrong background (e.g. dialogs look dark on a light theme), you can override:
  - `CLARITY_TUI_THEME=light` (or `dark` / `auto`)
  - or `CLARITY_TUI_DARKBG=false` (or `true`)
- Note: some terminals (and setups like tmux) may not reliably report background. If iTerm is showing dark/black surfaces but you’re on a light theme, set `CLARITY_TUI_THEME=light` (or `CLARITY_TUI_DARKBG=false`) in your shell profile for that terminal.

Comment/worklog/description editor:
- `ctrl+s`: save
- `tab` / `shift+tab`: focus body/save/cancel, `enter` activates buttons
