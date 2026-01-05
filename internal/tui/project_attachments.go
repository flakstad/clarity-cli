package tui

import (
        "fmt"
        "sort"
        "strings"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        "github.com/charmbracelet/bubbles/list"
)

type projectAttachmentListItem struct {
        Attachment model.Attachment
        ItemID     string
        ItemTitle  string
        OutlineID  string
        ProjectID  string
}

func (i projectAttachmentListItem) Title() string {
        title := strings.TrimSpace(i.Attachment.Title)
        if title == "" {
                title = strings.TrimSpace(i.Attachment.OriginalName)
        }
        if title == "" {
                title = strings.TrimSpace(i.Attachment.ID)
        }
        return title
}

func (i projectAttachmentListItem) Description() string {
        alt := strings.TrimSpace(i.Attachment.Alt)
        if len(alt) > 120 {
                alt = alt[:120] + "…"
        }
        itemID := strings.TrimSpace(i.ItemID)
        itemTitle := strings.TrimSpace(i.ItemTitle)
        loc := itemTitle
        if loc == "" {
                loc = "(untitled)"
        }
        if itemID != "" {
                loc += " (" + itemID + ")"
        }
        src := strings.ToLower(strings.TrimSpace(i.Attachment.EntityKind))
        if src == "" {
                src = "item"
        }
        meta := fmt.Sprintf("%s  %dB  %s  via %s", fmtTS(i.Attachment.CreatedAt), i.Attachment.SizeBytes, strings.TrimSpace(i.Attachment.ID), src)
        if alt != "" {
                return loc + "\n" + alt + "\n" + meta
        }
        return loc + "\n" + meta
}

func (i projectAttachmentListItem) FilterValue() string {
        parts := []string{
                i.Attachment.ID,
                i.Attachment.Title,
                i.Attachment.OriginalName,
                i.Attachment.Alt,
                i.ItemID,
                i.ItemTitle,
        }
        return strings.ToLower(strings.TrimSpace(strings.Join(parts, " ")))
}

func findCommentItemID(db *store.DB, commentID string) (string, bool) {
        if db == nil {
                return "", false
        }
        id := strings.TrimSpace(commentID)
        if id == "" {
                return "", false
        }
        for i := range db.Comments {
                if strings.TrimSpace(db.Comments[i].ID) == id {
                        return strings.TrimSpace(db.Comments[i].ItemID), true
                }
        }
        return "", false
}

func (m *appModel) refreshProjectAttachments(projectID string) {
        if m == nil || m.db == nil {
                return
        }
        projectID = strings.TrimSpace(projectID)
        if projectID == "" {
                m.projectAttachmentsList.SetItems(nil)
                return
        }

        items := make([]projectAttachmentListItem, 0, len(m.db.Attachments))
        for _, a := range m.db.Attachments {
                aid := strings.TrimSpace(a.ID)
                if aid == "" {
                        continue
                }
                var itemID string
                switch strings.ToLower(strings.TrimSpace(a.EntityKind)) {
                case "item":
                        itemID = strings.TrimSpace(a.EntityID)
                case "comment":
                        if id, ok := findCommentItemID(m.db, strings.TrimSpace(a.EntityID)); ok {
                                itemID = strings.TrimSpace(id)
                        }
                default:
                        continue
                }
                if itemID == "" {
                        continue
                }
                it, ok := m.db.FindItem(itemID)
                if !ok || it == nil {
                        continue
                }
                if strings.TrimSpace(it.ProjectID) != projectID {
                        continue
                }

                items = append(items, projectAttachmentListItem{
                        Attachment: a,
                        ItemID:     strings.TrimSpace(it.ID),
                        ItemTitle:  strings.TrimSpace(it.Title),
                        OutlineID:  strings.TrimSpace(it.OutlineID),
                        ProjectID:  strings.TrimSpace(it.ProjectID),
                })
        }

        sort.Slice(items, func(i, j int) bool {
                ai := items[i].Attachment.CreatedAt
                aj := items[j].Attachment.CreatedAt
                if ai.Equal(aj) {
                        return items[i].Attachment.ID < items[j].Attachment.ID
                }
                return ai.After(aj)
        })

        out := make([]list.Item, 0, len(items))
        for _, it := range items {
                out = append(out, it)
        }
        m.projectAttachmentsList.SetItems(out)
        if len(out) > 0 {
                m.projectAttachmentsList.Select(0)
        }
}
