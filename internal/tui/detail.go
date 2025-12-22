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

func renderItemDetail(db *store.DB, outline model.Outline, it model.Item, width, height int, focused bool, events []model.Event) string {
        titleStyle := lipgloss.NewStyle().Bold(true)
        if isEndState(outline, it.StatusID) {
                titleStyle = faintIfDark(lipgloss.NewStyle()).
                        Foreground(colorMuted).
                        Strikethrough(true).
                        Bold(true)
        }
        labelStyle := styleMuted()
        // NOTE: The returned string must be exactly `width` columns wide (ANSI-aware) so that
        // split-view rendering with lipgloss.JoinHorizontal stays stable.
        padX := 1
        innerW := width - (2 * padX)
        if innerW < 0 {
                innerW = 0
        }
        box := lipgloss.NewStyle().
                Width(innerW).
                Height(height).
                Padding(0, padX)

        status := renderStatus(outline, it.StatusID)
        assigned := "-"
        if it.AssignedActorID != nil && strings.TrimSpace(*it.AssignedActorID) != "" {
                assigned = *it.AssignedActorID
        }

        // Direct children (shown to support outline-style nesting).
        children := db.ChildrenOf(it.ID)
        sort.Slice(children, func(i, j int) bool { return compareOutlineItems(children[i], children[j]) < 0 })

        commentsCount := 0
        comments := db.CommentsForItem(it.ID)
        commentsCount = len(comments)

        worklogCount := "-"
        var myWorklog []model.WorklogEntry
        if db.CurrentActorID != "" {
                if humanID, ok := db.HumanUserIDForActor(db.CurrentActorID); ok {
                        n := 0
                        for _, w := range db.WorklogForItem(it.ID) {
                                if authorHuman, ok := db.HumanUserIDForActor(w.AuthorID); ok && authorHuman == humanID {
                                        n++
                                        myWorklog = append(myWorklog, w)
                                }
                        }
                        worklogCount = fmt.Sprintf("%d", n)
                }
        }
        // WorklogForItem is sorted by CreatedAt desc; keep stable ordering in case of ties.
        sort.SliceStable(myWorklog, func(i, j int) bool { return myWorklog[i].CreatedAt.After(myWorklog[j].CreatedAt) })

        desc := "(no description)"
        if strings.TrimSpace(it.Description) != "" {
                rendered := strings.TrimSpace(renderMarkdown(it.Description, innerW))
                if rendered == "" {
                        rendered = strings.TrimSpace(it.Description)
                }
                maxDescLines := height / 2
                if maxDescLines < 6 {
                        maxDescLines = 6
                }
                if maxDescLines > 24 {
                        maxDescLines = 24
                }
                desc = truncateLines(rendered, maxDescLines)
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
                labelStyle.Render("History"),
                renderHistory(db, events, it.ID, 8),
                "",
                labelStyle.Render("Hints"),
                "- tab toggles focus between outline/detail",
                "- n creates a new sibling (outline pane)",
                "- N creates a new subitem",
                "- e edits title; Shift+D edits description",
                "- C adds a comment; w adds a worklog entry",
                "- z toggles collapse; Shift+Z toggles collapse all/expand all",
                "- More via CLI:",
                "  clarity comments list " + it.ID,
                "  clarity worklog list " + it.ID,
        }

        if strings.TrimSpace(status) != "" {
                // Insert status after ID line.
                lines = append(lines[:4], append([]string{labelStyle.Render("Status: ") + status}, lines[4:]...)...)
        }

        // Normalize to guarantee stable split-pane rendering even with unbroken long tokens.
        return normalizePane(box.Render(strings.Join(lines, "\n")), width, height)
}

func renderItemDetailInteractive(db *store.DB, outline model.Outline, it model.Item, width, height int, focus itemPageFocus, events []model.Event) string {
        titleStyle := lipgloss.NewStyle().Bold(true)
        if isEndState(outline, it.StatusID) {
                titleStyle = faintIfDark(lipgloss.NewStyle()).
                        Foreground(colorMuted).
                        Strikethrough(true).
                        Bold(true)
        }
        labelStyle := styleMuted()
        // NOTE: The returned string must be exactly `width` columns wide (ANSI-aware) so that
        // split-view rendering with lipgloss.JoinHorizontal stays stable.
        padX := 1
        innerW := width - (2 * padX)
        if innerW < 0 {
                innerW = 0
        }
        box := lipgloss.NewStyle().
                Width(innerW).
                Height(height).
                Padding(0, padX)

        btnBase := lipgloss.NewStyle().
                Padding(0, 1).
                Foreground(colorSurfaceFg).
                Background(colorControlBg).
                MaxWidth(innerW)
        btnActive := btnBase.Copy().
                Foreground(colorSelectedFg).
                Background(colorAccent).
                Bold(true)
        btn := func(active bool) lipgloss.Style {
                if active {
                        return btnActive
                }
                return btnBase
        }

        status := renderStatus(outline, it.StatusID)
        assigned := "-"
        if it.AssignedActorID != nil && strings.TrimSpace(*it.AssignedActorID) != "" {
                assigned = *it.AssignedActorID
        }

        // Direct children (shown to support outline-style nesting).
        children := db.ChildrenOf(it.ID)
        sort.Slice(children, func(i, j int) bool { return compareOutlineItems(children[i], children[j]) < 0 })

        comments := db.CommentsForItem(it.ID)
        commentsCount := len(comments)

        worklogCount := "-"
        var myWorklog []model.WorklogEntry
        if db.CurrentActorID != "" {
                if humanID, ok := db.HumanUserIDForActor(db.CurrentActorID); ok {
                        n := 0
                        for _, w := range db.WorklogForItem(it.ID) {
                                if authorHuman, ok := db.HumanUserIDForActor(w.AuthorID); ok && authorHuman == humanID {
                                        n++
                                        myWorklog = append(myWorklog, w)
                                }
                        }
                        worklogCount = fmt.Sprintf("%d", n)
                }
        }
        // WorklogForItem is sorted by CreatedAt desc; keep stable ordering in case of ties.
        sort.SliceStable(myWorklog, func(i, j int) bool { return myWorklog[i].CreatedAt.After(myWorklog[j].CreatedAt) })

        desc := "(no description)"
        if strings.TrimSpace(it.Description) != "" {
                rendered := strings.TrimSpace(renderMarkdown(it.Description, innerW))
                if rendered == "" {
                        rendered = strings.TrimSpace(it.Description)
                }
                maxDescLines := height / 2
                if maxDescLines < 6 {
                        maxDescLines = 6
                }
                if maxDescLines > 24 {
                        maxDescLines = 24
                }
                desc = truncateLines(rendered, maxDescLines)
        }

        titleBtn := btn(focus == itemFocusTitle).Render(titleStyle.Render(it.Title))

        lines := []string{
                titleBtn,
                "",
                labelStyle.Render("ID: ") + it.ID,
                labelStyle.Render("Owner: ") + it.OwnerActorID,
                labelStyle.Render("Assigned: ") + assigned,
                labelStyle.Render("Priority: ") + fmt.Sprintf("%v", it.Priority),
                labelStyle.Render("On hold: ") + fmt.Sprintf("%v", it.OnHold),
                "",
                btn(focus == itemFocusDescription).Render("Description (edit)"),
                desc,
                "",
                labelStyle.Render("Children"),
                renderChildren(children, 8),
                "",
                labelStyle.Render("Related"),
                fmt.Sprintf("Comments: %d  %s    Worklog (yours): %s  %s",
                        commentsCount,
                        btn(focus == itemFocusAddComment).Render("Add comment"),
                        worklogCount,
                        btn(focus == itemFocusAddWorklog).Render("Add worklog"),
                ),
                "",
                labelStyle.Render("Recent comments"),
                renderComments(comments, 3),
                "",
                labelStyle.Render("Recent worklog (yours)"),
                renderWorklog(myWorklog, 3),
                "",
                labelStyle.Render("History"),
                renderHistory(db, events, it.ID, 8),
                "",
                labelStyle.Render("Hints"),
                "- tab / shift+tab: move focus",
                "- enter: edit focused field",
                "- e edits title; Shift+D edits description",
                "- C adds a comment; w adds a worklog entry",
                "- space sets status",
                "- More via CLI:",
                "  clarity comments list " + it.ID,
                "  clarity worklog list " + it.ID,
        }

        if strings.TrimSpace(status) != "" {
                // Insert status after ID line.
                statusLine := labelStyle.Render("Status: ") + btn(focus == itemFocusStatus).Render(status)
                lines = append(lines[:4], append([]string{statusLine}, lines[4:]...)...)
        }

        // Normalize to guarantee stable split-pane rendering even with unbroken long tokens.
        return normalizePane(box.Render(strings.Join(lines, "\n")), width, height)
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

func renderHistory(db *store.DB, events []model.Event, itemID string, max int) string {
        if max <= 0 {
                max = 1
        }
        if len(events) == 0 {
                return "(no events)"
        }

        // Filter once, then render newest-first.
        matches := make([]model.Event, 0)
        for _, ev := range events {
                if eventIsForItem(ev, itemID) {
                        matches = append(matches, ev)
                }
        }
        if len(matches) == 0 {
                return "(no events)"
        }

        lines := make([]string, 0, max+1)
        shown := 0
        for i := len(matches) - 1; i >= 0 && shown < max; i-- {
                ev := matches[i]
                actor := strings.TrimSpace(ev.ActorID)
                if db != nil {
                        if a, ok := db.FindActor(actor); ok && strings.TrimSpace(a.Name) != "" {
                                actor = a.Name
                        }
                }
                lines = append(lines, fmt.Sprintf("- %s  %s  %s", fmtTS(ev.TS), actor, eventSummary(ev)))
                shown++
        }
        if len(matches) > max {
                lines = append(lines, fmt.Sprintf("… and %d more", len(matches)-max))
        }
        return strings.Join(lines, "\n")
}

func eventIsForItem(ev model.Event, itemID string) bool {
        id := strings.TrimSpace(itemID)
        if id == "" {
                return false
        }
        if strings.TrimSpace(ev.EntityID) == id {
                return true
        }
        m, ok := ev.Payload.(map[string]any)
        if !ok {
                return false
        }
        switch strings.TrimSpace(ev.Type) {
        case "comment.add", "worklog.add":
                if v, ok := m["itemId"].(string); ok && strings.TrimSpace(v) == id {
                        return true
                }
        case "dep.add":
                if v, ok := m["fromItemId"].(string); ok && strings.TrimSpace(v) == id {
                        return true
                }
                if v, ok := m["toItemId"].(string); ok && strings.TrimSpace(v) == id {
                        return true
                }
        }
        return false
}

func eventSummary(ev model.Event) string {
        typ := strings.TrimSpace(ev.Type)
        if typ == "" {
                typ = "(unknown)"
        }
        m, ok := ev.Payload.(map[string]any)
        if !ok {
                return typ
        }

        switch typ {
        case "item.set_title":
                if v, ok := m["title"].(string); ok && strings.TrimSpace(v) != "" {
                        return "set title: " + truncateInline(v, 60)
                }
        case "item.set_status":
                // Prefer explicit transitions if available.
                from, _ := m["from"].(string)
                to, _ := m["to"].(string)
                from = strings.TrimSpace(from)
                to = strings.TrimSpace(to)
                if from != "" || to != "" {
                        if from == "" {
                                from = "none"
                        }
                        if to == "" {
                                to = "none"
                        }
                        if from == to {
                                return "set status: " + to
                        }
                        return "set status: " + from + " -> " + to
                }
                if v, ok := m["status"].(string); ok {
                        if strings.TrimSpace(v) == "" {
                                return "set status: none"
                        }
                        return "set status: " + v
                }
        case "item.set_description":
                return "updated description"
        case "item.set_priority":
                if v, ok := m["priority"].(bool); ok {
                        return fmt.Sprintf("set priority: %v", v)
                }
        case "item.set_on_hold":
                if v, ok := m["onHold"].(bool); ok {
                        return fmt.Sprintf("set on hold: %v", v)
                }
        case "item.set_assign":
                if v, ok := m["assignedActorId"]; ok {
                        if v == nil {
                                return "unassigned"
                        }
                        if s, ok := v.(string); ok {
                                return "assigned: " + s
                        }
                }
        case "comment.add":
                if v, ok := m["body"].(string); ok && strings.TrimSpace(v) != "" {
                        return "comment: " + truncateInline(v, 60)
                }
                return "comment added"
        case "worklog.add":
                if v, ok := m["body"].(string); ok && strings.TrimSpace(v) != "" {
                        return "worklog: " + truncateInline(v, 60)
                }
                return "worklog added"
        case "dep.add":
                from, _ := m["fromItemId"].(string)
                to, _ := m["toItemId"].(string)
                return "dep: " + strings.TrimSpace(from) + " -> " + strings.TrimSpace(to)
        }

        return typ
}

func truncateInline(s string, max int) string {
        s = strings.TrimSpace(s)
        s = strings.ReplaceAll(s, "\n", " ")
        s = strings.ReplaceAll(s, "\r", " ")
        if max <= 0 || len(s) <= max {
                return s
        }
        return s[:max] + "…"
}
