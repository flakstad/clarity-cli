# TUI

Run `clarity` with no subcommands to start the interactive TUI.

Current scope (early):
- Centered layout by default (projects/outlines/outline/item views)
- Breadcrumb at the top showing where you are (projects > project > outline > item)
- Go to projects → outlines → items
- Projects and outlines are shown as cards with basic metadata (counts + created/updated)
- Full-screen item view (subtree outline on the left, details on the right)
- Auto-refresh when the local store changes (polls file mtimes)
- Outline shows progress cookies for items with children (e.g. `1/2`)
- Outline list shows item descriptions inline (collapsed by default; `z` toggles)
- Outline list shows comment counts (`c:N`)
- Create items directly from the TUI (sibling and subitem)
- Reorder and restructure items (reorder, indent, outdent)

Key bindings:
- `enter`: open selected item (item view)
- `v`: cycle outline view mode (`list` ↔ `columns`)
- `O`: open outline actions menu (from outline screen; includes rename + description)
- `D` (on outlines screen): edit selected outline description
- `backspace` or `esc`: go back (from item view → outline; from outline → previous screen)
- `x` (or `?`): open the actions menu (context-aware; includes item actions from outline focus, details pane, and item view)
- `x` then `f`: appearance menu (profiles + glyph set; see below)
- For the keybinding contract + a more complete reference, see `clarity docs keybindings`.
- `r`: archive selected item (with confirm)
- `m`: move selected/open item (pick outline → pick mode → optionally pick a top-level item in that outline to become the new parent)
- `y`: copy selected item ID to clipboard
- `Y`: copy `clarity items show <id>` to clipboard
- `V`: duplicate selected/open item
- `C`: add a comment (selected/open item)
- `R`: reply to selected comment (when **Comments** is focused in item view)
- `w`: add an entry to **My worklog** (selected item)
- `o`: toggle on-hold (selected/open item)
- `d`: set/clear due date (selected/open item)
- `s`: set/clear schedule date (selected/open item)
- `g`: open the Go to menu (shows available destinations, including `/` jump-to-item)
  - `/`: jump to an item by id (accepts `item-vth` or just `vth`)
  - `A`: archived (browse archived content; items open read-only)
  - `1`–`5`: recently visited items (full item view)
  - `6`–`9`: recently captured items (via Capture)
  - When you jump to an item via Go to, `backspace`/`esc` returns you to the previous screen.
- `q` or `ctrl+c`: quit

Item view:
- The item view is a narrowed outline: selected item + descendants only.
- `enter`: narrow further to the selected row
- `backspace` / `esc`: widen (pop the narrow stack) or return to the outline
- `ctrl+x` then `o` (or `ctrl+o`, terminal-dependent): other window (focus left/right)
- Activity panel (Comments / My worklog / History): `tab` / `shift+tab` cycles section (when focused), `enter` views entry, `C` adds comment, `w` adds worklog, `R` replies, `L` opens links picker.

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
- `tab`: cycle subtree folding (collapsed → first layer → all layers) (list mode; not in split-preview)
- `shift+tab`: cycle global folding (all collapsed → first layer → all layers)
- `z`: cycle subtree folding (collapsed → first layer → all layers)
- `Shift+Z`: cycle global folding (all collapsed → first layer → all layers)

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
- `+ New` row: select it and press `enter` to add an item (handy for empty outlines)

Notes:
- The TUI still leans on the CLI for some features (for example: tags, due/schedule, advanced queries).

Appearance (profiles / glyphs / lists):
- Clarity can’t change your terminal’s actual font, but it can switch between **Unicode** and **ASCII** glyph sets for UI affordances (chevrons, separators, arrows).
- Use `x` → `f` to switch while running.
- Or set `CLARITY_TUI_GLYPHS=ascii` (or `unicode`) to pick a default.
- `x` → `f` also includes a few experimental **appearance profiles** focused on outline/status rendering. Default remains unchanged.
- Or set `CLARITY_TUI_PROFILE=default|alabaster|dracula|gruvbox|solarized|neon|pills|mono|terminal|custom` to pick a default profile.
- `dracula`, `gruvbox`, and `solarized` are curated palettes that explicitly set the core TUI colors for a consistent look across terminals.
- `terminal` uses the terminal’s built-in ANSI theme colors (0–15) for accents and keeps most surfaces unpainted.
- Palette selection is automatic, but you can force it:
  - `CLARITY_TUI_THEME=light|dark|auto` (preferred)
  - `CLARITY_TUI_DARKBG=true|false` (legacy/override)
  - On macOS, when the terminal doesn’t report a background, Clarity may fall back to OS appearance.
- `x` → `f` also includes experimental Projects/Outlines list styles (cards vs row-based).
- Or set `CLARITY_TUI_LISTS=cards|rows|minimal` to pick a default list style.
- Selections made via `x` → `f` are persisted to `~/.clarity/config.json` under the `tui` key (unless you override via env vars).
- You can define a basic custom profile by editing `config.json`:
  - Set `tui.profile` to `custom`
  - Define `tui.customProfile` colors (light/dark) for status/meta/progress/selection.
  - Example:
    ```json
    {
      "tui": {
        "profile": "custom",
        "glyphs": "unicode",
        "lists": "rows",
        "customProfile": {
          "selectedBg": { "light": "254", "dark": "55" },
          "selectedFg": { "light": "232", "dark": "255" },
          "statusNonEndFg": { "light": "#d16d7a", "dark": "#ff4fd8" },
          "statusEndFg": { "light": "#6c757d", "dark": "#3ddc84" },
          "progressFillBg": { "light": "189", "dark": "57" },
          "progressEmptyBg": { "light": "255", "dark": "235" }
        }
      }
    }
    ```

Comment/worklog editor:
- `ctrl+s`: save
- `ctrl+o`: open in `$VISUAL`/`$EDITOR`
- `ctrl+g`: close (cancel)
- `tab` / `shift+tab`: focus body/save/cancel, `enter` activates buttons

## Git auto-sync (default)

For Git-backed workspaces, the TUI will stage+commit canonical workspace changes after you stop editing for a short while (debounced).
If the repo has an upstream configured, it will also best-effort push.

Disable with:
- `CLARITY_AUTOCOMMIT=0`
- `CLARITY_AUTOPUSH=0` (still commits locally)

Notes:
- Commits include canonical paths only (`events/`, `meta/workspace.json`, `resources/`).
- If pushing fails (auth/non-fast-forward), you can always run `clarity sync push` (or `git push`) manually.
