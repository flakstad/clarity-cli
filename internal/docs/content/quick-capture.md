# Quick capture (hotkey terminal window)

Goal: an "org-capture"-style flow where a capture UI appears instantly (global shortcut), writes something, and disappears again.

## Key constraint

Bubble Tea (and TUIs in general) render to a terminal (TTY). So "a separate window" usually means: a terminal window that is presented like an app window.

## Recommended pattern (portable)

- Provide a dedicated, fast entrypoint: `clarity capture` (future).
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

- Launch a minimal capture UI (title/body/tags/target project+outline).
- Create the item on submit and print a stable JSON envelope to stdout (for scripting).
- Exit `0` on success, non-zero on cancel/error.

## Example: macOS + iTerm2

iTerm2 has a built-in "Hotkey Window" that can toggle a dropdown terminal instantly. A common setup is:

- Hotkey toggles the window (no new process).
- A shell function / keybinding runs `clarity capture`.

Equivalent setups exist in other terminals (WezTerm/kitty/Ghostty/Windows Terminal) and desktop environments, but configuration is terminal/OS-specific.
