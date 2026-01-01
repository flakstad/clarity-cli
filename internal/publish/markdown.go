package publish

import (
        "bytes"
        "fmt"
        "sort"
        "strings"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

type RenderOptions struct {
        IncludeArchived bool
        IncludeWorklog  bool
        ActorID         string
}

func RenderItemMarkdown(db *store.DB, itemID string, opt RenderOptions) (string, error) {
        if db == nil {
                return "", fmt.Errorf("missing db")
        }
        item, ok := db.FindItem(strings.TrimSpace(itemID))
        if !ok || item == nil {
                return "", fmt.Errorf("item not found: %s", itemID)
        }
        if item.Archived && !opt.IncludeArchived {
                return "", fmt.Errorf("item archived (use --include-archived): %s", item.ID)
        }

        var buf bytes.Buffer
        writeLn := func(s string) {
                buf.WriteString(s)
                buf.WriteString("\n")
        }

        writeLn("# " + strings.TrimSpace(item.Title))
        writeLn("")

        projectName := ""
        if p, ok := db.FindProject(item.ProjectID); ok && p != nil {
                projectName = strings.TrimSpace(p.Name)
        }
        outlineName := ""
        if o, ok := db.FindOutline(item.OutlineID); ok && o != nil {
                if o.Name != nil {
                        outlineName = strings.TrimSpace(*o.Name)
                }
        }

        writeLn("## Meta")
        writeLn("")
        writeLn("- ID: " + item.ID)
        if projectName != "" {
                writeLn("- Project: " + projectName + " (" + item.ProjectID + ")")
        } else {
                writeLn("- Project: " + item.ProjectID)
        }
        if outlineName != "" {
                writeLn("- Outline: " + outlineName + " (" + item.OutlineID + ")")
        } else {
                writeLn("- Outline: " + item.OutlineID)
        }
        if strings.TrimSpace(item.StatusID) != "" {
                writeLn("- Status: " + strings.TrimSpace(item.StatusID))
        }
        if item.Priority {
                writeLn("- Priority: true")
        }
        if item.OnHold {
                writeLn("- On hold: true")
        }
        if item.Archived {
                writeLn("- Archived: true")
        }
        if strings.TrimSpace(item.OwnerActorID) != "" {
                writeLn("- Owner: " + strings.TrimSpace(item.OwnerActorID))
        }
        if item.AssignedActorID != nil && strings.TrimSpace(*item.AssignedActorID) != "" {
                writeLn("- Assigned: " + strings.TrimSpace(*item.AssignedActorID))
        }
        if strings.TrimSpace(formatDateTime(item.Due)) != "" {
                writeLn("- Due: " + formatDateTime(item.Due))
        }
        if strings.TrimSpace(formatDateTime(item.Schedule)) != "" {
                writeLn("- Scheduled: " + formatDateTime(item.Schedule))
        }
        if len(item.Tags) > 0 {
                tags := make([]string, 0, len(item.Tags))
                for _, t := range item.Tags {
                        t = strings.TrimSpace(t)
                        if t == "" {
                                continue
                        }
                        tags = append(tags, t)
                }
                sort.Strings(tags)
                if len(tags) > 0 {
                        writeLn("- Tags: " + strings.Join(tags, ", "))
                }
        }
        writeLn("- Created: " + item.CreatedAt.UTC().Format(time.RFC3339))
        writeLn("- Updated: " + item.UpdatedAt.UTC().Format(time.RFC3339))

        desc := strings.TrimSpace(item.Description)
        if desc != "" {
                writeLn("")
                writeLn("## Description")
                writeLn("")
                writeLn(desc)
        }

        comments := commentsForItem(db, item.ID)
        if len(comments) > 0 {
                writeLn("")
                writeLn("## Comments")
                writeLn("")
                for _, c := range comments {
                        writeLn("### " + c.ID + " (" + c.CreatedAt.UTC().Format(time.RFC3339) + ")")
                        writeLn("")
                        if strings.TrimSpace(c.AuthorID) != "" {
                                writeLn("- Author: " + strings.TrimSpace(c.AuthorID))
                        }
                        writeLn("")
                        body := strings.TrimSpace(c.Body)
                        if body == "" {
                                body = "(empty)"
                        }
                        writeLn(body)
                        writeLn("")
                }
        }

        if opt.IncludeWorklog {
                worklog := visibleWorklogForItem(db, opt.ActorID, item.ID)
                if len(worklog) > 0 {
                        writeLn("")
                        writeLn("## Worklog (private)")
                        writeLn("")
                        for _, w := range worklog {
                                writeLn("### " + w.ID + " (" + w.CreatedAt.UTC().Format(time.RFC3339) + ")")
                                writeLn("")
                                if strings.TrimSpace(w.AuthorID) != "" {
                                        writeLn("- Author: " + strings.TrimSpace(w.AuthorID))
                                }
                                writeLn("")
                                body := strings.TrimSpace(w.Body)
                                if body == "" {
                                        body = "(empty)"
                                }
                                writeLn(body)
                                writeLn("")
                        }
                }
        }

        return buf.String(), nil
}

func formatDateTime(dt *model.DateTime) string {
        if dt == nil {
                return ""
        }
        date := strings.TrimSpace(dt.Date)
        if date == "" {
                return ""
        }
        if dt.Time == nil || strings.TrimSpace(*dt.Time) == "" {
                return date
        }
        return date + " " + strings.TrimSpace(*dt.Time)
}

func RenderOutlineIndexMarkdown(db *store.DB, outlineID string, items []*model.Item, opt RenderOptions) (string, error) {
        if db == nil {
                return "", fmt.Errorf("missing db")
        }
        outline, ok := db.FindOutline(strings.TrimSpace(outlineID))
        if !ok || outline == nil {
                return "", fmt.Errorf("outline not found: %s", outlineID)
        }

        var buf bytes.Buffer
        writeLn := func(s string) {
                buf.WriteString(s)
                buf.WriteString("\n")
        }

        title := outline.ID
        if outline.Name != nil && strings.TrimSpace(*outline.Name) != "" {
                title = strings.TrimSpace(*outline.Name) + " (" + outline.ID + ")"
        }
        writeLn("# " + title)
        writeLn("")

        if strings.TrimSpace(outline.Description) != "" {
                writeLn("## Description")
                writeLn("")
                writeLn(strings.TrimSpace(outline.Description))
                writeLn("")
        }

        writeLn("## Items")
        writeLn("")

        tree := buildOutlineTree(items, opt.IncludeArchived)
        for _, root := range tree.Roots {
                renderOutlineItemLine(&buf, tree, root, 0)
        }

        return buf.String(), nil
}

func renderOutlineItemLine(buf *bytes.Buffer, tree outlineTree, it *model.Item, depth int) {
        if buf == nil || it == nil {
                return
        }
        prefix := strings.Repeat("  ", depth)
        status := strings.TrimSpace(it.StatusID)
        if status != "" {
                status = " (" + status + ")"
        }
        fmt.Fprintf(buf, "%s- [%s](items/%s.md)%s\n", prefix, strings.TrimSpace(it.Title), it.ID, status)
        for _, ch := range tree.Children[it.ID] {
                renderOutlineItemLine(buf, tree, ch, depth+1)
        }
}

func commentsForItem(db *store.DB, itemID string) []model.Comment {
        out := make([]model.Comment, 0)
        for _, c := range db.Comments {
                if c.ItemID != itemID {
                        continue
                }
                out = append(out, c)
        }
        sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
        return out
}

func visibleWorklogForItem(db *store.DB, actorID string, itemID string) []model.WorklogEntry {
        actorID = strings.TrimSpace(actorID)
        if actorID == "" {
                return nil
        }
        humanID, ok := db.HumanUserIDForActor(actorID)
        if !ok {
                return nil
        }
        out := make([]model.WorklogEntry, 0)
        for _, w := range db.Worklog {
                if w.ItemID != itemID {
                        continue
                }
                authorHuman, ok := db.HumanUserIDForActor(w.AuthorID)
                if !ok {
                        continue
                }
                if authorHuman != humanID {
                        continue
                }
                out = append(out, w)
        }
        sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
        return out
}

type outlineTree struct {
        Roots    []*model.Item
        Children map[string][]*model.Item
}

func buildOutlineTree(items []*model.Item, includeArchived bool) outlineTree {
        byParent := map[string][]*model.Item{}
        roots := make([]*model.Item, 0)
        for _, it := range items {
                if it == nil {
                        continue
                }
                if it.Archived && !includeArchived {
                        continue
                }
                if it.ParentID == nil || strings.TrimSpace(*it.ParentID) == "" {
                        roots = append(roots, it)
                        continue
                }
                pid := strings.TrimSpace(*it.ParentID)
                byParent[pid] = append(byParent[pid], it)
        }

        store.SortItemsByRankOrder(roots)
        for pid := range byParent {
                store.SortItemsByRankOrder(byParent[pid])
        }
        return outlineTree{Roots: roots, Children: byParent}
}
