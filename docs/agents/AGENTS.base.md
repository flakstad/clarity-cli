# Base Agent Guidelines

These are global defaults for agents. Repo-local `AGENTS.md` files (when present) take precedence.

To use this template, copy it into whatever global “base AGENTS” location your agent runner supports, then rely on per-repo `AGENTS.md` files for repo-specific policies (e.g., whether agents may open PRs).

## Concurrent Git Work (Hard Requirement)
If there is any chance multiple agents/humans are working in the same git repo at the same time, **do not** do branch switching or destructive commands (e.g. `git checkout <other-branch>`, `git reset --hard`, `git clean -fdx`) in a shared checkout.

Instead, each agent must work in its own **git worktree + branch**.

### Why worktrees
- A worktree is an additional working directory tied to the same underlying git repo object database.
- Each worktree has its own checked-out `HEAD`, index, and working files, so agents don’t trample each other.
- Creating a worktree is cheaper than cloning: it shares commits/objects with the main repo.

### Standard worktree recipe
Run from an existing checkout of the repo (any checkout is fine; treat that checkout as read-only after creating worktrees):

```bash
# Prefer a stable per-agent session id when available.
AGENT_ID="${CLARITY_AGENT_SESSION:-${AGENT_SESSION:-agent}}"
ITEM_ID="${CLARITY_ITEM_ID:-item-task}" # typically item-xxx
ITEM_SUFFIX="${ITEM_ID#item-}"          # prefer using just xxx in branch names

# Branch can include slashes; directory name should not.
BRANCH="agent/${AGENT_ID}/${ITEM_SUFFIX}"
WT_BASE="${WORKTREE_BASE:-$PWD/.worktrees}"
WORKTREE_DIR="$WT_BASE/${AGENT_ID}-${ITEM_ID}"

mkdir -p "$(dirname "$WORKTREE_DIR")"

git worktree add -b "$BRANCH" "$WORKTREE_DIR" HEAD
cd "$WORKTREE_DIR"
```

Rules:
- Do all edits/tests/formatting inside the worktree directory.
- Treat the original checkout you ran `git worktree add` from as read-only.
- Use a unique branch per agent+item (git will not allow the same branch to be checked out in two worktrees).
- Test early and often; set up a tight feedback loop (run targeted tests/builds after each small change, then broader checks before handoff).
- Agents may commit freely to their own branch/worktree; prefer small, standalone commits and commit frequently.
- Cleanup after handoff: `git worktree remove "$WORKTREE_DIR"` (optional) and delete the branch only if requested.

### If the repo is already “dirty”
Create the worktree from the current `HEAD` anyway; do not attempt to “fix” other people’s changes in the shared checkout.

## Using Clarity
When operating with Clarity, prefer branch/worktree names that include the `item-...` id so work stays attributable and easy to review.
