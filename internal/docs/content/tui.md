# TUI

Run `clarity` with no subcommands to start the interactive TUI.

Current scope (early):
- Navigate projects → outlines → items
- Read item details in a split view
- Auto-refresh when the local store changes (polls file mtimes)
- Manual reload state from disk (so changes made via CLI show up immediately)

Key bindings:
- `enter`: select / drill down
- `backspace` or `esc`: go back
- `r`: reload from disk
- `q` or `ctrl+c`: quit

Notes:
- The TUI is intentionally minimal early on. Use the CLI for writes (create/edit/comment/worklog).
