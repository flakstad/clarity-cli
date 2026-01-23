# Web UI (v1, self-host)

Clarity is local-first. The v1 web UI is designed to run **on your machine** and operate on the same workspace directories and Git repos as the CLI/TUI.

## Run

```bash
clarity webtui --addr 127.0.0.1:3334
```

Or run the minimal HTML UI (no JavaScript):

```bash
clarity web --addr 127.0.0.1:3335
```

Or run it in a native webview window (requires a build tag and CGO):

```bash
go run -tags webview ./cmd/clarity webview
```

Or explicitly select a workspace:

```bash
clarity --workspace "Flakstad Software" webtui --addr :3334
```

Or:

```bash
clarity --workspace "Flakstad Software" web --addr :3335
```

## Current status

`clarity webtui` is an **experimental** demo that runs the existing Bubble Tea TUI over the web via a server-side PTY and a browser terminal emulator.

`clarity web` is an **experimental** minimal server-rendered HTML + CSS UI (no JavaScript). Currently it renders the project list for the selected workspace.

`clarity webview` is an **experimental** native webview wrapper around `clarity web`, opening the same UI in a native window by pointing a webview at a local HTTP server.

Notes:
- No auth yet; bind to loopback unless you fully trust your network.
- Each browser tab starts a TUI subprocess on the server.
