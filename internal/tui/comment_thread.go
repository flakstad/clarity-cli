package tui

import (
        "strings"

        "clarity-cli/internal/model"
)

type commentThreadRow struct {
        Comment model.Comment
        Depth   int
}

// buildCommentThreadRows returns a stable oldest-first threaded traversal:
// root comments (no replyTo) in chronological order, each followed by its replies (also chronological).
//
// Input `comments` is typically newest-first from the store index; this function normalizes internally.
func buildCommentThreadRows(comments []model.Comment) []commentThreadRow {
        if len(comments) == 0 {
                return nil
        }

        // Oldest-first list.
        ordered := make([]model.Comment, 0, len(comments))
        for i := len(comments) - 1; i >= 0; i-- {
                ordered = append(ordered, comments[i])
        }

        byID := map[string]model.Comment{}
        for _, c := range ordered {
                id := strings.TrimSpace(c.ID)
                if id == "" {
                        continue
                }
                byID[id] = c
        }

        children := map[string][]model.Comment{} // parentID -> kids (already oldest-first)
        roots := make([]model.Comment, 0, len(ordered))
        for _, c := range ordered {
                parent := ""
                if c.ReplyToCommentID != nil {
                        parent = strings.TrimSpace(*c.ReplyToCommentID)
                }
                if parent == "" || byID[parent].ID == "" {
                        roots = append(roots, c)
                        continue
                }
                children[parent] = append(children[parent], c)
        }

        out := make([]commentThreadRow, 0, len(ordered))
        seen := map[string]bool{}
        var walk func(c model.Comment, depth int)
        walk = func(c model.Comment, depth int) {
                id := strings.TrimSpace(c.ID)
                if id == "" {
                        return
                }
                if seen[id] {
                        return
                }
                seen[id] = true
                if depth < 0 {
                        depth = 0
                }
                if depth > 8 {
                        depth = 8
                }
                out = append(out, commentThreadRow{Comment: c, Depth: depth})
                for _, kid := range children[id] {
                        walk(kid, depth+1)
                }
        }

        for _, r := range roots {
                walk(r, 0)
        }

        // Any orphans/cycles that weren't reached: append oldest-first at root depth.
        for _, c := range ordered {
                id := strings.TrimSpace(c.ID)
                if id == "" || seen[id] {
                        continue
                }
                out = append(out, commentThreadRow{Comment: c, Depth: 0})
        }
        return out
}

func indexOfCommentRow(rows []commentThreadRow, commentID string) int {
        commentID = strings.TrimSpace(commentID)
        if commentID == "" {
                return 0
        }
        for i := range rows {
                if strings.TrimSpace(rows[i].Comment.ID) == commentID {
                        return i
                }
        }
        if len(rows) > 0 {
                return len(rows) - 1
        }
        return 0
}
