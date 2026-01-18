# Keybindings (TUI)

This document is the **reference + working contract** for Clarity’s TUI keybindings.
It aims to keep bindings:

- **Discoverable** (a small set of “openers” + dispatch menus)
- **Consistent** across views (outline, item, agenda)
- **Low-collision** (view-scoped direct keys; very few true globals)

If this document and the implementation ever disagree, the implementation wins; treat the mismatch as a bug and update one or the other.

## Model: scopes

Clarity has three binding scopes:

- **Global**: works from most places (when not in a modal / not typing into a text input).
- **View-local**: works only in a specific view (outline vs item vs agenda, etc).
- **Modal/component**: a modal or component owns keys while it is active (text input, action panel, pickers, etc).

## The core contract

1) **Everything is reachable from `x`**

- `x` opens a context-aware dispatch menu (“Actions”).
- Every non-trivial operation that Clarity supports in the TUI must be reachable from `x` in the relevant context, even if it also has a direct shortcut.
  - Appearance tweaks are under `x` → `f` (profiles + Unicode/ASCII glyph set).

2) **Direct keys exist for common actions**

- Common actions have view-local direct bindings for speed (e.g. `e` edit title, `space` status).
- These direct keys should stay stable; new actions should preferentially be added to dispatch first.

3) **Reserved global openers**

These are the keys we try to keep globally consistent (outside modals / not typing):

- `x` / `?`: context dispatch (“Actions”)
- `g`: navigation dispatch (“Go to”)
- `a`: agenda dispatch (“Agenda Commands”)
- `c`: capture (currently opens the capture flow; may become a capture dispatch over time)
- `q` / `ctrl+c`: quit

4) **Cancel/back is predictable**

- `esc` and `backspace` mean “back/cancel” (with a few view-specific nuances).
- `ctrl+g` is the “always close” escape hatch inside modals/menus (especially when `esc` is used as “back”).

5) **Navigation has three equivalent key families**

For navigation (moving selection, moving between columns/panes, parent/child traversal), prefer supporting these in parallel when feasible:

- Arrow keys (`←/→/↑/↓`)
- vi keys (`h/j/k/l`)
- Emacs keys (`ctrl+b/ctrl+n/ctrl+p/ctrl+f`)

## Dispatch menus (“Magit-ish”)

The dispatch menu is the primary discoverability surface.

### Action panel keys

When the action panel is open:

- Close: `ctrl+g`
- Back (pop submenu): `esc`, `backspace`
- Move selection: `tab` / `shift+tab`, `↑/↓`, `j/k`, `ctrl+p/ctrl+n` (and `h/l` or `ctrl+b/ctrl+f` where horizontal navigation exists)
- Execute: `enter` (runs selected action when the panel doesn’t define its own `enter`)
- Execute by key: type the action’s key (single-key bindings)

### Dispatch semantics

- The panel should typically **close after executing** an action.
- Submenus should be used to keep the root menu small and to avoid key collisions.

## View-local direct keys (current)

These are the “power-user” direct keys meant to stay stable.

### Outline view (list + columns)

Navigation:
- `↑/↓`, `j/k`, `ctrl+n/ctrl+p`: previous/next item
- `→`, `l`, `ctrl+f`: go into first child (expands if collapsed)
- `←`, `h`, `ctrl+b`: go to parent
- `home`/`g`/`<`: go to start
- `end`/`G`/`>`: go to end

Folding (list mode):
- `tab`: cycle subtree folding (when not in split-preview focus mode)
- `shift+tab`: cycle global folding
- `z`: toggle/cycle subtree folding
- `Z`: toggle/cycle global folding

Item actions (selected row):
- `enter`: open item
- `e`: edit title
- `D`: edit description
- `space`: change status (picker)
- `shift+←/→`: cycle status backward/forward
- `n`: new sibling
- `N`: new child
- `m`: move
- `r`: archive (with confirm)
- `V`: duplicate
- `y`: copy item ref
- `Y`: copy CLI show command
- `C`: add comment
- `w`: add worklog entry
- `p`: toggle priority
- `o`: toggle on-hold
- `A`: assign
- `t`: tags
- `d`: due date
- `s`: schedule date

Outline view controls:
- `v`: cycle outline view mode (`list` ↔ `columns`)
- `S`: edit outline statuses
- `O`: open the outline submenu in the action panel

Structure editing (outline pane only):
- Move: `alt+↑/↓` (or `alt+k/j`, `alt+p/n`)
- Indent/outdent: `alt+→/←` (or `alt+l/f`, `alt+h/b`)
- Cross-terminal fallbacks exist for move/indent/outdent (see `clarity docs tui` for details).

### Item view

The item view is a narrowed outline: it shows the selected item + descendants only, split 50/50 with an activity panel on the right.

Navigation:
- `enter`: narrow further to the selected row
- `esc` / `backspace`: widen (pop the narrow stack) or return to the outline
- `ctrl+x` then `o` (or `ctrl+o`, terminal-dependent): other window (focus left/right)
- `tab` or `z` (left pane): toggle collapse for selected subtree
- `shift+tab` / `backtab` or `Z`: toggle collapse for all

Activity panel (Comments / Worklog / History):
- `tab` / `shift+tab`: cycle section
- `↑/↓`, `j/k`, `ctrl+n/ctrl+p`: move selection
- `pgup/pgdown` (or `ctrl+u/ctrl+d`): scroll the selected entry preview
- `enter`: view selected entry
- `C`: add comment
- `R`: reply to selected comment (Comments)
- `w`: add worklog entry
- `l`: open links picker (Comments/Worklog)

### Agenda view

Navigation:
- `↑/↓`, `j/k`, `ctrl+n/ctrl+p`: previous/next row
- `→`, `l`, `ctrl+f`: expand / go to first child
- `←`, `h`, `ctrl+b`: go to parent
- `z` / `Z`: collapse/expand (subtree / all)

Item actions (selected item row):
- `enter`: open item
- `e`: edit title
- `D`: edit description
- `space`: change status
- `shift+←/→`: cycle status backward/forward
- `r`: archive
- `C`: add comment
- `w`: add worklog entry
- `y` / `Y`: copy helpers

## Design notes on collisions

- `a` is the global agenda opener.
- `A` is used for **Assign…** in outline/item/capture contexts; inside dispatch menus Assign is also `A`.

## Implementation pointers (for contributors)

- Global routing (non-modal): `internal/tui/app_update.go`
- Outline view keys: `internal/tui/app.go` (search for `updateOutline` and `mutateOutlineByKey`)
- Item view keys: `internal/tui/app_update.go` (search for `updateItem`)
- Agenda view keys: `internal/tui/app.go` (search for `updateAgenda`)
- Dispatch menus (action panel): `internal/tui/app.go` (search for `actionPanelActions`)
