# TUI planning notes: item-499, item-acz, item-yp6

This note captures the current exploration + v1 planning for three items in the `Clarity` project (`out-naj7ynvr`).

Relevant existing docs in `clarity-cli/`:
- `internal/docs/content/tui.md` (current TUI bindings + interaction patterns)
- `internal/tui/markdown.go` (Markdown rendering via `glamour`)
- `internal/tui/item_side_panel.go` (comments/worklog/history side panel rendering)

## item-499 — Link to other items in Markdown fields

### Problem
People want to reference other Clarity items from:
- item description
- comments
- worklog

And then **follow** those references in the TUI without copy/paste gymnastics.

### Proposed “reference” syntax (v1)
Recognize references in plain text inside Markdown:
- `item-abc` (canonical short id form)

Optional follow-ons (not required for v1):
- `clarity:item-abc` (URI form)
- `[[item-abc]]` (wiki-ish)

### Proposed TUI UX (v1)
Keep `glamour` rendering as-is; add a navigation layer:
- Extract referenced item ids from the **currently viewed** Markdown (focused field / selected comment / selected worklog entry).
- Add a single entrypoint to “follow links”:
  - Prefer: `x` action panel → `L` “Links…” (contextual action).
  - If exactly one reference exists: jump directly.
  - If multiple exist: open a picker showing `(id, title)` and open on `enter`.

### Open questions
- Do we accept beads-style ids too (allow `abc` as `item-abc` when unambiguous)?
- Should `Links…` search only the focused field, or aggregate description+comments+worklog on the item page?
- Should we also support “backlinks” later (derived from the event log)?

### Tracked subitems
- `item-otl` (spec + extractor)
- `item-95f` (TUI: follow refs)
- `item-ky4` (docs/tests)

## item-acz — File attachments

### Problem
Teams want attachments (images, PDFs, docs, etc) on items (and possibly comments) while keeping:
- offline-first behavior
- Git-backed workspace semantics
- low merge-conflict incidence

### Proposed storage model (v1)
- Bytes are stored in committed workspace resources:
  - `resources/attachments/<attachment-id>/<original-filename>`
- Metadata is event-sourced (JSONL) and derived into SQLite.

### Proposed metadata (v1)
Introduce an `attachment` entity:
- `id`
- `filename`
- `mime`
- `sizeBytes`
- `sha256` (verify content-addressing; helps detect corruption/duplication)
- `createdAt`, `createdBy`

And add link edges:
- item ↔ attachment (v1)
- comment ↔ attachment (nice-to-have; can be v1.1)

### Proposed UX (v1)
- CLI
  - attach a local file to an item
  - list attachments on an item
  - open/export/download
- TUI
  - add an “Attachments” section/panel on the item page
  - open selected attachment (delegates to OS: `open`/`xdg-open`)
  - optional: preview toggles later

### Git considerations
- For v1: “full clone” semantics apply; large blobs are the user’s responsibility.
- Add guardrails:
  - warn above a size threshold
  - recommend Git LFS (docs + maybe a detection hint)

### Open questions
- “Hard limit” vs “warn only” for attachment size?
- Item-only first, or do we need per-comment attachments immediately?

### Tracked subitems
- `item-ikd` (spec + storage layout)
- `item-uhg` (CLI: add/list/open)
- `item-ldy` (TUI: panel)
- Existing children: `item-vnm`, `item-ks3`, `item-sa2`, `item-zrf`

## item-yp6 — Keybinding convention: lowercase vs uppercase

### Observation
The TUI already has a strong structure (see `clarity-cli/internal/docs/content/tui.md`), including some intentional overrides:
- `a` is used for assignment in outline view (outline.js parity)
- `A` is a global agenda alias because of that
- `C` is “comment” (context-local) while `c` is “capture” (global)
- `N` is “new child” while `n` is “new sibling”

### Proposed convention (v1)
- **Lowercase**: global entrypoints / primary flows (with a small number of documented view overrides)
- **Uppercase**: focused/contextual actions or “variant” of the lowercase command

### What needs deciding
- Do we keep `a` = assign in outline long-term?
  - If yes: explicitly document the exception (`A` = agenda alias).
  - If no: move assign to `A` and reclaim `a` for agenda everywhere (but loses outline.js parity).

### Tracked subitems
- `item-6zz` (audit inventory)
- `item-0dj` (docs)
- `item-tzv` (alignment/refactor as features land)
