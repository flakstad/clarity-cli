package tui

import (
        "fmt"
        "math"
        "strings"

        "clarity-cli/internal/model"

        "github.com/charmbracelet/bubbles/list"
        "github.com/charmbracelet/lipgloss"
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

func outlineDisplayName(o model.Outline) string {
        if o.Name != nil && strings.TrimSpace(*o.Name) != "" {
                return strings.TrimSpace(*o.Name)
        }
        return "(unnamed outline)"
}

func (i outlineItem) FilterValue() string {
        if i.outline.Name != nil {
                return *i.outline.Name
        }
        return ""
}
func (i outlineItem) Title() string {
        return outlineDisplayName(i.outline)
}
func (i outlineItem) Description() string { return i.outline.ID }

// outlineMoveOptionItem is used by the "Move to outline" picker.
// Unlike outlineItem, it includes the project name in the display to disambiguate
// outlines across projects.
type outlineMoveOptionItem struct {
        outline     model.Outline
        projectName string
}

func (i outlineMoveOptionItem) FilterValue() string {
        return strings.TrimSpace(i.projectName) + " " + outlineDisplayName(i.outline)
}

func (i outlineMoveOptionItem) Title() string {
        p := strings.TrimSpace(i.projectName)
        if p == "" {
                p = "(unknown project)"
        }
        return fmt.Sprintf("%s / %s", p, outlineDisplayName(i.outline))
}

func (i outlineMoveOptionItem) Description() string {
        // Keep it compact but useful in debug/help.
        oid := strings.TrimSpace(i.outline.ID)
        pid := strings.TrimSpace(i.outline.ProjectID)
        if oid == "" {
                oid = "(unknown outline)"
        }
        if pid == "" {
                pid = "(unknown project)"
        }
        return fmt.Sprintf("%s  (%s)", oid, pid)
}

type workspaceItem struct {
        name    string
        current bool
}

func (i workspaceItem) FilterValue() string { return i.name }
func (i workspaceItem) Title() string {
        if i.current {
                return "• " + i.name
        }
        return i.name
}
func (i workspaceItem) Description() string { return "" }

type outlineRow struct {
        item          model.Item
        depth         int
        hasChildren   bool
        collapsed     bool
        doneChildren  int
        totalChildren int
}

type outlineRowItem struct {
        row     outlineRow
        outline model.Outline
        // flashKind is used for short-lived visual feedback (e.g. permission denied).
        // Known values: "", "error".
        flashKind string
}

func (i outlineRowItem) FilterValue() string { return i.row.item.Title }
func (i outlineRowItem) Title() string {
        prefix := strings.Repeat("  ", i.row.depth)
        status := renderStatus(i.outline, i.row.item.StatusID)
        twisty := " "
        if i.row.hasChildren {
                if i.row.collapsed {
                        twisty = "▸"
                } else {
                        twisty = "▾"
                }
        }
        progress := renderProgressCookie(i.row.doneChildren, i.row.totalChildren)
        if strings.TrimSpace(status) == "" {
                return fmt.Sprintf("%s%s %s%s", prefix, twisty, i.row.item.Title, progress)
        }
        return fmt.Sprintf("%s%s %s %s%s", prefix, twisty, status, i.row.item.Title, progress)
}
func (i outlineRowItem) Description() string { return "" }

type addItemRow struct{}

func (i addItemRow) FilterValue() string { return "" }
func (i addItemRow) Title() string       { return "+ Add" }
func (i addItemRow) Description() string { return "" }

type addProjectRow struct{}

func (i addProjectRow) FilterValue() string { return "" }
func (i addProjectRow) Title() string       { return "+ Add" }
func (i addProjectRow) Description() string { return "" }

type addOutlineRow struct{}

func (i addOutlineRow) FilterValue() string { return "" }
func (i addOutlineRow) Title() string       { return "+ Add" }
func (i addOutlineRow) Description() string { return "" }

type statusOptionItem struct {
        id    string
        label string
}

func (i statusOptionItem) FilterValue() string { return "" }
func (i statusOptionItem) Title() string       { return i.label }
func (i statusOptionItem) Description() string { return i.id }

type outlineStatusDefItem struct {
        def model.OutlineStatusDef
}

func (i outlineStatusDefItem) FilterValue() string { return strings.TrimSpace(i.def.Label) }
func (i outlineStatusDefItem) Title() string {
        lbl := strings.TrimSpace(i.def.Label)
        if lbl == "" {
                lbl = "(unnamed)"
        }
        if i.def.IsEndState {
                return lbl + "  (end)"
        }
        return lbl
}
func (i outlineStatusDefItem) Description() string { return strings.TrimSpace(i.def.ID) }

func statusLabel(outline model.Outline, statusID string) string {
        if strings.TrimSpace(statusID) == "" {
                return ""
        }
        for _, def := range outline.StatusDefs {
                if def.ID == statusID {
                        return def.Label
                }
        }
        // fallback: show raw id
        return statusID
}

var (
        progressFillBg  = ac("189", "242") // light: very light cyan; dark: gray fill
        progressEmptyBg = ac("255", "237") // light: white; dark: dark gray empty
        progressFillFg  = ac("235", "255") // light: dark text; dark: light text
        progressEmptyFg = ac("240", "252") // light: muted; dark: light-ish
)

func renderProgressCookie(done, total int) string {
        if total <= 0 {
                return ""
        }
        if done < 0 {
                done = 0
        }
        if done > total {
                done = total
        }

        inner := fmt.Sprintf("%d/%d", done, total)
        innerRunes := []rune(inner)
        if len(innerRunes) == 0 {
                return ""
        }

        ratio := float64(done) / float64(total)
        width := 10
        minW := len(innerRunes) + 2
        if minW > width {
                width = minW
        }
        filledN := int(math.Round(ratio * float64(width)))
        if filledN < 0 {
                filledN = 0
        }
        if filledN > width {
                filledN = width
        }
        start := (width - len(innerRunes)) / 2

        var b strings.Builder
        for i := 0; i < width; i++ {
                bg := progressEmptyBg
                fg := progressEmptyFg
                if i < filledN {
                        bg = progressFillBg
                        fg = progressFillFg
                }
                ch := " "
                if i >= start && i < start+len(innerRunes) {
                        ch = string(innerRunes[i-start])
                }
                b.WriteString(lipgloss.NewStyle().Background(bg).Foreground(fg).Render(ch))
        }
        return " " + b.String()
}

var (
        // Match the default colors from clarity-components/outline.js:
        // - non-end statuses: --clarity-outline-color-todo (#d16d7a)
        // - end statuses:     --clarity-outline-color-done (#6c757d)
        statusNonEndStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#d16d7a")).Bold(true)
        statusEndStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#6c757d")).Bold(true)
)

func renderStatus(outline model.Outline, statusID string) string {
        label := strings.TrimSpace(statusLabel(outline, statusID))
        if label == "" {
                return ""
        }
        txt := strings.ToUpper(label)
        if isEndState(outline, statusID) {
                return statusEndStyle.Render(txt)
        }
        return statusNonEndStyle.Render(txt)
}

func newList(title, help string, items []list.Item) list.Model {
        l := list.New(items, list.NewDefaultDelegate(), 0, 0)
        l.Title = title
        // We render our own global footer + breadcrumb, so keep list chrome minimal.
        l.SetShowTitle(false)
        l.SetShowHelp(false)
        l.SetShowStatusBar(false)
        l.SetShowPagination(false)
        l.SetFilteringEnabled(true)
        l.SetStatusBarItemName("item", "items")
        // Bubble list defaults to quitting on ESC; in Clarity ESC is "back/cancel".
        l.KeyMap.Quit.SetKeys("q")
        // Add extra aliases for go-to-start/end to better support non-US keyboard muscle memory.
        // This complements existing defaults: GoToStart = home/g, GoToEnd = end/G.
        goToStartKeys := append([]string{}, l.KeyMap.GoToStart.Keys()...)
        goToStartKeys = append(goToStartKeys, "<")
        l.KeyMap.GoToStart.SetKeys(goToStartKeys...)

        goToEndKeys := append([]string{}, l.KeyMap.GoToEnd.Keys()...)
        goToEndKeys = append(goToEndKeys, ">")
        l.KeyMap.GoToEnd.SetKeys(goToEndKeys...)
        return l
}

type agendaRowItem struct {
        row     outlineRow
        outline model.Outline
}

func (i agendaRowItem) FilterValue() string {
        return strings.TrimSpace(i.row.item.Title)
}

func (i agendaRowItem) Title() string {
        indent := strings.Repeat("  ", i.row.depth)
        twisty := ""
        if i.row.hasChildren {
                if i.row.collapsed {
                        twisty = "▸"
                } else {
                        twisty = "▾"
                }
        }

        status := renderStatus(i.outline, i.row.item.StatusID)
        title := strings.TrimSpace(i.row.item.Title)
        if title == "" {
                title = "(untitled)"
        }

        metaParts := make([]string, 0, 3)
        if i.row.item.Priority {
                metaParts = append(metaParts, "priority")
        }
        if i.row.item.OnHold {
                metaParts = append(metaParts, "on hold")
        }
        if i.row.totalChildren > 0 {
                metaParts = append(metaParts, renderProgressCookie(i.row.doneChildren, i.row.totalChildren))
        }
        meta := ""
        if len(metaParts) > 0 {
                meta = "  " + strings.Join(metaParts, " ")
        }

        // Avoid extra indentation under the outline header: top-level leaf items should start
        // flush left. Keep indentation for nested children, and keep a twisty for parents.
        lead := ""
        if i.row.depth > 0 {
                if twisty == "" {
                        lead = indent
                } else {
                        lead = indent + twisty + " "
                }
        } else if twisty != "" {
                lead = twisty + " "
        }

        if strings.TrimSpace(status) == "" {
                if lead == "" {
                        return fmt.Sprintf("%s%s", title, meta)
                }
                return fmt.Sprintf("%s%s%s", lead, title, meta)
        }
        if lead == "" {
                return fmt.Sprintf("%s %s%s", status, title, meta)
        }
        return fmt.Sprintf("%s%s %s%s", lead, status, title, meta)
}

func (i agendaRowItem) Description() string { return "" }

type agendaHeadingItem struct {
        projectName string
        outlineName string
}

func (i agendaHeadingItem) FilterValue() string {
        return strings.TrimSpace(i.projectName + " " + i.outlineName)
}
func (i agendaHeadingItem) Title() string {
        p := strings.TrimSpace(i.projectName)
        if p == "" {
                p = "(project)"
        }
        o := strings.TrimSpace(i.outlineName)
        if o == "" {
                o = "(unnamed outline)"
        }
        return lipgloss.NewStyle().Foreground(ac("240", "245")).Bold(true).Render(p + " / " + o)
}
func (i agendaHeadingItem) Description() string { return "" }

// Archived list items (used by the Archived view).

type archivedHeadingItem struct {
        label string
}

func (i archivedHeadingItem) FilterValue() string { return strings.TrimSpace(i.label) }
func (i archivedHeadingItem) Title() string {
        lbl := strings.TrimSpace(i.label)
        if lbl == "" {
                lbl = "Archived"
        }
        return lipgloss.NewStyle().Foreground(ac("240", "245")).Bold(true).Render(lbl)
}
func (i archivedHeadingItem) Description() string { return "" }

type archivedProjectItem struct {
        projectName string
        projectID   string
}

func (i archivedProjectItem) FilterValue() string {
        return strings.TrimSpace(i.projectName + " " + i.projectID)
}
func (i archivedProjectItem) Title() string {
        name := strings.TrimSpace(i.projectName)
        if name == "" {
                name = "(unnamed project)"
        }
        // Display-only row: keep it muted.
        return lipgloss.NewStyle().Foreground(ac("241", "245")).Render(name)
}
func (i archivedProjectItem) Description() string { return strings.TrimSpace(i.projectID) }

type archivedOutlineItem struct {
        projectName string
        outlineName string
        outlineID   string
}

func (i archivedOutlineItem) FilterValue() string {
        return strings.TrimSpace(i.projectName + " " + i.outlineName + " " + i.outlineID)
}
func (i archivedOutlineItem) Title() string {
        p := strings.TrimSpace(i.projectName)
        if p == "" {
                p = "(project)"
        }
        o := strings.TrimSpace(i.outlineName)
        if o == "" {
                o = "(unnamed outline)"
        }
        // Display-only row: keep it muted and include context.
        return lipgloss.NewStyle().Foreground(ac("241", "245")).Render(p + " / " + o)
}
func (i archivedOutlineItem) Description() string { return strings.TrimSpace(i.outlineID) }

type archivedItemRowItem struct {
        projectName string
        outlineName string
        title       string
        itemID      string
}

func (i archivedItemRowItem) FilterValue() string {
        return strings.TrimSpace(i.projectName + " " + i.outlineName + " " + i.title + " " + i.itemID)
}
func (i archivedItemRowItem) Title() string {
        p := strings.TrimSpace(i.projectName)
        if p == "" {
                p = "(project)"
        }
        o := strings.TrimSpace(i.outlineName)
        if o == "" {
                o = "(outline)"
        }
        t := strings.TrimSpace(i.title)
        if t == "" {
                t = "(untitled)"
        }
        return fmt.Sprintf("%s / %s  %s", p, o, t)
}
func (i archivedItemRowItem) Description() string { return strings.TrimSpace(i.itemID) }
