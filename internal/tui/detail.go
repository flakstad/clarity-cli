package tui

import (
        "fmt"
        "sort"
        "strings"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        "github.com/charmbracelet/lipgloss"
)

func renderItemDetail(db *store.DB, outline model.Outline, it model.Item, width, height int, focused bool) string {
        titleStyle := lipgloss.NewStyle().Bold(true)
        labelStyle := lipgloss.NewStyle().Faint(true)
        box := lipgloss.NewStyle().
                Width(width).
                Height(height).
                Padding(0, 1)

        status := renderStatus(outline, it.StatusID)
        assigned := "-"
        if it.AssignedActorID != nil && strings.TrimSpace(*it.AssignedActorID) != "" {
                assigned = *it.AssignedActorID
        }

        // Direct children (shown to support outline-style nesting).
        var children []model.Item
        for _, ch := range db.Items {
                if ch.Archived {
                        continue
                }
                if ch.OutlineID != it.OutlineID {
                        continue
                }
                if ch.ParentID == nil || *ch.ParentID != it.ID {
                        continue
                }
                children = append(children, ch)
        }
        sort.Slice(children, func(i, j int) bool { return compareOutlineItems(children[i], children[j]) < 0 })

        commentsCount := 0
        var comments []model.Comment
        for _, c := range db.Comments {
                if c.ItemID != it.ID {
                        continue
                }
                comments = append(comments, c)
        }
        sort.Slice(comments, func(i, j int) bool { return comments[i].CreatedAt.After(comments[j].CreatedAt) })
        commentsCount = len(comments)

        worklogCount := "-"
        var myWorklog []model.WorklogEntry
        if db.CurrentActorID != "" {
                if humanID, ok := db.HumanUserIDForActor(db.CurrentActorID); ok {
                        n := 0
                        for _, w := range db.Worklog {
                                if w.ItemID != it.ID {
                                        continue
                                }
                                if authorHuman, ok := db.HumanUserIDForActor(w.AuthorID); ok && authorHuman == humanID {
                                        n++
                                        myWorklog = append(myWorklog, w)
                                }
                        }
                        worklogCount = fmt.Sprintf("%d", n)
                }
        }
        sort.Slice(myWorklog, func(i, j int) bool { return myWorklog[i].CreatedAt.After(myWorklog[j].CreatedAt) })

        desc := strings.TrimSpace(it.Description)
        if desc == "" {
                desc = "(no description)"
        } else {
                desc = truncateLines(desc, 12)
        }

        lines := []string{
                titleStyle.Render(it.Title),
                "",
                labelStyle.Render("ID: ") + it.ID,
                labelStyle.Render("Owner: ") + it.OwnerActorID,
                labelStyle.Render("Assigned: ") + assigned,
                labelStyle.Render("Priority: ") + fmt.Sprintf("%v", it.Priority),
                labelStyle.Render("On hold: ") + fmt.Sprintf("%v", it.OnHold),
                "",
                labelStyle.Render("Description"),
                desc,
                "",
                labelStyle.Render("Children"),
                renderChildren(children, 8),
                "",
                labelStyle.Render("Related"),
                fmt.Sprintf("Comments: %d  Worklog (yours): %s", commentsCount, worklogCount),
                "",
                labelStyle.Render("Recent comments"),
                renderComments(comments, 3),
                "",
                labelStyle.Render("Recent worklog (yours)"),
                renderWorklog(myWorklog, 3),
                "",
                labelStyle.Render("Hints"),
                "- tab toggles focus between outline/detail",
                "- n creates a new sibling (outline pane)",
                "- N creates a new subitem",
                "- c adds a comment; w adds a worklog entry",
                "- z toggles collapse; Shift+Z toggles collapse all/expand all",
                "- More via CLI:",
                "  clarity comments list " + it.ID,
                "  clarity worklog list " + it.ID,
        }

        if strings.TrimSpace(status) != "" {
                // Insert status after ID line.
                lines = append(lines[:4], append([]string{labelStyle.Render("Status: ") + status}, lines[4:]...)...)
        }

        return box.Render(strings.Join(lines, "\n"))
}

func truncateLines(s string, maxLines int) string {
        if maxLines <= 0 {
                return ""
        }
        lines := strings.Split(s, "\n")
        if len(lines) <= maxLines {
                return s
        }
        return strings.Join(lines[:maxLines], "\n") + "\n…"
}

func renderChildren(children []model.Item, max int) string {
        if len(children) == 0 {
                return "(no children)"
        }
        if max <= 0 {
                max = 1
        }
        n := len(children)
        if n > max {
                n = max
        }
        lines := make([]string, 0, n+1)
        for i := 0; i < n; i++ {
                lines = append(lines, fmt.Sprintf("- %s", children[i].Title))
        }
        if len(children) > max {
                lines = append(lines, fmt.Sprintf("… and %d more", len(children)-max))
        }
        return strings.Join(lines, "\n")
}

func renderComments(comments []model.Comment, max int) string {
        if len(comments) == 0 {
                return "(no comments)"
        }
        if max <= 0 {
                max = 1
        }
        n := len(comments)
        if n > max {
                n = max
        }
        lines := make([]string, 0, n+1)
        for i := 0; i < n; i++ {
                c := comments[i]
                snippet := strings.TrimSpace(c.Body)
                snippet = strings.ReplaceAll(snippet, "\n", " ")
                if len(snippet) > 80 {
                        snippet = snippet[:80] + "…"
                }
                lines = append(lines, fmt.Sprintf("- %s  %s  %s", fmtTS(c.CreatedAt), c.AuthorID, snippet))
        }
        if len(comments) > max {
                lines = append(lines, fmt.Sprintf("… and %d more", len(comments)-max))
        }
        return strings.Join(lines, "\n")
}

func renderWorklog(entries []model.WorklogEntry, max int) string {
        if len(entries) == 0 {
                return "(no worklog)"
        }
        if max <= 0 {
                max = 1
        }
        n := len(entries)
        if n > max {
                n = max
        }
        lines := make([]string, 0, n+1)
        for i := 0; i < n; i++ {
                w := entries[i]
                snippet := strings.TrimSpace(w.Body)
                snippet = strings.ReplaceAll(snippet, "\n", " ")
                if len(snippet) > 80 {
                        snippet = snippet[:80] + "…"
                }
                lines = append(lines, fmt.Sprintf("- %s  %s", fmtTS(w.CreatedAt), snippet))
        }
        if len(entries) > max {
                lines = append(lines, fmt.Sprintf("… and %d more", len(entries)-max))
        }
        return strings.Join(lines, "\n")
}

func fmtTS(t time.Time) string {
        if t.IsZero() {
                return "-"
        }
        return t.Local().Format("2006-01-02 15:04")
}
