package tui

import (
        "fmt"
        "strings"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

func isItemEndState(db *store.DB, it *model.Item) bool {
        if it == nil {
                return false
        }
        if db != nil {
                if o, ok := db.FindOutline(strings.TrimSpace(it.OutlineID)); ok && o != nil {
                        return isEndState(*o, it.StatusID)
                }
        }
        // Fallback for missing outline/status defs.
        return isEndState(model.Outline{}, it.StatusID)
}

func hasIncompleteChildren(db *store.DB, taskID string) bool {
        taskID = strings.TrimSpace(taskID)
        if db == nil || taskID == "" {
                return false
        }
        for i := range db.Items {
                child := &db.Items[i]
                if child.ParentID == nil || strings.TrimSpace(*child.ParentID) != taskID {
                        continue
                }
                if child.Archived {
                        continue
                }
                if !isItemEndState(db, child) {
                        return true
                }
        }
        return false
}

func isBlockedByUndoneDeps(db *store.DB, taskID string) bool {
        taskID = strings.TrimSpace(taskID)
        if db == nil || taskID == "" {
                return false
        }
        for _, d := range db.Deps {
                if d.Type != model.DependencyBlocks {
                        continue
                }
                if strings.TrimSpace(d.FromItemID) != taskID {
                        continue
                }
                dep, ok := db.FindItem(strings.TrimSpace(d.ToItemID))
                if !ok || dep == nil {
                        // If the dep target is missing, treat as blocked until cleaned up.
                        return true
                }
                if dep.Archived {
                        continue
                }
                if !isItemEndState(db, dep) {
                        return true
                }
        }
        return false
}

func explainCompletionBlockers(db *store.DB, taskID string) string {
        hasChildren := hasIncompleteChildren(db, taskID)
        hasDeps := isBlockedByUndoneDeps(db, taskID)
        if hasChildren && hasDeps {
                return "has incomplete children and incomplete dependencies"
        }
        if hasChildren {
                return "has incomplete children"
        }
        if hasDeps {
                return "blocked by incomplete dependencies"
        }
        return ""
}

type completionBlockedError struct {
        taskID string
        reason string
}

func (e completionBlockedError) Error() string {
        if strings.TrimSpace(e.reason) == "" {
                return fmt.Sprintf("cannot complete item %s", strings.TrimSpace(e.taskID))
        }
        return fmt.Sprintf("cannot complete item %s: %s", strings.TrimSpace(e.taskID), strings.TrimSpace(e.reason))
}
