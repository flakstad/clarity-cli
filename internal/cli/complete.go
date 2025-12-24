package cli

import (
        "errors"
        "fmt"
        "strings"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

func validateCanComplete(db *store.DB, t *model.Item) error {
        if hasIncompleteChildren(db, t.ID) {
                return errors.New("cannot complete item: has incomplete children")
        }
        if isBlockedByUndoneDeps(db, t.ID) {
                return errors.New("cannot complete item: blocked by incomplete dependencies")
        }
        return nil
}

func hasIncompleteChildren(db *store.DB, taskID string) bool {
        for _, child := range db.Items {
                if child.ParentID == nil || *child.ParentID != taskID {
                        continue
                }
                if child.Archived {
                        continue
                }
                // Children without an explicit status do not participate in completion blocking.
                if strings.TrimSpace(child.StatusID) == "" {
                        continue
                }
                if !isEndState(db, child.OutlineID, child.StatusID) {
                        return true
                }
        }
        return false
}

func isBlockedByUndoneDeps(db *store.DB, taskID string) bool {
        for _, d := range db.Deps {
                if d.Type != model.DependencyBlocks {
                        continue
                }
                if d.FromItemID != taskID {
                        continue
                }
                dep, ok := db.FindItem(d.ToItemID)
                if !ok {
                        // If the dep target is missing, treat as blocked until cleaned up.
                        return true
                }
                if dep.Archived {
                        continue
                }
                if !isEndState(db, dep.OutlineID, dep.StatusID) {
                        return true
                }
        }
        return false
}

func explainCompletionBlockers(db *store.DB, taskID string) string {
        if hasIncompleteChildren(db, taskID) && isBlockedByUndoneDeps(db, taskID) {
                return "has incomplete children and incomplete dependencies"
        }
        if hasIncompleteChildren(db, taskID) {
                return "has incomplete children"
        }
        if isBlockedByUndoneDeps(db, taskID) {
                return "blocked by incomplete dependencies"
        }
        return ""
}

type completionBlockedError struct {
        taskID string
        reason string
}

func (e completionBlockedError) Error() string {
        if e.reason == "" {
                return fmt.Sprintf("cannot complete item %s", e.taskID)
        }
        return fmt.Sprintf("cannot complete item %s: %s", e.taskID, e.reason)
}
