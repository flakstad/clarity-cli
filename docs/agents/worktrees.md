# Agent-Concurrent Git Workflow (Worktrees)

This repo is often edited by multiple agents at the same time. To avoid agents clobbering each other’s working files (or “fixing” unrelated changes), use **git worktrees**.

## Concepts
- **Anchor checkout**: any normal clone/checkout of the repo. Use it to create worktrees and to run read-only commands (e.g. `git fetch`, `git log`). Treat it as read-only during concurrent work.
- **Worktree**: an additional working directory backed by the same git object database. Each worktree has its own checked-out `HEAD`, index, and working files.

## Directory convention
Keep all worktrees under:

`<repo>/.worktrees/...` (ignored by git; safe for Go tooling because it starts with `.`)

Example:
- `.worktrees/agent-abc-item-123`

## Agent workflow
From the anchor checkout:

```bash
AGENT_ID="${AGENT_SESSION:-agent}"
ITEM_ID="${CLARITY_ITEM_ID:-task}" # typically item-xxx
ITEM_SUFFIX="${ITEM_ID#item-}"     # prefer using just xxx in branch names

BRANCH="agent/${AGENT_ID}/${ITEM_SUFFIX}"
WT_BASE="${WORKTREE_BASE:-$PWD/.worktrees}"
WORKTREE_DIR="$WT_BASE/${AGENT_ID}-${ITEM_ID}"

mkdir -p "$(dirname "$WORKTREE_DIR")"
git worktree add -b "$BRANCH" "$WORKTREE_DIR" HEAD
```

Rules:
- Do all edits/tests/formatting inside the worktree directory.
- Test early and often; set up a tight feedback loop (run targeted tests/builds after each small change, then broader checks before handoff).
- Agents may commit freely to their own branch/worktree; prefer small, standalone commits and commit frequently.
- Never run destructive commands in the anchor checkout (`git reset --hard`, `git clean -fdx`, switching branches).

## Human review (Magit)
To review an agent’s work, open the agent worktree directory in Emacs and run `magit-status` there. Build/test in that same directory so artifacts stay isolated.

## Human merge from anchor checkout
If you don’t want to create an integration worktree, merge from your normal anchor checkout (keep it on `main`):

```bash
git fetch origin
git switch main
git merge --no-ff "origin/agent/<agent-id>/<item-suffix>"
```
