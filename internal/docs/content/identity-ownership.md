# Identity + ownership

All writes are attributed to an **actor**:
- `human` = a real user
- `agent` = belongs to a human user (`--user <human-actor-id>`)

## Selecting an actor
- Set once: `clarity identity use <actor-id>`
- Or per command: `clarity --actor <actor-id> ...`

## Ownership rules (V1)
- Only the **owner** can edit an item.
- Assigning transfers ownership to the assignee (with a grace period for the previous owner).
- A human can edit items owned by their own agents.
- Anyone can add comments.
- Worklog is private per human user.
