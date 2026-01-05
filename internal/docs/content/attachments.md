# Attachments

Attachments are files stored inside a workspace and linked to either:
- an item, or
- a comment.

Files are copied into the workspace under `resources/attachments/` (so they can be committed/synced with the workspace repo).

## CLI

Attach a file:

```bash
clarity attachments add <item-id> <path> --title "Optional title" --alt "Optional description"
clarity attachments add <comment-id> <path> --kind comment
```

List attachments:

```bash
clarity attachments list <item-id>
clarity attachments list <comment-id>
```

Open (OS default handler):

```bash
clarity attachments open <attachment-id>
```

Export (copy file out):

```bash
clarity attachments export <attachment-id> <dest-path>
```

## TUI

In the full-screen item view:
- Tab to “Attachments” to open the attachments side panel.
- `enter` opens the selected attachment (OS default handler).
- `u` uploads an attachment:
  - when focusing Comments, uploads to the selected comment
  - otherwise uploads to the current item
  - opens a file picker, then prompts for title + optional description/alt text
- `e` edits the selected attachment metadata (title + description/alt text).

## Notes

- Default max attachment size is 50MB (`clarity attachments add --max-mb`).
- Inline attachment previews are intentionally not implemented for now; we rely on OS open.
- To reference an attachment in markdown, include its id (e.g. `att-...`) in the description/comment. While focused on Description or Comments, press `l` to open a picker of targets (URLs + `att-...`), then `enter` to open. Worklog supports URLs only.
