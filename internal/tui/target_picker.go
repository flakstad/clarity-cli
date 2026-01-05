package tui

import (
        "fmt"
        "strings"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        "github.com/charmbracelet/bubbles/list"
)

type targetPickTargetKind int

const (
        targetPickTargetURL targetPickTargetKind = iota
        targetPickTargetAttachment
)

type targetPickTarget struct {
        Kind         targetPickTargetKind
        Target       string
        Label        string
        Description  string
        RelatedItem  string
        RelatedTitle string
}

type targetPickItem struct {
        t targetPickTarget
}

func (i targetPickItem) Title() string {
        if s := strings.TrimSpace(i.t.Label); s != "" {
                return s
        }
        return strings.TrimSpace(i.t.Target)
}

func (i targetPickItem) Description() string {
        parts := make([]string, 0, 3)
        if s := strings.TrimSpace(i.t.Description); s != "" {
                parts = append(parts, s)
        }
        if s := strings.TrimSpace(i.t.RelatedTitle); s != "" {
                id := strings.TrimSpace(i.t.RelatedItem)
                if id != "" {
                        parts = append(parts, fmt.Sprintf("%s (%s)", s, id))
                } else {
                        parts = append(parts, s)
                }
        }
        return strings.TrimSpace(strings.Join(parts, "  "))
}

func (i targetPickItem) FilterValue() string {
        return strings.ToLower(strings.TrimSpace(i.Title() + " " + i.Description() + " " + i.t.Target))
}

func (m *appModel) startTargetPicker(title string, targets []targetPickTarget) {
        if m == nil {
                return
        }
        m.targetPickTargets = targets
        items := make([]list.Item, 0, len(targets))
        for _, t := range targets {
                items = append(items, targetPickItem{t: t})
        }
        m.targetPickList.Title = strings.TrimSpace(title)
        m.targetPickList.SetItems(items)
        m.targetPickList.Select(0)

        // Size similarly to other pickers.
        modalW := modalBoxWidth(m.width)
        h := len(targets) + 2
        if h > 18 {
                h = 18
        }
        if h < 8 {
                h = 8
        }
        m.targetPickList.SetSize(modalW-6, h)

        m.modal = modalPickTargets
        m.modalForID = ""
        m.modalForKey = ""
}

func (m *appModel) targetsForMarkdownLinks(md string) []targetPickTarget {
        md = strings.TrimSpace(md)
        if md == "" || m == nil || m.db == nil {
                return nil
        }

        links := extractLinkTargets(md)
        if len(links) == 0 {
                return nil
        }

        out := make([]targetPickTarget, 0, len(links))
        for _, l := range links {
                switch l.Kind {
                case linkNavTargetAttachment:
                        attID := strings.TrimSpace(l.Target)
                        label := attID
                        desc := ""
                        if a, ok := m.db.FindAttachment(attID); ok && a != nil {
                                label = strings.TrimSpace(a.Title)
                                if label == "" {
                                        label = strings.TrimSpace(a.OriginalName)
                                }
                                if label == "" {
                                        label = attID
                                }
                                desc = truncateInline(strings.TrimSpace(a.Alt), 120)

                                itemID, itemTitle, _ := findItemForAttachment(m.db, *a)
                                out = append(out, targetPickTarget{
                                        Kind:         targetPickTargetAttachment,
                                        Target:       attID,
                                        Label:        label,
                                        Description:  desc,
                                        RelatedItem:  itemID,
                                        RelatedTitle: itemTitle,
                                })
                                continue
                        }
                        out = append(out, targetPickTarget{
                                Kind:        targetPickTargetAttachment,
                                Target:      attID,
                                Label:       label,
                                Description: desc,
                        })
                default:
                        u := strings.TrimSpace(l.Target)
                        out = append(out, targetPickTarget{
                                Kind:   targetPickTargetURL,
                                Target: u,
                                Label:  u,
                        })
                }
        }
        return out
}

func findItemForAttachment(db *store.DB, a model.Attachment) (string, string, bool) {
        if db == nil {
                return "", "", false
        }
        switch strings.ToLower(strings.TrimSpace(a.EntityKind)) {
        case "item":
                if it, ok := db.FindItem(strings.TrimSpace(a.EntityID)); ok && it != nil {
                        return strings.TrimSpace(it.ID), strings.TrimSpace(it.Title), true
                }
        case "comment":
                for i := range db.Comments {
                        if strings.TrimSpace(db.Comments[i].ID) == strings.TrimSpace(a.EntityID) {
                                itemID := strings.TrimSpace(db.Comments[i].ItemID)
                                if it, ok := db.FindItem(itemID); ok && it != nil {
                                        return strings.TrimSpace(it.ID), strings.TrimSpace(it.Title), true
                                }
                                return itemID, "", true
                        }
                }
        }
        return "", "", false
}

func (m *appModel) targetsForMarkdownLinksURLOnly(md string) []targetPickTarget {
        md = strings.TrimSpace(md)
        if md == "" || m == nil {
                return nil
        }
        us := extractURLTargets(md)
        if len(us) == 0 {
                return nil
        }
        out := make([]targetPickTarget, 0, len(us))
        for _, u := range us {
                u = strings.TrimSpace(u)
                if u == "" {
                        continue
                }
                out = append(out, targetPickTarget{Kind: targetPickTargetURL, Target: u, Label: u})
        }
        return out
}
