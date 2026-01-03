# Serving the existing Bubble Tea TUI over the Web (PTY + xterm.js) — demo plan

This explores a “**don’t rebuild the UI**” approach: serve the existing `clarity-cli` Bubble Tea TUI from a web server via a **server-side PTY** and a **browser terminal emulator** (xterm.js), connected over **WebSockets**.

## 1) Is this a good alternative?

### When it *is* a good idea
- **Fastest path to feature parity**: the browser UI becomes “the TUI”, so you immediately get the same keybindings, views, modals, etc.
- **One UX to maintain**: fewer duplicated interactions between TUI/Web.
- **Remote access**: open Clarity from any machine with a browser, without installing Go/binaries.
- **Best as an “access mode”**: complements a real web UI for guests/mobile, but gives power-users a reliable fallback.

### When it’s *not* a good idea
- **Mobile UX** is weak (soft keyboard, no real terminal feel, limited screen).
- **Multi-user scaling**: each session typically requires a process + PTY; costs and ops rise quickly.
- **Security** becomes serious if exposed publicly (sessions, auth, rate limits, isolation).
- **Web affordances** (buttons, inline previews, attachments UI, rich navigation) remain “terminal-shaped”.

### Recommendation
Treat this as an **additional view** (Terminal Mode) rather than the only web UI:
- Keep the datastar web UI for “Basecamp-like” accessibility.
- Add “TUI-in-browser” as a power-user mode and a fast path to parity.

## 2) Demo goals (minimal but real)

**Success criteria for a demo:**
1. `clarity web` serves a page with a terminal emulator.
2. Opening the page starts a **real Clarity TUI session** on the server (alt-screen, colors, keyboard navigation).
3. Browser sends keystrokes; server sends terminal output; **window resize works**.
4. Session ends cleanly on tab close (process killed, PTY closed).

Non-goals for the first demo:
- Multi-user auth, persistence across reconnect, audit logging, sandboxing, or perfect mobile UX.

## 3) Architecture

```
Browser
  xterm.js
    ⇅ WebSocket
Server (clarity web)
  PTY
    └─ child process: clarity (no args => TUI)
```

Key design decision: **spawn a subprocess** rather than trying to run Bubble Tea “in-process”.
- Keeps the server stable even if the TUI panics/exits.
- Avoids tricky stdio plumbing and global terminal state.
- Makes per-session lifecycle and isolation simpler.

## 4) Concrete demo implementation plan (in `clarity-cli`)

### 4.1 Add a “terminal” route to the existing web server

Add a new web mode/route (preferably behind a flag):
- `clarity web --tui` (or `--terminal`)
- Routes:
  - `GET /terminal` → HTML page with xterm.
  - `GET /terminal/ws` → WebSocket endpoint (PTY bridge).

Optional later: link to `/terminal` from the existing web UI header (power-user shortcut).

### 4.2 Vendor xterm.js (no CDN)

For a demo that works offline and self-contained:
- Vendor:
  - `xterm.js`
  - `xterm.css`
  - `xterm-addon-fit.js` (recommended)
- Put under something like: `clarity-cli/internal/web/static/xterm/…`
- Serve via the existing static file handler (or `go:embed`).

### 4.3 WebSocket ↔ PTY protocol

Keep it simple and robust:
- **Client → server**
  - “data” frames: raw bytes for keypresses.
  - “resize” message: JSON `{type:"resize", cols:..., rows:...}`.
- **Server → client**
  - raw PTY bytes (binary WebSocket frames).

Notes:
- WebSocket binary frames avoid UTF-8 issues and preserve escape sequences.
- Use ping/keepalive so idle connections don’t die silently.

### 4.4 Server-side PTY session lifecycle

On WebSocket connect:
1. Start a PTY with `creack/pty`.
2. Spawn child process: `os.Executable()` with args `["clarity", "--workspace", <...>]` or `["clarity"]`.
3. Bridge IO:
   - PTY stdout → WS
   - WS input → PTY stdin
4. Handle resize:
   - Parse resize messages and call `pty.Setsize`.
5. On disconnect:
   - Kill process, close PTY, return.

Hardening (later):
- Max session duration + idle timeout.
- Limit concurrency.
- Clean termination on browser refresh storms.

### 4.5 Workspace + actor selection

For a demo, pick one of these:
- **Simple**: `/terminal?workspace=Flakstad%20Software` → server passes `--workspace`.
- **Safer**: tie the terminal session to the same “current workspace” already used by `clarity web`.

Actor:
- Reuse existing `clarity web` auth/identity selection where possible, but for demo:
  - Respect server’s configured `--actor`, or
  - Require that workspace already has `currentActorId` set.

### 4.6 Local-only binding for demo

Recommend default bind:
- `--addr 127.0.0.1:3333` (no public exposure).

If exposed later:
- Add auth and CSRF protections.
- Consider per-session isolation (container / separate user).

## 5) Security considerations (why this matters)

Even though the demo only runs `clarity`, it still manipulates local files (workspaces).

If this becomes a real feature:
- Require authentication (already being built for web).
- Ensure the terminal session cannot run arbitrary shell commands.
- Restrict workspace paths and don’t trust query parameters.
- Add resource limits (CPU/mem/session count).

## 6) How this interacts with Git-backed Clarity

Good news: it fits.
- The TUI already runs against the workspace directory + sqlite derived state.
- Git auto-sync continues to work as it does for TUI/CLI.

Caveats:
- If you run **one shared server clone** for many users, they’re all operating on the same repo clone → you’ll want:
  - auto-pull before session start,
  - conflict detection,
  - and maybe “one session per actor” semantics.

## 7) Demo checklist (what we should build first)

1. Add `/terminal` page with xterm.
2. Add `/terminal/ws` WebSocket + PTY bridge.
3. Confirm Bubble Tea alt-screen works in browser.
4. Add resize (fit addon + server sets PTY size).
5. Kill process on disconnect; prevent orphan sessions.

## 8) Questions (to confirm before coding)

1. Should the demo live inside `clarity-cli` as part of `clarity web`, or as a separate `clarity webtui` command?
2. For demo: should terminal sessions always attach to the same workspace as `clarity web --workspace …`, or allow selecting via query string?
3. OK to start with **localhost-only** and no auth, then integrate with the existing magic-link auth later?
