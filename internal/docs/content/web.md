# Web UI (v1, self-host)

Clarity is local-first. The v1 web UI is designed to run **on your machine** and operate on the same workspace directories and Git repos as the CLI/TUI.

## Run

```bash
clarity webtui --addr 127.0.0.1:3334
```

Or explicitly select a workspace:

```bash
clarity --workspace "Flakstad Software" webtui --addr :3334
```

## Current status

`clarity webtui` is an **experimental** demo that runs the existing Bubble Tea TUI over the web via a server-side PTY and a browser terminal emulator.

Notes:
- No auth yet; bind to loopback unless you fully trust your network.
- Each browser tab starts a TUI subprocess on the server.
