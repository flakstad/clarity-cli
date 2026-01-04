# Quick capture (hotkey terminal window)

Goal: an "org-capture"-style flow where a capture UI appears instantly (global shortcut), writes something, and disappears again.

## Key constraint

Bubble Tea (and TUIs in general) render to a terminal (TTY). So "a separate window" usually means: a terminal window that is presented like an app window.

## Recommended pattern (portable)

- Provide a dedicated, fast entrypoint: `clarity capture`.
- Configure a terminal/OS hotkey to toggle a *pre-existing* terminal window, then run `clarity capture` inside it.
- On save/cancel, `clarity capture` exits immediately so the hotkey window can hide/close again.

This keeps the Clarity behavior cross-platform while letting each user pick their terminal + window manager.

## Performance notes

"Instant" requires avoiding cold starts:

- Keep the hotkey terminal already running (no app launch / shell startup on each capture).
- Keep the session warm (e.g. a dedicated tab/pane, optionally under `tmux`).
- Keep `clarity capture` as a minimal startup path (avoid loading extra TUI views/state).

If cold-start performance still matters, consider a future `clarity daemon` so `clarity capture` becomes a thin client that asks a long-lived process to do the work.

## What `clarity capture` should do (proposal)

- Launch a minimal capture UI (template selection + draft item editor).
- Create the item on submit and print a stable JSON envelope to stdout (for scripting).
- Exit `0` on success, non-zero on cancel/error.

Hotkey-friendly flags:
- `--no-output` suppresses JSON output.
- `--exit-0-on-cancel` makes cancel exit `0` (useful if your hotkey launcher treats non-zero as a failure).
- `--hotkey` implies both of the above.

## Capture templates (v1)

Capture templates are configured globally in `~/.clarity/config.json` so you can capture into other workspaces (e.g. personal workspace while working in a day job workspace).

You can manage templates in the TUI (recommended): open the Action Panel (`x`), go to Capture (`c`), then open “Capture templates…” (`ctrl+t`) (includes prompts + defaults).

Templates are selected via a multi-key sequence (org-capture style). Each key in `keys` must be exactly one character.

Example:

```json
{
  "captureTemplates": [
    {
      "name": "Work inbox",
      "keys": ["w", "i"],
      "target": { "workspace": "Flakstad Software", "outlineId": "out-y2v74pgi" },
      "defaults": { "title": "Inbox: {{date}}", "tags": ["inbox"] }
    },
    {
      "name": "Personal inbox",
      "keys": ["p", "i"],
      "target": { "workspace": "Personal", "outlineId": "out-abc123" }
    }
  ]
}
```

Notes:
- `target` is stored as `(workspace name, outline id)` for stability, but the TUI shows outline names (users shouldn’t need to think about ids).
- During capture you can change the target outline (move) before saving; if the destination outline has different status definitions, capture will prompt you to pick a valid status.

### Template defaults (v2)

Templates can optionally seed the capture draft with defaults:
- `defaults.title` (string)
- `defaults.description` (string, Markdown)
- `defaults.tags` (list of strings; stored without leading `#`)

`defaults.title` and `defaults.description` support lightweight expansions:
- `{{date}}` → `YYYY-MM-DD`
- `{{time}}` → `HH:MM`
- `{{now}}` → RFC3339 timestamp
- `{{clipboard}}` → clipboard text (best-effort; can be overridden with `CLARITY_CAPTURE_CLIPBOARD`)
- `{{url}}` → `CLARITY_CAPTURE_URL` or `clarity capture --url ...`
- `{{selection}}` → `CLARITY_CAPTURE_SELECTION` or `clarity capture --selection ...`

Notes:
- Clarity does not (yet) auto-detect URL/selection from your OS/app; those values are currently provided via env vars by your launcher script/window manager.
- During capture, `{{...}}` expansions are also applied when saving the title/description fields (so you can type e.g. `{{clipboard}}` directly).

### Template prompts (v3)

Templates can optionally ask questions before capture starts. Each prompt answer becomes a variable you can use as `{{name}}` inside `defaults.title` / `defaults.description` (and later in the title/description fields during capture).

Prompt types:
- `string` (enter/ctrl+s: next)
- `multiline` (ctrl+s: next)
- `choice` (enter: select)
- `confirm` (enter: select yes/no; value is `true`/`false`)

Example:

```json
{
  "captureTemplates": [
    {
      "name": "Work task",
      "keys": ["w", "t"],
      "target": { "workspace": "Flakstad Software", "outlineId": "out-y2v74pgi" },
      "prompts": [
        { "name": "project", "label": "Project", "type": "choice", "options": ["Clarity", "Client", "Internal"], "required": true },
        { "name": "note", "label": "Note (optional)", "type": "multiline" }
      ],
      "defaults": { "title": "{{project}}: {{date}}", "description": "{{note}}\n\nSource: {{url}}" }
    }
  ]
}
```

### Cross-workspace capture + Git sync (v1)
Capture writes directly into the target workspace directory (which may be a Git-backed workspace repo).

Rules:
- If the target workspace repo has an in-progress merge/rebase or unmerged files, capture is blocked (reads are still fine elsewhere). Resolve Git first (e.g. `clarity sync resolve`).
- If the target workspace is behind upstream, capture still works but the UI warns; run Sync soon to reduce the chance of future conflicts.

### Capturing while doing other work (agent-friendly)
If you’re in the middle of working on an item and notice an unrelated issue, capture it as a new item and include where it came from:

```bash
clarity items create --title "..." --description "..." --filed-from <current-item-id>
```

## Example: macOS + iTerm2

iTerm2 has a built-in "Hotkey Window" that can toggle a dropdown terminal instantly. A common setup is:

- Hotkey toggles the window (no new process).
- A shell function / keybinding runs `clarity capture`.

Equivalent setups exist in other terminals (WezTerm/kitty/Ghostty/Windows Terminal) and desktop environments, but configuration is terminal/OS-specific.
