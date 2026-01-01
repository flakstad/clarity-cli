# Project sources (v1 placeholder)

Clarity v1 is **workspace-first**: a workspace is a directory (often a Git repo) containing canonical event logs and optional resources.

For privacy and access control, the v1 recommendation is to use **separate workspaces** (separate repos) per access scope.

## Future: external project sources

To prepare for a future feature where a workspace can reference projects that live in other repos/paths, Clarity reserves an optional file:

- `meta/project-sources.json`

If present, it contains a list of additional sources the workspace may aggregate into its agenda/search views.

Current status:
- Not used by the CLI/TUI yet (placeholder only).
- The file is intentionally simple and Git-friendly.

Example:

```json
{
  "version": 1,
  "sources": [
    { "kind": "external", "name": "Secret", "path": "../secret-clarity-workspace" }
  ]
}
```

