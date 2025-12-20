package tui

import (
        "fmt"
        "strings"

        "clarity-cli/internal/model"

        "github.com/charmbracelet/bubbles/list"
)

type projectItem struct {
        project model.Project
        current bool
}

func (i projectItem) FilterValue() string { return i.project.Name }
func (i projectItem) Title() string {
        if i.current {
                return "• " + i.project.Name
        }
        return i.project.Name
}
func (i projectItem) Description() string { return i.project.ID }

type outlineItem struct {
        outline model.Outline
}

func (i outlineItem) FilterValue() string {
        if i.outline.Name != nil {
                return *i.outline.Name
        }
        return ""
}
func (i outlineItem) Title() string {
        if i.outline.Name != nil && strings.TrimSpace(*i.outline.Name) != "" {
                return *i.outline.Name
        }
        return "(unnamed outline)"
}
func (i outlineItem) Description() string { return i.outline.ID }

type outlineRow struct {
        item        model.Item
        depth       int
        hasChildren bool
        collapsed   bool
}

type outlineRowItem struct {
        row     outlineRow
        outline model.Outline
}

func (i outlineRowItem) FilterValue() string { return i.row.item.Title }
func (i outlineRowItem) Title() string {
        prefix := strings.Repeat("  ", i.row.depth)
        status := statusLabel(i.outline, i.row.item.StatusID)
        twisty := " "
        if i.row.hasChildren {
                if i.row.collapsed {
                        twisty = "▸"
                } else {
                        twisty = "▾"
                }
        }
        return fmt.Sprintf("%s%s [%s] %s", prefix, twisty, status, i.row.item.Title)
}
func (i outlineRowItem) Description() string { return i.row.item.ID }

type addItemRow struct{}

func (i addItemRow) FilterValue() string { return "" }
func (i addItemRow) Title() string       { return "+ Add" }
func (i addItemRow) Description() string { return "__add__" }

func statusLabel(outline model.Outline, statusID string) string {
        if strings.TrimSpace(statusID) == "" {
                return "-"
        }
        for _, def := range outline.StatusDefs {
                if def.ID == statusID {
                        return def.Label
                }
        }
        // fallback: show raw id
        return statusID
}

func newList(title, help string, items []list.Item) list.Model {
        l := list.New(items, list.NewDefaultDelegate(), 0, 0)
        l.Title = title
        l.SetShowHelp(true)
        l.SetFilteringEnabled(true)
        l.SetStatusBarItemName("item", "items")
        return l
}
