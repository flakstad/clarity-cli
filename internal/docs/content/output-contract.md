# Output contract (progressive disclosure)

Clarity CLI is designed to be scriptable for agents while remaining usable for humans.

## Stable envelope
Most commands return a stable JSON envelope:
- `data`: the primary result (object or array)
- `meta` (optional): counts, pagination, and other lightweight metadata
- `_hints` (optional): suggested follow-up commands to retrieve related or larger data

Use `--pretty` for human readability.

## Progressive disclosure
Commands avoid inlining large related collections by default.

Example:
- `clarity items show <item-id>` returns the item plus counts and `_hints` for:
  - `clarity comments list <item-id>`
  - `clarity worklog list <item-id>`
  - (Agents) `clarity items claim <item-id>`
  - (Agents) `clarity items set-status <item-id> --status <...>`
  - (Agents) `clarity worklog add <item-id> --body "..."`
  - (Agents) `clarity comments add <item-id> --body "..."`

List endpoints (like comments/worklog) are typically paginated:
- `--limit N` (default small)
- `--offset N`

The response includes:
- `meta.total`, `meta.returned`, `meta.limit`, `meta.offset`
- `_hints` for fetching all or the next page
