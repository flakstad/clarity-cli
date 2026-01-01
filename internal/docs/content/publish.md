# Publish (Markdown export)

Publishing produces **derived** files for reading/sharing/archival. It does **not** modify the canonical event logs.

Outputs are designed to be:
- human-readable Markdown
- stable under Git
- safe to regenerate

## Publish an item

Writes `items/<item-id>.md` under the output directory:

```bash
clarity publish item item-abc123 --to ./published
```

Flags:
- `--include-archived`: include archived items (default: exclude)
- `--include-worklog`: include your private worklog entries (default: exclude)
- `--overwrite=false`: fail if output files already exist

## Publish an outline

Writes:
- `outlines/<outline-id>/index.md`
- `outlines/<outline-id>/items/<item-id>.md` (one file per item in the outline)

```bash
clarity publish outline out-xyz --to ./published
```

## Suggested Git workflow

```bash
git status
git add -A
git commit -m "Publish: outline out-xyz"
```
