# Dependencies (deps)

Clarity supports two dependency types:
- `blocks`: item B must be done before item A can be completed
- `related`: non-blocking relation

## Add a blocking dependency

```bash
clarity deps add <item-a> --blocks <item-b>
```

## Inspect

```bash
clarity deps tree <item-id>
clarity deps cycles
clarity items ready
```
