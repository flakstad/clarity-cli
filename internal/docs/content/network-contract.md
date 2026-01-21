# Network contract (proposal)

Clarity is designed to be **local-first**. Most commands should not perform any network I/O.

This document describes the intended network behavior as a contract, to make Clarity easier to trust and audit.

## Default stance

- **No telemetry by default**.
- **No network calls for core CRUD** (projects/outlines/items/comments/worklog/attachments metadata).
- Any network-capable feature should be clearly scoped, user-initiated, and documented.

## Expected network I/O surfaces

### Git sync

The `clarity sync ...` commands may perform network I/O by calling Git when you have a remote configured (fetch/pull/push).

Clarity should still function without `git` installed; only the `sync` commands require Git.

### WebTUI serving

`clarity webtui ...` starts an HTTP server that serves a UI. This is
network exposure in the sense that it binds to an address you choose, but it is
not an outbound network call.

If you bind to a non-loopback address, you are explicitly making the UI accessible on your network.

### Licensing (if enabled)

If Clarity implements licensing, the intended model is:
- a one-time online step to obtain a signed license file (optional), and
- offline verification forever after the license is installed.

Licensing should not require periodic “phone home” checks to keep Clarity usable.

## Proposed controls

If Clarity adds any networked surfaces beyond Git sync and explicit web serving, it should also provide:
- a global “offline mode” that blocks outbound network I/O, and
- clear error messages that explain which command attempted network access and why.
