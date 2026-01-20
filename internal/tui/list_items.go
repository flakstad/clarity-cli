package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"clarity-cli/internal/model"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
)

type projectItem struct {
	project model.Project
	current bool
	meta    projectCardMeta
}

func (i projectItem) FilterValue() string { return i.project.Name }
func (i projectItem) Title() string {
	if i.current {
		return i.project.Name + " " + glyphBullet()
	}
	return i.project.Name
}
func (i projectItem) Description() string { return i.project.ID }

type projectCardMeta struct {
	outlinesTotal    int
	outlinesArchived int

	itemsTotal    int
	itemsDone     int
	itemsOnHold   int
	itemsNoStatus int

	updatedAt  time.Time
	hasUpdated bool
}

type outlineItem struct {
	outline model.Outline
	current bool
	meta    outlineCardMeta
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
	t := outlineDisplayName(i.outline)
	if i.current {
		return t + " " + glyphBullet()
	}
	return t
}
func (i outlineItem) Description() string { return i.outline.ID }

type outlineCardMeta struct {
	topLevel int

	itemsTotal      int
	itemsWithStatus int
	itemsDone       int
	itemsOnHold     int
	itemsNoStatus   int

	updatedAt  time.Time
	hasUpdated bool
}

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

type moveModeOptionItem struct {
	mode  string
	title string
	desc  string
}

func (i moveModeOptionItem) FilterValue() string { return strings.TrimSpace(i.title + " " + i.desc) }
func (i moveModeOptionItem) Title() string       { return i.title }
func (i moveModeOptionItem) Description() string { return i.desc }

type workspaceItem struct {
	name     string
	desc     string
	current  bool
	archived bool
}

func (i workspaceItem) FilterValue() string { return i.name }
func (i workspaceItem) Title() string {
	n := i.name
	if i.archived {
		n = n + " (archived)"
	}
	if i.current {
		return n + " " + glyphBullet()
	}
	return n
}
func (i workspaceItem) Description() string { return i.desc }

type outlineRow struct {
	item           model.Item
	depth          int
	hasChildren    bool
	hasDescription bool
	collapsed      bool
	checkbox       bool
	doneChildren   int
	totalChildren  int
	commentsCount  int
	// assignedLabel is a cached display label for item.AssignedActorID (computed during refresh).
	// It should not include the leading '@' so renderers can style/prefix consistently.
	assignedLabel string
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
	status := renderItemState(i.outline, i.row.item.StatusID, i.row.checkbox)
	twisty := " "
	if i.row.hasChildren || i.row.hasDescription {
		if i.row.collapsed {
			twisty = glyphTwistyCollapsed()
		} else {
			twisty = glyphTwistyExpanded()
		}
	}
	progress := renderProgressCookie(i.row.doneChildren, i.row.totalChildren)
	if strings.TrimSpace(status) == "" {
		return fmt.Sprintf("%s%s %s%s", prefix, twisty, i.row.item.Title, progress)
	}
	return fmt.Sprintf("%s%s %s %s%s", prefix, twisty, status, i.row.item.Title, progress)
}
func (i outlineRowItem) Description() string { return "" }

type outlineActivityKind string

const (
	outlineActivityCommentsRoot outlineActivityKind = "comments_root"
	outlineActivityComment      outlineActivityKind = "comment"
	outlineActivityDepsRoot     outlineActivityKind = "deps_root"
	outlineActivityDepEdge      outlineActivityKind = "dep_edge"
	outlineActivityWorklogRoot  outlineActivityKind = "worklog_root"
	outlineActivityWorklogEntry outlineActivityKind = "worklog_entry"
	outlineActivityHistoryEntry outlineActivityKind = "history_entry"
)

// outlineActivityRowItem is a "virtual" row rendered in the outline list (used in item view)
// to show comments/worklog/history as if they were outline children.
type outlineActivityRowItem struct {
	// id uniquely identifies the row for selection/collapse state.
	id string
	// itemID is the item this row belongs to (the "real" item in the store).
	itemID string

	kind      outlineActivityKind
	depth     int
	label     string
	collapsed bool

	// entity IDs (optional, kind-dependent).
	commentID      string
	worklogID      string
	eventID        string
	depOtherItemID string

	hasChildren bool
	// hasDescription indicates the row has a body rendered as outlineDescRowItem lines.
	hasDescription bool
}

func (i outlineActivityRowItem) FilterValue() string { return i.label }
func (i outlineActivityRowItem) Title() string       { return i.label }
func (i outlineActivityRowItem) Description() string { return "" }

type outlineDescRowItem struct {
	parentID string
	depth    int
	line     string
}

func (i outlineDescRowItem) FilterValue() string { return "" }
func (i outlineDescRowItem) Title() string       { return i.line }
func (i outlineDescRowItem) Description() string { return "" }

type addItemRow struct{}

func (i addItemRow) FilterValue() string { return "" }
func (i addItemRow) Title() string       { return "+ New" }
func (i addItemRow) Description() string { return "" }

type addProjectRow struct{}

func (i addProjectRow) FilterValue() string { return "" }
func (i addProjectRow) Title() string       { return "+ New" }
func (i addProjectRow) Description() string { return "" }

type addOutlineRow struct{}

func (i addOutlineRow) FilterValue() string { return "" }
func (i addOutlineRow) Title() string       { return "+ New" }
func (i addOutlineRow) Description() string { return "" }

type projectUploadsRow struct {
	projectID string
	count     int
}

func (i projectUploadsRow) FilterValue() string { return "uploads" }
func (i projectUploadsRow) Title() string       { return "Uploads" }
func (i projectUploadsRow) Description() string {
	if i.count == 1 {
		return "1 attachment"
	}
	if i.count > 1 {
		return fmt.Sprintf("%d attachments", i.count)
	}
	return ""
}

type statusOptionItem struct {
	id    string
	label string
}

func (i statusOptionItem) FilterValue() string { return "" }
func (i statusOptionItem) Title() string       { return i.label }
func (i statusOptionItem) Description() string { return i.id }

type assigneeOptionItem struct {
	id    string // empty => none
	label string // display label (typically actor name; falls back to id)
}

func (i assigneeOptionItem) FilterValue() string { return strings.TrimSpace(i.label + " " + i.id) }
func (i assigneeOptionItem) Title() string {
	if strings.TrimSpace(i.id) == "" {
		return "None"
	}
	lbl := strings.TrimSpace(i.label)
	if lbl == "" {
		lbl = strings.TrimSpace(i.id)
	}
	return "@" + lbl
}
func (i assigneeOptionItem) Description() string { return strings.TrimSpace(i.id) }

type tagOptionItem struct {
	tag     string
	checked bool
}

func (i tagOptionItem) FilterValue() string { return strings.TrimSpace(i.tag) }
func (i tagOptionItem) Title() string {
	tag := strings.TrimSpace(i.tag)
	if tag == "" {
		tag = "(empty)"
	}
	if i.checked {
		return "[x] " + tag
	}
	return "[ ] " + tag
}
func (i tagOptionItem) Description() string { return "" }

type outlineStatusDefItem struct {
	def model.OutlineStatusDef
}

func (i outlineStatusDefItem) FilterValue() string { return strings.TrimSpace(i.def.Label) }
func (i outlineStatusDefItem) Title() string {
	lbl := strings.TrimSpace(i.def.Label)
	if lbl == "" {
		lbl = "(unnamed)"
	}
	var flags []string
	if i.def.IsEndState {
		flags = append(flags, "end")
	}
	if i.def.RequiresNote {
		flags = append(flags, "note")
	}
	if len(flags) > 0 {
		return lbl + "  (" + strings.Join(flags, ", ") + ")"
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
	defaultProgressFillBg  = ac("189", "242") // light: very light cyan; dark: gray fill
	defaultProgressEmptyBg = ac("255", "237") // light: white; dark: dark gray empty
	defaultProgressFillFg  = ac("235", "255") // light: dark text; dark: light text
	defaultProgressEmptyFg = ac("240", "252") // light: muted; dark: light-ish

	progressFillBg  lipgloss.TerminalColor = defaultProgressFillBg
	progressEmptyBg lipgloss.TerminalColor = defaultProgressEmptyBg
	progressFillFg  lipgloss.TerminalColor = defaultProgressFillFg
	progressEmptyFg lipgloss.TerminalColor = defaultProgressEmptyFg
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

	// Mono profile: show a plain progress cookie instead of a colored bar.
	if appearanceProfile() == appearanceMono {
		return " " + inner
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
	// - priority:        --clarity-outline-color-priority (#5f9fb0)
	// - on hold:         --clarity-outline-color-on-hold (#f39c12)
	defaultStatusNonEndStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#d16d7a")).Bold(true)
	defaultStatusEndStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#6c757d")).Bold(true)
	defaultMetaPriorityStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#5f9fb0")).Bold(true)
	defaultMetaOnHoldStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#f39c12")).Bold(true)

	// Current styles (switchable via appearance profiles). Default must remain identical.
	statusNonEndStyle = defaultStatusNonEndStyle
	statusEndStyle    = defaultStatusEndStyle
	metaPriorityStyle = defaultMetaPriorityStyle
	metaOnHoldStyle   = defaultMetaOnHoldStyle
	// due/schedule buttons in outline.js use the default "has-data" color (text-secondary), not a semantic accent.
	defaultMetaDueStyle      = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
	defaultMetaScheduleStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
	defaultMetaAssignStyle   = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
	defaultMetaCommentStyle  = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
	defaultMetaTagStyle      = lipgloss.NewStyle().Foreground(colorChromeMutedFg)

	metaDueStyle      = defaultMetaDueStyle
	metaScheduleStyle = defaultMetaScheduleStyle
	metaAssignStyle   = defaultMetaAssignStyle
	metaCommentStyle  = defaultMetaCommentStyle
	metaTagStyle      = defaultMetaTagStyle
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

func renderCheckbox(outline model.Outline, statusID string) string {
	checked := isCheckboxChecked(outline, statusID)
	txt := "☐"
	if checked {
		txt = "☑"
	}
	if glyphs() == glyphSetASCII {
		txt = "[ ]"
		if checked {
			txt = "[x]"
		}
	}
	if checked {
		return statusEndStyle.Render(txt)
	}
	return statusNonEndStyle.Render(txt)
}

func renderItemState(outline model.Outline, statusID string, checkbox bool) string {
	if checkbox {
		return renderCheckbox(outline, statusID)
	}
	return renderStatus(outline, statusID)
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
	// Add Emacs-style navigation aliases (common muscle memory).
	cursorUpKeys := append([]string{}, l.KeyMap.CursorUp.Keys()...)
	cursorUpKeys = append(cursorUpKeys, "ctrl+p")
	l.KeyMap.CursorUp.SetKeys(cursorUpKeys...)

	cursorDownKeys := append([]string{}, l.KeyMap.CursorDown.Keys()...)
	cursorDownKeys = append(cursorDownKeys, "ctrl+n")
	l.KeyMap.CursorDown.SetKeys(cursorDownKeys...)
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
			twisty = glyphTwistyCollapsed()
		} else {
			twisty = glyphTwistyExpanded()
		}
	}

	status := renderItemState(i.outline, i.row.item.StatusID, i.row.checkbox)
	title := strings.TrimSpace(i.row.item.Title)
	if title == "" {
		title = "(untitled)"
	}

	metaParts := make([]string, 0, 12)
	if i.row.item.Priority {
		metaParts = append(metaParts, metaPriorityStyle.Render("priority"))
	}
	if i.row.item.OnHold {
		metaParts = append(metaParts, metaOnHoldStyle.Render("on hold"))
	}
	if s := strings.TrimSpace(formatScheduleLabel(i.row.item.Schedule)); s != "" {
		metaParts = append(metaParts, metaScheduleStyle.Render(s))
	}
	if s := strings.TrimSpace(formatDueLabel(i.row.item.Due)); s != "" {
		metaParts = append(metaParts, metaDueStyle.Render(s))
	}
	if lbl := strings.TrimSpace(i.row.assignedLabel); lbl != "" {
		metaParts = append(metaParts, metaAssignStyle.Render("@"+lbl))
	}
	for _, tag := range i.row.item.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		metaParts = append(metaParts, metaTagStyle.Render("#"+tag))
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
	return lipgloss.NewStyle().Foreground(colorChromeMutedFg).Bold(true).Render(p + " / " + o)
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
	return lipgloss.NewStyle().Foreground(colorChromeMutedFg).Bold(true).Render(lbl)
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
	return lipgloss.NewStyle().Foreground(colorChromeSubtleFg).Render(name)
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
	return lipgloss.NewStyle().Foreground(colorChromeSubtleFg).Render(p + " / " + o)
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
