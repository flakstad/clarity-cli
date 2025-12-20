package tui

import (
        "fmt"
        "strings"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        "github.com/charmbracelet/lipgloss"
)

func renderItemDetail(db *store.DB, outline model.Outline, it model.Item, width, height int) string {
        titleStyle := lipgloss.NewStyle().Bold(true)
        labelStyle := lipgloss.NewStyle().Faint(true)
        box := lipgloss.NewStyle().
                Width(width).
                Height(height).
                Padding(0, 1).
                Border(lipgloss.RoundedBorder()).
                BorderForeground(lipgloss.Color("240"))

        status := statusLabel(outline, it.StatusID)
        assigned := "-"
        if it.AssignedActorID != nil && strings.TrimSpace(*it.AssignedActorID) != "" {
                assigned = *it.AssignedActorID
        }

        commentsCount := 0
        for _, c := range db.Comments {
                if c.ItemID == it.ID {
                        commentsCount++
                }
        }

        worklogCount := "-"
        if db.CurrentActorID != "" {
                if humanID, ok := db.HumanUserIDForActor(db.CurrentActorID); ok {
                        n := 0
                        for _, w := range db.Worklog {
                                if w.ItemID != it.ID {
                                        continue
                                }
                                if authorHuman, ok := db.HumanUserIDForActor(w.AuthorID); ok && authorHuman == humanID {
                                        n++
                                }
                        }
                        worklogCount = fmt.Sprintf("%d", n)
                }
        }

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
                labelStyle.Render("Status: ") + status,
                labelStyle.Render("Owner: ") + it.OwnerActorID,
                labelStyle.Render("Assigned: ") + assigned,
                labelStyle.Render("Priority: ") + fmt.Sprintf("%v", it.Priority),
                labelStyle.Render("On hold: ") + fmt.Sprintf("%v", it.OnHold),
                "",
                labelStyle.Render("Description"),
                desc,
                "",
                labelStyle.Render("Related"),
                fmt.Sprintf("Comments: %d  Worklog (yours): %s", commentsCount, worklogCount),
                "",
                labelStyle.Render("Hints"),
                "- Use the CLI to add comments/worklog while TUI is read-only:",
                "  clarity comments add " + it.ID + " --body \"...\"",
                "  clarity worklog add " + it.ID + " --body \"...\"",
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
