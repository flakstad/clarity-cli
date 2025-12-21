# Identity + ownership

All writes are attributed to an **actor**:
- `human` = a real user
- `agent` = belongs to a human user (`--user <human-actor-id>`)

## Selecting an actor
- Set once: `clarity identity use <actor-id>`
- Or per command: `clarity --actor <actor-id> ...`

## Agent sessions (recommended)
For autonomous agents, use a stable **session key** and a dedicated agent identity:

- Ensure a session identity (creates if missing, then uses it):
  - `CLARITY_AGENT_SESSION=... clarity identity agent ensure`
- Or, start work on an item (ensure identity + claim item):
  - `CLARITY_AGENT_SESSION=... clarity agent start <item-id>`

If you don't care about session stability, you can omit the session key and Clarity will
generate one automatically (i.e. **new agent identity per run**):

- `clarity identity agent ensure`
- `clarity agent start <item-id>`

## Avoiding agent collisions (recommended)
By default:
- `clarity items ready` only shows **unassigned** items
- `clarity items claim` / `clarity agent start` refuse to take an already-assigned item

To explicitly take an item that is already assigned, pass:
- `--take-assigned`

## Ownership rules (V1)
- **Assignment is the edit lock**: if an item is assigned to another human (including their agents), you can't edit it.
- Otherwise, a human can edit:
  - items they **own**
  - items **assigned to them**
  - items **assigned to their agents**
  - items **owned by their agents**
- Assigning typically transfers ownership to the assignee (with a grace period for the previous owner).
- Anyone can add comments.
- Worklog is private per human user.
