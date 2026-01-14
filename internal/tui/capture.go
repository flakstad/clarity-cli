package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"clarity-cli/internal/gitrepo"
	"clarity-cli/internal/model"
	"clarity-cli/internal/store"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var ErrCaptureCanceled = errors.New("capture canceled")

type CaptureResult struct {
	Workspace string
	Dir       string
	ItemID    string
}

type captureFinishedMsg struct {
	result   CaptureResult
	canceled bool
}

// captureOpenTemplatesMsg is sent by embedded capture to ask the parent app model
// to open the capture templates manager.
type captureOpenTemplatesMsg struct{}

type captureEditConfigDoneMsg struct{ err error }

func RunCapture(cfg *store.GlobalConfig, actorOverride string) (CaptureResult, error) {
	applyThemePreference()

	m, err := newCaptureModel(cfg, strings.TrimSpace(actorOverride))
	if err != nil {
		return CaptureResult{}, err
	}
	mm, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return CaptureResult{}, err
	}
	out := mm.(captureModel)
	if out.canceled {
		return CaptureResult{}, ErrCaptureCanceled
	}
	if out.result.ItemID == "" {
		return CaptureResult{}, errors.New("capture ended without result")
	}
	return out.result, nil
}

type capturePhase int

const (
	capturePhaseSelectTemplate capturePhase = iota
	capturePhaseEditDraft
)

type captureModalKind int

const (
	captureModalNone captureModalKind = iota
	captureModalTemplateSearch
	captureModalEditTitle
	captureModalEditDescription
	captureModalPromptInput
	captureModalPromptTextarea
	captureModalPromptChoice
	captureModalPromptConfirm
	captureModalPickOutline
	captureModalPickStatus
	captureModalPickAssignee
	captureModalEditTags
	captureModalSetDue
	captureModalSetSchedule
	captureModalConfirmExit
)

type captureTemplateNode struct {
	children map[string]*captureTemplateNode
	template *store.CaptureTemplate
}

type captureModel struct {
	phase capturePhase

	width  int
	height int

	cfg           *store.GlobalConfig
	actorOverride string

	templateTree       *captureTemplateNode
	templatePrefix     []string
	templateGroupNames map[string]string // keyPath -> label (e.g. "w" => "Work")
	templateList       list.Model
	templateSearchList list.Model
	templateHint       string
	outlineLabelByRef  map[string]string

	promptTemplate *store.CaptureTemplate
	promptIndex    int
	promptValues   map[string]string
	promptList     list.Model

	workspace string
	dir       string
	st        store.Store
	db        *store.DB

	// draftOutlineID is the target outline for all draft items in this capture session.
	draftOutlineID string
	// draftItems are temporary (not persisted) items to be created on save.
	draftItems []model.Item
	// draftCollapsed controls rendering of draftItems via the same outline flattener.
	draftCollapsed map[string]bool
	// draftList renders draftItems using the same outline row renderer as the main outline view.
	draftList list.Model
	// draftSeq is used to generate stable temporary ids (draft-1, draft-2, ...).
	draftSeq int
	// draftVars are prompt answers available for {{var}} expansion during capture.
	draftVars map[string]string

	modal                captureModalKind
	modalForDraftID      string
	pendingOutlineChange string

	outlinePickList list.Model
	statusPickList  list.Model
	assigneeList    list.Model
	tagsList        list.Model
	tagsListActive  *bool
	tagsFocus       tagsModalFocus

	// Date modal inputs (due/schedule).
	yearInput   textinput.Model
	monthInput  textinput.Model
	dayInput    textinput.Model
	hourInput   textinput.Model
	minuteInput textinput.Model
	dateFocus   dateModalFocus
	timeEnabled bool

	titleInput textinput.Model
	input      textinput.Model
	textarea   textarea.Model

	minibuffer string

	canceled bool
	result   CaptureResult

	autoCommit *gitrepo.DebouncedCommitter

	// quitOnDone controls whether capture exits (standalone `clarity capture`) or returns
	// control to a parent model (embedded capture within the main TUI).
	quitOnDone bool
}

type captureOptionItem struct {
	key   string
	label string
	kind  string // "prefix" | "select"
}

func (i captureOptionItem) FilterValue() string { return strings.TrimSpace(i.label) }
func (i captureOptionItem) Title() string {
	key := strings.TrimSpace(i.key)
	if key == "" {
		key = "ENTER"
	}
	if strings.TrimSpace(i.kind) == "select" {
		return fmt.Sprintf("%-6s %s", key, i.label)
	}
	return fmt.Sprintf("%-6s %s", key, i.label)
}
func (i captureOptionItem) Description() string { return "" }

type captureTemplateSearchItem struct {
	template store.CaptureTemplate
	keys     string
	name     string
	target   string
}

func (i captureTemplateSearchItem) FilterValue() string {
	return strings.TrimSpace(i.keys) + " " + strings.TrimSpace(i.name) + " " + strings.TrimSpace(i.target)
}
func (i captureTemplateSearchItem) Title() string {
	keys := strings.TrimSpace(i.keys)
	if keys == "" {
		keys = "(no keys)"
	}
	name := strings.TrimSpace(i.name)
	if name == "" {
		name = "(unnamed template)"
	}
	target := strings.TrimSpace(i.target)
	if target == "" {
		target = strings.TrimSpace(i.template.Target.Workspace) + "/" + strings.TrimSpace(i.template.Target.OutlineID)
	}
	return fmt.Sprintf("[%s] %s  → %s", keys, name, target)
}
func (i captureTemplateSearchItem) Description() string { return "" }

type captureOutlineItem struct {
	outline model.Outline
	label   string
}

func (i captureOutlineItem) FilterValue() string { return strings.TrimSpace(i.label) }
func (i captureOutlineItem) Title() string       { return i.label }
func (i captureOutlineItem) Description() string { return "" }

type captureStatusItem struct {
	def model.OutlineStatusDef
}

func (i captureStatusItem) FilterValue() string { return strings.TrimSpace(i.def.Label) }
func (i captureStatusItem) Title() string {
	lbl := strings.TrimSpace(i.def.Label)
	if lbl == "" {
		lbl = i.def.ID
	}
	return lbl
}
func (i captureStatusItem) Description() string { return "" }

type capturePromptChoiceItem struct {
	value string
	label string
}

func (i capturePromptChoiceItem) FilterValue() string { return strings.TrimSpace(i.label) }
func (i capturePromptChoiceItem) Title() string {
	if strings.TrimSpace(i.label) != "" {
		return strings.TrimSpace(i.label)
	}
	return strings.TrimSpace(i.value)
}
func (i capturePromptChoiceItem) Description() string { return "" }

func newCaptureModel(cfg *store.GlobalConfig, actorOverride string) (captureModel, error) {
	if cfg == nil {
		cfg = &store.GlobalConfig{}
	}
	if err := store.ValidateCaptureTemplates(cfg); err != nil {
		return captureModel{}, err
	}
	root := buildCaptureTemplateTree(cfg.CaptureTemplates)
	groupNames := captureTemplateGroupNameMap(cfg)

	m := captureModel{
		phase:              capturePhaseSelectTemplate,
		cfg:                cfg,
		actorOverride:      actorOverride,
		templateTree:       root,
		templateGroupNames: groupNames,
		outlineLabelByRef:  map[string]string{},
		quitOnDone:         true,
	}

	m.templateList = newList("Capture Templates", "Select a template", []list.Item{})
	m.templateList.SetFilteringEnabled(false)
	m.templateList.SetShowFilter(false)

	m.templateSearchList = newList("Template Search", "Search templates", []list.Item{})
	m.templateSearchList.SetDelegate(newCompactItemDelegate())
	m.templateSearchList.SetFilteringEnabled(true)
	m.templateSearchList.SetShowFilter(true)

	m.promptList = newList("", "", []list.Item{})
	m.promptList.SetFilteringEnabled(false)
	m.promptList.SetShowFilter(false)
	m.templateList.SetDelegate(newCompactItemDelegate())

	m.outlinePickList = newList("Outlines", "Select an outline", []list.Item{})
	m.outlinePickList.SetDelegate(newCompactItemDelegate())
	m.outlinePickList.SetShowHelp(false)
	m.outlinePickList.SetShowStatusBar(false)
	m.outlinePickList.SetShowPagination(false)

	m.statusPickList = newList("Status", "Select status", []list.Item{})
	m.statusPickList.SetDelegate(newCompactItemDelegate())
	m.statusPickList.SetShowHelp(false)
	m.statusPickList.SetShowStatusBar(false)
	m.statusPickList.SetShowPagination(false)

	m.draftList = newList("Draft", "", []list.Item{})
	m.draftList.SetDelegate(newOutlineItemDelegate())
	m.draftList.SetFilteringEnabled(false)
	m.draftList.SetShowFilter(false)
	m.draftList.SetShowHelp(false)
	m.draftList.SetShowStatusBar(false)
	m.draftList.SetShowPagination(false)
	m.draftCollapsed = map[string]bool{}

	m.assigneeList = newList("Assignee", "Select an assignee", []list.Item{})
	m.assigneeList.SetDelegate(newCompactItemDelegate())
	m.assigneeList.SetFilteringEnabled(false)
	m.assigneeList.SetShowFilter(false)
	m.assigneeList.SetShowHelp(false)
	m.assigneeList.SetShowStatusBar(false)
	m.assigneeList.SetShowPagination(false)

	m.tagsListActive = new(bool)
	m.tagsList = newList("Tags", "Edit tags", []list.Item{})
	m.tagsList.SetDelegate(newFocusAwareCompactItemDelegate(m.tagsListActive))
	m.tagsList.SetFilteringEnabled(false)
	m.tagsList.SetShowFilter(false)
	m.tagsList.SetShowHelp(false)
	m.tagsList.SetShowStatusBar(false)
	m.tagsList.SetShowPagination(false)
	m.tagsFocus = tagsFocusInput

	m.titleInput = textinput.New()
	m.titleInput.Prompt = ""
	m.titleInput.CharLimit = 256
	m.titleInput.Width = 48
	m.titleInput.Placeholder = "Title"

	// Shared modal input (tags editor, etc).
	m.input = textinput.New()
	m.input.Prompt = ""
	m.input.CharLimit = 256
	m.input.Width = 48

	// Date/time inputs for due/schedule (outline.js-style: date required, time optional).
	m.yearInput = textinput.New()
	m.yearInput.Placeholder = "YYYY"
	m.yearInput.CharLimit = 4
	m.yearInput.Width = 6
	m.monthInput = textinput.New()
	m.monthInput.Placeholder = "MM"
	m.monthInput.CharLimit = 2
	m.monthInput.Width = 4
	m.dayInput = textinput.New()
	m.dayInput.Placeholder = "DD"
	m.dayInput.CharLimit = 2
	m.dayInput.Width = 4
	m.hourInput = textinput.New()
	m.hourInput.Placeholder = "HH"
	m.hourInput.CharLimit = 2
	m.hourInput.Width = 4
	m.minuteInput = textinput.New()
	m.minuteInput.Placeholder = "MM"
	m.minuteInput.CharLimit = 2
	m.minuteInput.Width = 4

	m.textarea = textarea.New()
	// No size limits: capture descriptions/prompts should be able to exceed the
	// textarea defaults (bubbles v0.20 has a small default CharLimit).
	m.textarea.CharLimit = 0
	// Avoid the default line-count cap (MaxHeight governs newline insertion).
	m.textarea.MaxHeight = 0
	// Prefer line numbers for editing longer content.
	m.textarea.ShowLineNumbers = true
	m.textarea.FocusedStyle.CursorLine = m.textarea.BlurredStyle.CursorLine

	m.refreshTemplateList()
	return m, nil
}

func newEmbeddedCaptureModel(cfg *store.GlobalConfig, actorOverride string) (captureModel, error) {
	m, err := newCaptureModel(cfg, actorOverride)
	if err != nil {
		return captureModel{}, err
	}
	m.quitOnDone = false
	return m, nil
}

func (m *captureModel) reloadTemplates(cfg *store.GlobalConfig) {
	if m == nil {
		return
	}
	if cfg == nil {
		cfg = &store.GlobalConfig{}
	}
	if err := store.ValidateCaptureTemplates(cfg); err != nil {
		m.minibuffer = "Templates: " + err.Error()
		return
	}
	m.cfg = cfg
	m.templateTree = buildCaptureTemplateTree(cfg.CaptureTemplates)
	m.templateGroupNames = captureTemplateGroupNameMap(cfg)
	m.templatePrefix = nil
	m.refreshTemplateList()
}

func (m *captureModel) outlineLabel(ws, outlineID string) string {
	ws = strings.TrimSpace(ws)
	outlineID = strings.TrimSpace(outlineID)
	if ws == "" || outlineID == "" {
		return strings.TrimSpace(ws) + "/" + strings.TrimSpace(outlineID)
	}
	ref := ws + "|" + outlineID
	if v, ok := m.outlineLabelByRef[ref]; ok {
		return v
	}

	label := ws + "/" + outlineID
	if dir, err := store.WorkspaceDir(ws); err == nil {
		s := store.Store{Dir: dir}
		if db, err := s.Load(); err == nil && db != nil {
			if o, ok := db.FindOutline(outlineID); ok && o != nil {
				pn := ""
				if p, ok := db.FindProject(o.ProjectID); ok && p != nil {
					pn = strings.TrimSpace(p.Name)
				}
				on := outlineDisplayName(*o)
				label = strings.TrimSpace(ws) + " / " + pn + " / " + on
			}
		}
	}

	m.outlineLabelByRef[ref] = label
	return label
}

func buildCaptureTemplateTree(templates []store.CaptureTemplate) *captureTemplateNode {
	root := &captureTemplateNode{children: map[string]*captureTemplateNode{}}
	for i := range templates {
		keys, err := store.NormalizeCaptureTemplateKeys(templates[i].Keys)
		if err != nil {
			continue
		}
		cur := root
		for _, k := range keys {
			if cur.children == nil {
				cur.children = map[string]*captureTemplateNode{}
			}
			if cur.children[k] == nil {
				cur.children[k] = &captureTemplateNode{children: map[string]*captureTemplateNode{}}
			}
			cur = cur.children[k]
		}
		cur.template = &templates[i]
	}
	return root
}

func captureTemplateGroupNameMap(cfg *store.GlobalConfig) map[string]string {
	out := map[string]string{}
	if cfg == nil || len(cfg.CaptureTemplateGroups) == 0 {
		return out
	}
	for i := range cfg.CaptureTemplateGroups {
		g := cfg.CaptureTemplateGroups[i]
		name := strings.TrimSpace(g.Name)
		if name == "" {
			continue
		}
		keys, err := store.NormalizeCaptureTemplateKeys(g.Keys)
		if err != nil {
			continue
		}
		out[strings.Join(keys, "")] = name
	}
	return out
}

func (m *captureModel) currentTemplateNode() *captureTemplateNode {
	cur := m.templateTree
	for _, k := range m.templatePrefix {
		if cur == nil || cur.children == nil {
			return nil
		}
		cur = cur.children[k]
	}
	return cur
}

func (m *captureModel) refreshTemplateList() {
	cur := m.currentTemplateNode()
	if cur == nil {
		m.templateList.SetItems([]list.Item{})
		return
	}

	var keys []string
	for k := range cur.children {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	items := make([]list.Item, 0, len(keys)+1)
	for _, k := range keys {
		child := cur.children[k]
		label := "(prefix)"
		if child != nil && child.template != nil {
			label = strings.TrimSpace(child.template.Name)
			if label == "" {
				label = "(unnamed template)"
			}
			label = label + "  → " + m.outlineLabel(child.template.Target.Workspace, child.template.Target.OutlineID)
		} else if child != nil {
			label = m.capturePrefixLabel(append(append([]string{}, m.templatePrefix...), k), child)
		}
		items = append(items, captureOptionItem{key: k, label: label, kind: "prefix"})
	}

	if cur.template != nil {
		lbl := strings.TrimSpace(cur.template.Name)
		if lbl == "" {
			lbl = "(unnamed template)"
		}
		items = append([]list.Item{captureOptionItem{key: "ENTER", label: "Use template: " + lbl, kind: "select"}}, items...)
	}

	m.templateList.SetItems(items)
	if len(items) > 0 {
		m.templateList.Select(0)
	}

	if len(m.templatePrefix) == 0 {
		m.templateHint = "Press a key to start a capture template sequence."
	} else {
		m.templateHint = "Prefix: " + strings.Join(m.templatePrefix, " ")
	}
}

func (m *captureModel) capturePrefixLabel(fullPrefix []string, node *captureTemplateNode) string {
	if node == nil {
		return "(group)"
	}
	if len(fullPrefix) > 0 {
		if name, ok := m.templateGroupNames[strings.Join(fullPrefix, "")]; ok && strings.TrimSpace(name) != "" {
			n := countCaptureTemplates(node)
			if n > 1 {
				return fmt.Sprintf("%s (%d)", strings.TrimSpace(name), n)
			}
			return strings.TrimSpace(name)
		}
	}

	// Default: derive from the first word of the first template under this prefix.
	if t := firstCaptureTemplate(node); t != nil {
		base := strings.TrimSpace(t.Name)
		if fields := strings.Fields(base); len(fields) > 0 {
			n := countCaptureTemplates(node)
			if n > 1 {
				return fmt.Sprintf("%s (%d)", fields[0], n)
			}
			return fields[0]
		}
	}
	n := countCaptureTemplates(node)
	if n > 1 {
		return fmt.Sprintf("(group) (%d)", n)
	}
	return "(group)"
}

func countCaptureTemplates(node *captureTemplateNode) int {
	if node == nil {
		return 0
	}
	n := 0
	if node.template != nil {
		n++
	}
	for _, ch := range node.children {
		n += countCaptureTemplates(ch)
	}
	return n
}

func firstCaptureTemplate(node *captureTemplateNode) *store.CaptureTemplate {
	if node == nil {
		return nil
	}
	if node.template != nil {
		return node.template
	}
	if len(node.children) == 0 {
		return nil
	}
	keys := make([]string, 0, len(node.children))
	for k := range node.children {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if t := firstCaptureTemplate(node.children[k]); t != nil {
			return t
		}
	}
	return nil
}

func (m captureModel) Init() tea.Cmd { return nil }

func (m captureModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeLists()
		return m, nil

	case captureEditConfigDoneMsg:
		if msg.err != nil {
			m.minibuffer = "Edit templates: " + msg.err.Error()
			return m, nil
		}
		cfg, err := store.LoadConfig()
		if err != nil {
			m.minibuffer = "Reload templates: " + err.Error()
			return m, nil
		}
		(&m).reloadTemplates(cfg)
		m.minibuffer = "Templates reloaded"
		return m, nil

	case tea.KeyMsg:
		// Global cancel.
		switch msg.String() {
		case "ctrl+g", "esc":
			if m.modal != captureModalNone {
				// Cancel modal only (but prompt modals cancel the whole prompt flow).
				if isCapturePromptModal(m.modal) {
					(&m).cancelPromptFlow()
				}
				(&m).closeModal()
				return m, nil
			}
			// Confirm exit when a draft is in progress, to prevent accidental loss.
			if m.phase == capturePhaseEditDraft {
				m.modal = captureModalConfirmExit
				return m, nil
			}
			m.canceled = true
			return m, m.finishCmd(true)
		}

		if m.modal != captureModalNone {
			return m.updateModal(msg)
		}

		switch m.phase {
		case capturePhaseSelectTemplate:
			return m.updateTemplateSelect(msg)
		case capturePhaseEditDraft:
			return m.updateDraft(msg)
		}
	}
	return m, nil
}

func (m captureModel) finishCmd(canceled bool) tea.Cmd {
	if m.quitOnDone {
		return tea.Quit
	}
	res := m.result
	return func() tea.Msg {
		return captureFinishedMsg{result: res, canceled: canceled}
	}
}

func (m *captureModel) closeModal() {
	if m == nil {
		return
	}
	switch m.modal {
	case captureModalTemplateSearch:
		m.templateSearchList.ResetFilter()
	case captureModalEditTitle:
		m.titleInput.Blur()
	case captureModalEditDescription:
		m.textarea.Blur()
	case captureModalPromptInput:
		m.input.Blur()
	case captureModalPromptTextarea:
		m.textarea.Blur()
	case captureModalEditTags:
		m.tagsFocus = tagsFocusInput
		if m.tagsListActive != nil {
			*m.tagsListActive = false
		}
		m.input.Placeholder = "Title"
		m.input.SetValue("")
		m.input.Blur()
	case captureModalSetDue, captureModalSetSchedule:
		m.yearInput.Blur()
		m.monthInput.Blur()
		m.dayInput.Blur()
		m.hourInput.Blur()
		m.minuteInput.Blur()
		m.dateFocus = dateFocusDay
	case captureModalConfirmExit:
	}
	m.modal = captureModalNone
	m.modalForDraftID = ""
	m.pendingOutlineChange = ""
	m.minibuffer = ""
}

func (m *captureModel) openTemplateSearchModal() {
	if m == nil {
		return
	}

	rows := make([]captureTemplateSearchItem, 0, len(m.cfg.CaptureTemplates))
	for _, t := range m.cfg.CaptureTemplates {
		keys, err := store.NormalizeCaptureTemplateKeys(t.Keys)
		if err != nil {
			continue
		}
		rows = append(rows, captureTemplateSearchItem{
			template: t,
			keys:     strings.Join(keys, " "),
			name:     strings.TrimSpace(t.Name),
			target:   m.outlineLabel(t.Target.Workspace, t.Target.OutlineID),
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		ai := strings.TrimSpace(rows[i].keys)
		aj := strings.TrimSpace(rows[j].keys)
		if ai != aj {
			return ai < aj
		}
		ni := strings.ToLower(strings.TrimSpace(rows[i].name))
		nj := strings.ToLower(strings.TrimSpace(rows[j].name))
		if ni != nj {
			return ni < nj
		}
		return strings.TrimSpace(rows[i].template.Target.OutlineID) < strings.TrimSpace(rows[j].template.Target.OutlineID)
	})

	items := make([]list.Item, 0, len(rows))
	for _, r := range rows {
		items = append(items, r)
	}

	m.templateSearchList.ResetFilter()
	m.templateSearchList.SetItems(items)
	if len(items) > 0 {
		m.templateSearchList.Select(0)
	}
	m.modal = captureModalTemplateSearch
}

func (m captureModel) updateTemplateSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+t":
		if !m.quitOnDone {
			return m, func() tea.Msg { return captureOpenTemplatesMsg{} }
		}
		path, err := store.ConfigPath()
		if err != nil {
			m.minibuffer = "Edit templates: " + err.Error()
			return m, nil
		}
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		if _, err := os.Stat(path); err != nil {
			_ = os.WriteFile(path, []byte("{}\n"), 0o644)
		}
		editor := strings.TrimSpace(externalEditorName())
		if editor == "" {
			m.minibuffer = "Edit templates: set $VISUAL or $EDITOR"
			return m, nil
		}
		cmd := exec.Command(editor, path)
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return captureEditConfigDoneMsg{err: err} })
	case "/", "ctrl+f":
		(&m).openTemplateSearchModal()
		// Immediately enter filter mode so the next keypress searches.
		var cmd tea.Cmd
		m.templateSearchList, cmd = m.templateSearchList.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		return m, cmd
	case "backspace":
		if len(m.templatePrefix) > 0 {
			m.templatePrefix = m.templatePrefix[:len(m.templatePrefix)-1]
			m.refreshTemplateList()
		}
		return m, nil
	case "enter":
		// Support list-driven selection (arrow keys + enter).
		// - If the selected row is "Use template…" -> start draft.
		// - If the selected row is a prefix key -> treat enter like pressing that key.
		if sel, ok := m.templateList.SelectedItem().(captureOptionItem); ok {
			if strings.TrimSpace(sel.kind) == "select" {
				cur := m.currentTemplateNode()
				if cur != nil && cur.template != nil {
					if err := m.beginTemplateCapture(*cur.template); err != nil {
						m.minibuffer = err.Error()
						return m, nil
					}
					return m, nil
				}
			}
			k := strings.TrimSpace(sel.key)
			if k != "" && k != "ENTER" {
				cur := m.currentTemplateNode()
				if cur == nil || cur.children == nil || cur.children[k] == nil {
					m.minibuffer = "No template for key: " + k
					return m, nil
				}
				m.templatePrefix = append(m.templatePrefix, k)
				m.minibuffer = ""
				m.refreshTemplateList()

				// Auto-advance: if the prefix resolves directly to a template and there are no further
				// prefixes, start the draft immediately.
				next := m.currentTemplateNode()
				if next != nil && next.template != nil && len(next.children) == 0 {
					if err := m.beginTemplateCapture(*next.template); err != nil {
						m.minibuffer = err.Error()
						return m, nil
					}
				}
				return m, nil
			}
		}
	}

	// Allow Enter-based fallback via list.
	if msg.String() == "up" || msg.String() == "down" || msg.String() == "/" || strings.HasPrefix(msg.String(), "ctrl+") {
		var cmd tea.Cmd
		m.templateList, cmd = m.templateList.Update(msg)
		return m, cmd
	}

	// Key-driven selection.
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		k := string(msg.Runes[0])
		cur := m.currentTemplateNode()
		if cur == nil || cur.children == nil || cur.children[k] == nil {
			m.minibuffer = "No template for key: " + k
			return m, nil
		}
		m.templatePrefix = append(m.templatePrefix, k)
		m.minibuffer = ""
		m.refreshTemplateList()

		next := m.currentTemplateNode()
		if next != nil && next.template != nil && len(next.children) == 0 {
			if err := m.beginTemplateCapture(*next.template); err != nil {
				m.minibuffer = err.Error()
				return m, nil
			}
		}
		return m, nil
	}

	return m, nil
}

func isCapturePromptModal(k captureModalKind) bool {
	switch k {
	case captureModalPromptInput, captureModalPromptTextarea, captureModalPromptChoice, captureModalPromptConfirm:
		return true
	default:
		return false
	}
}

func (m *captureModel) cancelPromptFlow() {
	if m == nil {
		return
	}
	m.promptTemplate = nil
	m.promptIndex = 0
	m.promptValues = nil
}

func (m captureModel) currentPrompt() (store.CaptureTemplatePrompt, bool) {
	if m.promptTemplate == nil {
		return store.CaptureTemplatePrompt{}, false
	}
	if m.promptIndex < 0 || m.promptIndex >= len(m.promptTemplate.Prompts) {
		return store.CaptureTemplatePrompt{}, false
	}
	return m.promptTemplate.Prompts[m.promptIndex], true
}

func capturePromptLabel(p store.CaptureTemplatePrompt) string {
	if s := strings.TrimSpace(p.Label); s != "" {
		return s
	}
	return strings.TrimSpace(p.Name)
}

func (m *captureModel) beginTemplateCapture(t store.CaptureTemplate) error {
	if m == nil {
		return nil
	}
	// Reset any previous prompt state.
	m.cancelPromptFlow()

	if len(t.Prompts) == 0 {
		return m.startDraftFromTemplateWithVars(t, nil)
	}
	m.promptTemplate = &t
	m.promptIndex = 0
	m.promptValues = map[string]string{}
	return m.openPromptModal(t.Prompts[0])
}

func (m *captureModel) openPromptModal(p store.CaptureTemplatePrompt) error {
	if m == nil || m.promptTemplate == nil {
		return errors.New("internal: prompt without template")
	}

	ws := strings.TrimSpace(m.promptTemplate.Target.Workspace)
	outlineID := strings.TrimSpace(m.promptTemplate.Target.OutlineID)
	ctx := newCaptureExpansionContext(ws, outlineID)
	ctx.Vars = m.promptValues

	label := capturePromptLabel(p)
	typ := strings.TrimSpace(p.Type)
	initial := expandCaptureTemplateString(p.Default, ctx)

	switch typ {
	case "string":
		m.modal = captureModalPromptInput
		m.input.Placeholder = label
		m.input.SetValue(initial)
		m.input.Focus()
		return nil
	case "multiline":
		m.modal = captureModalPromptTextarea
		m.textarea.Placeholder = label
		m.textarea.SetValue(initial)
		m.textarea.Focus()
		return nil
	case "choice":
		m.modal = captureModalPromptChoice
		opts := make([]list.Item, 0, len(p.Options))
		selected := 0
		for i, opt := range p.Options {
			opt = strings.TrimSpace(opt)
			opts = append(opts, capturePromptChoiceItem{value: opt, label: opt})
			if initial != "" && opt == initial {
				selected = i
			}
		}
		m.promptList.SetItems(opts)
		if len(opts) > 0 {
			m.promptList.Select(selected)
		}
		bodyW := modalBodyWidth(m.width)
		h := len(opts) + 2
		if h > 14 {
			h = 14
		}
		if h < 6 {
			h = 6
		}
		m.promptList.SetSize(bodyW, h)
		return nil
	case "confirm":
		m.modal = captureModalPromptConfirm
		opts := []list.Item{
			capturePromptChoiceItem{value: "true", label: "Yes"},
			capturePromptChoiceItem{value: "false", label: "No"},
		}
		selected := 1
		switch strings.ToLower(strings.TrimSpace(initial)) {
		case "y", "yes", "true", "1":
			selected = 0
		}
		m.promptList.SetItems(opts)
		m.promptList.Select(selected)
		bodyW := modalBodyWidth(m.width)
		m.promptList.SetSize(bodyW, 6)
		return nil
	default:
		return fmt.Errorf("unknown prompt type: %q", p.Type)
	}
}

func (m *captureModel) finishPromptAnswer(p store.CaptureTemplatePrompt, value string) error {
	if m == nil || m.promptTemplate == nil {
		return nil
	}

	trimmed := strings.TrimSpace(value)
	if p.Required && trimmed == "" {
		return fmt.Errorf("%s is required", capturePromptLabel(p))
	}
	if strings.TrimSpace(p.Type) != "multiline" {
		value = trimmed
	}
	m.promptValues[strings.TrimSpace(p.Name)] = value

	m.promptIndex++
	if m.promptIndex < len(m.promptTemplate.Prompts) {
		return m.openPromptModal(m.promptTemplate.Prompts[m.promptIndex])
	}

	// Done prompting; start the draft with the collected vars.
	t := *m.promptTemplate
	vars := m.promptValues
	m.cancelPromptFlow()
	return m.startDraftFromTemplateWithVars(t, vars)
}

func (m captureModel) updatePromptInputModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter", "ctrl+s":
		p, ok := m.currentPrompt()
		if !ok {
			(&m).cancelPromptFlow()
			(&m).closeModal()
			return m, nil
		}
		if err := (&m).finishPromptAnswer(p, m.input.Value()); err != nil {
			m.minibuffer = err.Error()
			return m, nil
		}
		m.minibuffer = ""
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m captureModel) updatePromptTextareaModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+s":
		p, ok := m.currentPrompt()
		if !ok {
			(&m).cancelPromptFlow()
			(&m).closeModal()
			return m, nil
		}
		if err := (&m).finishPromptAnswer(p, m.textarea.Value()); err != nil {
			m.minibuffer = err.Error()
			return m, nil
		}
		m.minibuffer = ""
		return m, nil
	}
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

func (m captureModel) updatePromptChoiceModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		p, ok := m.currentPrompt()
		if !ok {
			(&m).cancelPromptFlow()
			(&m).closeModal()
			return m, nil
		}
		sel, ok := m.promptList.SelectedItem().(capturePromptChoiceItem)
		if !ok {
			m.minibuffer = "No selection"
			return m, nil
		}
		if err := (&m).finishPromptAnswer(p, sel.value); err != nil {
			m.minibuffer = err.Error()
			return m, nil
		}
		m.minibuffer = ""
		return m, nil
	}
	var cmd tea.Cmd
	m.promptList, cmd = m.promptList.Update(msg)
	return m, cmd
}

func (m *captureModel) startDraftFromTemplate(t store.CaptureTemplate) error {
	return m.startDraftFromTemplateWithVars(t, nil)
}

func (m *captureModel) startDraftFromTemplateWithVars(t store.CaptureTemplate, vars map[string]string) error {
	ws := strings.TrimSpace(t.Target.Workspace)
	if ws == "" {
		return errors.New("template target workspace is empty")
	}
	dir, err := store.WorkspaceDir(ws)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	gs, _ := gitrepo.GetStatus(ctx, dir)
	if gs.IsRepo && (gs.Unmerged || gs.InProgress) {
		return fmt.Errorf("target workspace %q is in a git merge/rebase; resolve first (try: clarity sync resolve)", ws)
	}
	warn := ""
	if gs.IsRepo && gs.Behind > 0 {
		warn = fmt.Sprintf("Note: workspace is behind upstream by %d commit(s); consider syncing soon.", gs.Behind)
	}

	st := store.Store{Dir: dir}
	db, err := st.Load()
	if err != nil {
		return err
	}
	outID := strings.TrimSpace(t.Target.OutlineID)
	o, ok := db.FindOutline(outID)
	if !ok || o == nil {
		return fmt.Errorf("outline not found in workspace %q: %s", ws, outID)
	}

	exp := newCaptureExpansionContext(ws, o.ID)
	exp.Vars = vars
	defaultTitle := expandCaptureTemplateString(t.Defaults.Title, exp)
	defaultTitle = strings.ReplaceAll(defaultTitle, "\n", " ")
	defaultTitle = strings.ReplaceAll(defaultTitle, "\r", " ")
	defaultTitle = strings.TrimSpace(defaultTitle)
	defaultDesc := expandCaptureTemplateString(t.Defaults.Description, exp)
	defaultTags := store.NormalizeCaptureTemplateTags(t.Defaults.Tags)

	m.workspace = ws
	m.dir = dir
	m.st = st
	m.db = db
	if shouldAutoCommit() && st.IsJSONLWorkspace() {
		m.autoCommit = gitrepo.NewDebouncedCommitter(gitrepo.DebouncedCommitterOpts{
			WorkspaceDir:   dir,
			Debounce:       2 * time.Second,
			AutoPush:       shouldAutoPush(),
			AutoPullRebase: true,
		})
	}
	m.draftOutlineID = o.ID
	m.draftItems = nil
	m.draftCollapsed = map[string]bool{}
	m.draftSeq = 0
	m.draftVars = vars
	// Seed with a single root draft item.
	rootID := m.addDraftItem(nil, defaultTitle)
	if d := m.findDraftItem(rootID); d != nil {
		d.Description = defaultDesc
		d.Tags = defaultTags
		m.setDraftItem(*d)
	}
	m.phase = capturePhaseEditDraft
	m.modal = captureModalEditTitle
	m.minibuffer = warn
	m.openTitleModal(rootID, defaultTitle)
	m.refreshDraftList()
	return nil
}

func (m captureModel) appendEvent(actorID string, eventType string, entityID string, payload any) error {
	if err := m.st.AppendEvent(actorID, eventType, entityID, payload); err != nil {
		return err
	}
	if m.autoCommit != nil {
		label := actorID
		if m.db != nil {
			if a, ok := m.db.FindActor(actorID); ok && strings.TrimSpace(a.Name) != "" {
				label = strings.TrimSpace(a.Name)
			}
		}
		m.autoCommit.Notify(label)
	}
	return nil
}

func (m captureModel) updateDraft(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	activeID := m.activeDraftID()
	active := m.findDraftItem(activeID)

	switch msg.String() {
	case "e":
		if active != nil {
			m.modal = captureModalEditTitle
			m.openTitleModal(active.ID, active.Title)
		}
		return m, nil
	case "D":
		if active != nil {
			m.modal = captureModalEditDescription
			m.modalForDraftID = strings.TrimSpace(active.ID)
			m.openDescriptionModal(active.Description)
		}
		return m, nil
	case "A":
		m.modal = captureModalPickAssignee
		m.modalForDraftID = strings.TrimSpace(activeID)
		m.openAssigneePicker(active)
		return m, nil
	case "t":
		m.modal = captureModalEditTags
		m.modalForDraftID = strings.TrimSpace(activeID)
		m.openTagsEditor(active, "")
		return m, nil
	case " ":
		m.modal = captureModalPickStatus
		m.modalForDraftID = strings.TrimSpace(activeID)
		if active != nil {
			m.openStatusPicker(active.StatusID)
		} else {
			m.openStatusPicker("")
		}
		return m, nil
	case "d":
		m.modal = captureModalSetDue
		m.modalForDraftID = strings.TrimSpace(activeID)
		if active != nil {
			m.openDateModal(active.Due)
		} else {
			m.openDateModal(nil)
		}
		return m, nil
	case "s":
		m.modal = captureModalSetSchedule
		m.modalForDraftID = strings.TrimSpace(activeID)
		if active != nil {
			m.openDateModal(active.Schedule)
		} else {
			m.openDateModal(nil)
		}
		return m, nil
	case "p":
		if active != nil {
			active.Priority = !active.Priority
			m.setDraftItem(*active)
			m.refreshDraftList()
		}
		return m, nil
	case "o":
		if active != nil {
			active.OnHold = !active.OnHold
			m.setDraftItem(*active)
			m.refreshDraftList()
		}
		return m, nil
	case "n":
		if active != nil {
			(&m).insertDraftSiblingAfter(active.ID)
		}
		return m, nil
	case "N":
		if active != nil {
			(&m).insertDraftChild(active.ID)
		}
		return m, nil
	case "m":
		m.modal = captureModalPickOutline
		m.openOutlinePicker(m.draftOutlineID)
		return m, nil
	case "enter", "ctrl+s":
		// Require a title for every draft item (prevents accidentally creating blank items).
		for _, d := range m.draftItems {
			if strings.TrimSpace(d.Title) == "" {
				m.minibuffer = "Title is empty"
				m.refreshDraftList()
				m.selectDraftItem(d.ID)
				m.modal = captureModalEditTitle
				m.openTitleModal(d.ID, d.Title)
				return m, nil
			}
		}

		id, err := m.createDraftItems()
		if err != nil {
			m.minibuffer = err.Error()
			return m, nil
		}
		m.result = CaptureResult{Workspace: m.workspace, Dir: m.dir, ItemID: id}
		return m, m.finishCmd(false)
	}

	// Allow navigating the draft outline.
	var cmd tea.Cmd
	m.draftList, cmd = m.draftList.Update(msg)
	return m, cmd
}

func (m captureModel) activeDraftID() string {
	if it, ok := m.draftList.SelectedItem().(outlineRowItem); ok {
		return strings.TrimSpace(it.row.item.ID)
	}
	// Fallback: first draft item.
	if len(m.draftItems) > 0 {
		return strings.TrimSpace(m.draftItems[0].ID)
	}
	return ""
}

func (m *captureModel) findDraftItem(id string) *model.Item {
	if m == nil {
		return nil
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	for i := range m.draftItems {
		if strings.TrimSpace(m.draftItems[i].ID) == id {
			return &m.draftItems[i]
		}
	}
	return nil
}

func (m *captureModel) setDraftItem(it model.Item) {
	if m == nil {
		return
	}
	id := strings.TrimSpace(it.ID)
	if id == "" {
		return
	}
	for i := range m.draftItems {
		if strings.TrimSpace(m.draftItems[i].ID) == id {
			m.draftItems[i] = it
			return
		}
	}
}

func (m *captureModel) addDraftItem(parentID *string, title string) string {
	if m == nil || m.db == nil {
		return ""
	}
	outID := strings.TrimSpace(m.draftOutlineID)
	o, ok := m.db.FindOutline(outID)
	if !ok || o == nil {
		return ""
	}
	actorID := resolveWriteActorID(m.db, m.actorOverride)
	if actorID == "" {
		actorID = strings.TrimSpace(m.db.CurrentActorID)
	}

	m.draftSeq++
	id := fmt.Sprintf("draft-%d", m.draftSeq)

	pid := (*string)(nil)
	if parentID != nil {
		if v := strings.TrimSpace(*parentID); v != "" {
			tmp := v
			pid = &tmp
		}
	}

	// Append at end of sibling list within the draft.
	maxRank := ""
	for i := range m.draftItems {
		if !sameParent(m.draftItems[i].ParentID, pid) {
			continue
		}
		if r := strings.TrimSpace(m.draftItems[i].Rank); r != "" && r > maxRank {
			maxRank = r
		}
	}
	rank := ""
	if maxRank == "" {
		if r, err := store.RankInitial(); err == nil {
			rank = r
		} else {
			rank = "h"
		}
	} else {
		if r, err := store.RankAfter(maxRank); err == nil {
			rank = r
		} else {
			rank = maxRank + "0"
		}
	}

	now := time.Now().UTC()
	it := model.Item{
		ID:              id,
		ProjectID:       o.ProjectID,
		OutlineID:       o.ID,
		ParentID:        pid,
		Rank:            rank,
		Title:           strings.TrimSpace(title),
		Description:     "",
		StatusID:        store.FirstStatusID(o.StatusDefs),
		Priority:        false,
		OnHold:          false,
		Due:             nil,
		Schedule:        nil,
		Tags:            nil,
		Archived:        false,
		OwnerActorID:    actorID,
		AssignedActorID: defaultAssignedActorID(m.db, actorID),
		CreatedBy:       actorID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	m.draftItems = append(m.draftItems, it)
	m.selectDraftItem(id)
	return id
}

func (m *captureModel) selectDraftItem(id string) {
	if m == nil {
		return
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	for i := 0; i < len(m.draftList.Items()); i++ {
		if it, ok := m.draftList.Items()[i].(outlineRowItem); ok && strings.TrimSpace(it.row.item.ID) == id {
			m.draftList.Select(i)
			return
		}
	}
}

func (m *captureModel) insertDraftSiblingAfter(afterID string) {
	if m == nil || m.db == nil {
		return
	}
	after := m.findDraftItem(afterID)
	if after == nil {
		return
	}
	parentID := after.ParentID

	// Collect siblings sorted by rank.
	var sibs []*model.Item
	for i := range m.draftItems {
		if sameParent(m.draftItems[i].ParentID, parentID) {
			sibs = append(sibs, &m.draftItems[i])
		}
	}
	sort.Slice(sibs, func(i, j int) bool { return compareOutlineItems(*sibs[i], *sibs[j]) < 0 })

	idx := -1
	for i := range sibs {
		if strings.TrimSpace(sibs[i].ID) == strings.TrimSpace(afterID) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	lower := strings.TrimSpace(sibs[idx].Rank)
	upper := ""
	if idx+1 < len(sibs) {
		upper = strings.TrimSpace(sibs[idx+1].Rank)
	}

	// Generate a rank between lower/upper (or after lower if at end).
	rank := ""
	if lower != "" && upper != "" && lower < upper {
		existing := map[string]bool{}
		for i := range sibs {
			rn := strings.ToLower(strings.TrimSpace(sibs[i].Rank))
			if rn != "" {
				existing[rn] = true
			}
		}
		if r, err := store.RankBetweenUnique(existing, lower, upper); err == nil {
			rank = r
		}
	}
	if rank == "" {
		if r, err := store.RankAfter(lower); err == nil {
			rank = r
		} else {
			rank = lower + "0"
		}
	}

	m.draftSeq++
	id := fmt.Sprintf("draft-%d", m.draftSeq)
	outID := strings.TrimSpace(m.draftOutlineID)
	o, _ := m.db.FindOutline(outID)
	if o == nil {
		return
	}
	actorID := resolveWriteActorID(m.db, m.actorOverride)
	if actorID == "" {
		actorID = strings.TrimSpace(m.db.CurrentActorID)
	}
	now := time.Now().UTC()
	it := model.Item{
		ID:              id,
		ProjectID:       o.ProjectID,
		OutlineID:       o.ID,
		ParentID:        parentID,
		Rank:            rank,
		Title:           "",
		Description:     "",
		StatusID:        store.FirstStatusID(o.StatusDefs),
		Priority:        false,
		OnHold:          false,
		Due:             nil,
		Schedule:        nil,
		Tags:            nil,
		Archived:        false,
		OwnerActorID:    actorID,
		AssignedActorID: defaultAssignedActorID(m.db, actorID),
		CreatedBy:       actorID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	m.draftItems = append(m.draftItems, it)
	m.refreshDraftList()
	m.selectDraftItem(id)
	m.modal = captureModalEditTitle
	m.openTitleModal(id, "")
}

func (m *captureModel) insertDraftChild(parentID string) {
	if m == nil || m.db == nil {
		return
	}
	parentID = strings.TrimSpace(parentID)
	if parentID == "" {
		return
	}
	tmp := parentID
	m.insertDraftSiblingAfterWithParent(&tmp, "")
}

func (m *captureModel) insertDraftSiblingAfterWithParent(parentID *string, _ string) {
	// Append a new child at the end of the parent's children.
	id := m.addDraftItem(parentID, "")
	if id == "" {
		return
	}
	m.refreshDraftList()
	m.selectDraftItem(id)
	m.modal = captureModalEditTitle
	m.openTitleModal(id, "")
}

func (m *captureModel) refreshDraftList() {
	if m == nil || m.db == nil {
		return
	}
	o, ok := m.db.FindOutline(strings.TrimSpace(m.draftOutlineID))
	if !ok || o == nil {
		m.draftList.SetItems([]list.Item{})
		return
	}
	activeID := m.activeDraftID()

	flat := flattenOutline(*o, m.draftItems, m.draftCollapsed)
	items := make([]list.Item, 0, len(flat))
	for _, row := range flat {
		if row.item.AssignedActorID != nil && strings.TrimSpace(*row.item.AssignedActorID) != "" {
			row.assignedLabel = actorCompactLabel(m.db, strings.TrimSpace(*row.item.AssignedActorID))
		}
		items = append(items, outlineRowItem{row: row, outline: *o})
	}
	m.draftList.SetItems(items)
	if activeID != "" {
		m.selectDraftItem(activeID)
	} else if len(items) > 0 {
		m.draftList.Select(0)
	}
}

func (m *captureModel) createDraftItems() (string, error) {
	if m == nil || m.db == nil {
		return "", errors.New("no workspace loaded")
	}
	outID := strings.TrimSpace(m.draftOutlineID)
	if outID == "" {
		return "", errors.New("no outline selected")
	}
	if len(m.draftItems) == 0 {
		return "", errors.New("no draft items")
	}

	db, err := m.st.Load()
	if err != nil {
		return "", err
	}
	m.db = db

	out, ok := db.FindOutline(outID)
	if !ok || out == nil {
		return "", fmt.Errorf("outline not found: %s", outID)
	}

	actorID := resolveWriteActorID(db, m.actorOverride)
	if actorID == "" {
		return "", errors.New("no current actor; run `clarity identity use <actor-id>` (or pass --actor)")
	}

	// Create items in a stable parent-before-child order (pre-order traversal by rank).
	flat := flattenOutline(*out, m.draftItems, map[string]bool{})
	idMap := map[string]string{} // draft-id -> real-id
	var firstRealID string
	now := time.Now().UTC()

	for _, row := range flat {
		d := row.item
		if strings.TrimSpace(d.ID) == "" {
			continue
		}

		var parentReal *string
		if d.ParentID != nil && strings.TrimSpace(*d.ParentID) != "" {
			if rid, ok := idMap[strings.TrimSpace(*d.ParentID)]; ok && strings.TrimSpace(rid) != "" {
				tmp := strings.TrimSpace(rid)
				parentReal = &tmp
			}
		}

		statusID := strings.TrimSpace(d.StatusID)
		if !statusIDInDefs(statusID, out.StatusDefs) {
			statusID = store.FirstStatusID(out.StatusDefs)
		}

		newID := m.st.NextID(db, "item")
		newItem := model.Item{
			ID:              newID,
			ProjectID:       out.ProjectID,
			OutlineID:       out.ID,
			ParentID:        parentReal,
			Rank:            nextSiblingRank(db, out.ID, parentReal),
			Title:           strings.TrimSpace(d.Title),
			Description:     d.Description,
			StatusID:        statusID,
			Priority:        d.Priority,
			OnHold:          d.OnHold,
			Due:             d.Due,
			Schedule:        d.Schedule,
			Tags:            uniqueSortedStrings(d.Tags),
			Archived:        false,
			OwnerActorID:    actorID,
			AssignedActorID: d.AssignedActorID,
			CreatedBy:       actorID,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if newItem.AssignedActorID == nil {
			newItem.AssignedActorID = defaultAssignedActorID(db, actorID)
		}

		db.Items = append(db.Items, newItem)
		if err := m.appendEvent(actorID, "item.create", newItem.ID, newItem); err != nil {
			return "", err
		}
		idMap[strings.TrimSpace(d.ID)] = newID
		if firstRealID == "" {
			firstRealID = newID
		}
	}

	if firstRealID == "" {
		return "", errors.New("no items created")
	}
	if err := m.st.Save(db); err != nil {
		return "", err
	}
	return firstRealID, nil
}

func resolveWriteActorID(db *store.DB, override string) string {
	if db == nil {
		return ""
	}
	if strings.TrimSpace(override) != "" {
		cur := strings.TrimSpace(override)
		if humanID, ok := db.HumanUserIDForActor(cur); ok {
			if strings.TrimSpace(humanID) != "" {
				return strings.TrimSpace(humanID)
			}
		}
		return cur
	}

	cur := strings.TrimSpace(db.CurrentActorID)
	if cur == "" {
		return ""
	}
	if humanID, ok := db.HumanUserIDForActor(cur); ok {
		if strings.TrimSpace(humanID) != "" {
			return strings.TrimSpace(humanID)
		}
	}
	return cur
}

func statusIDInDefs(id string, defs []model.OutlineStatusDef) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, d := range defs {
		if strings.TrimSpace(d.ID) == id {
			return true
		}
	}
	return false
}

func (m *captureModel) updateModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.modal {
	case captureModalTemplateSearch:
		return m.updateTemplateSearchModal(msg)
	case captureModalEditTitle:
		return m.updateTitleModal(msg)
	case captureModalEditDescription:
		return m.updateDescriptionModal(msg)
	case captureModalPromptInput:
		return m.updatePromptInputModal(msg)
	case captureModalPromptTextarea:
		return m.updatePromptTextareaModal(msg)
	case captureModalPromptChoice, captureModalPromptConfirm:
		return m.updatePromptChoiceModal(msg)
	case captureModalPickOutline:
		return m.updateOutlinePicker(msg)
	case captureModalPickStatus:
		return m.updateStatusPicker(msg)
	case captureModalPickAssignee:
		return m.updateAssigneePicker(msg)
	case captureModalEditTags:
		return m.updateTagsModal(msg)
	case captureModalSetDue, captureModalSetSchedule:
		return m.updateDateModal(msg)
	case captureModalConfirmExit:
		return m.updateConfirmExitModal(msg)
	default:
		return *m, nil
	}
}

func (m captureModel) updateConfirmExitModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter", "y":
		m.canceled = true
		(&m).closeModal()
		return m, m.finishCmd(true)
	case "n", "esc", "ctrl+g":
		(&m).closeModal()
		return m, nil
	}
	return m, nil
}

func (m captureModel) updateTemplateSearchModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		sel, ok := m.templateSearchList.SelectedItem().(captureTemplateSearchItem)
		(&m).closeModal()
		if !ok {
			return m, nil
		}
		if err := (&m).beginTemplateCapture(sel.template); err != nil {
			m.minibuffer = err.Error()
			return m, nil
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.templateSearchList, cmd = m.templateSearchList.Update(msg)
	return m, cmd
}

func (m *captureModel) openTitleModal(forDraftID string, initial string) {
	m.modalForDraftID = strings.TrimSpace(forDraftID)
	m.titleInput.SetValue(strings.TrimSpace(initial))
	m.titleInput.Placeholder = "Title"
	bodyW := modalBodyWidth(m.width)
	inputW := bodyW - 2 // one space padding on each side
	if inputW < 10 {
		inputW = 10
	}
	m.titleInput.Width = inputW
	m.titleInput.Focus()
}

func (m captureModel) updateTitleModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter", "ctrl+s":
		id := strings.TrimSpace(m.modalForDraftID)
		if id == "" {
			(&m).closeModal()
			return m, nil
		}
		it := m.findDraftItem(id)
		if it == nil {
			(&m).closeModal()
			return m, nil
		}
		title := m.titleInput.Value()
		if strings.Contains(title, "{{") {
			ctx := newCaptureExpansionContext(m.workspace, m.draftOutlineID)
			ctx.Vars = m.draftVars
			title = expandCaptureTemplateString(title, ctx)
		}
		title = strings.ReplaceAll(title, "\n", " ")
		title = strings.ReplaceAll(title, "\r", " ")
		it.Title = strings.TrimSpace(title)
		(&m).setDraftItem(*it)
		(&m).refreshDraftList()
		(&m).closeModal()
		return m, nil
	}
	var cmd tea.Cmd
	m.titleInput, cmd = m.titleInput.Update(msg)
	return m, cmd
}

func (m *captureModel) openDescriptionModal(initial string) {
	m.textarea.SetValue(initial)
	m.textarea.Placeholder = "Markdown description…"
	m.textarea.Focus()
}

func (m captureModel) updateDescriptionModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+s":
		id := strings.TrimSpace(m.modalForDraftID)
		if id != "" {
			if it := m.findDraftItem(id); it != nil {
				desc := m.textarea.Value()
				if strings.Contains(desc, "{{") {
					ctx := newCaptureExpansionContext(m.workspace, m.draftOutlineID)
					ctx.Vars = m.draftVars
					desc = expandCaptureTemplateString(desc, ctx)
				}
				it.Description = desc
				(&m).setDraftItem(*it)
				(&m).refreshDraftList()
			}
		}
		(&m).closeModal()
		return m, nil
	}
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

func (m *captureModel) openOutlinePicker(selectedOutlineID string) {
	if m.db == nil {
		m.outlinePickList.SetItems([]list.Item{})
		return
	}
	// Sort by project name then outline name/id.
	projNameByID := map[string]string{}
	for _, p := range m.db.Projects {
		projNameByID[p.ID] = strings.TrimSpace(p.Name)
	}

	outs := append([]model.Outline{}, m.db.Outlines...)
	sort.Slice(outs, func(i, j int) bool {
		a, b := outs[i], outs[j]
		pa, pb := projNameByID[a.ProjectID], projNameByID[b.ProjectID]
		if pa != pb {
			return pa < pb
		}
		na, nb := captureOutlineDisplayName(a), captureOutlineDisplayName(b)
		if na != nb {
			return na < nb
		}
		return a.ID < b.ID
	})

	items := make([]list.Item, 0, len(outs))
	selected := 0
	for _, o := range outs {
		if o.Archived {
			continue
		}
		pn := strings.TrimSpace(projNameByID[o.ProjectID])
		on := captureOutlineDisplayName(o)
		label := pn + " / " + on
		items = append(items, captureOutlineItem{outline: o, label: label})
		if strings.TrimSpace(selectedOutlineID) != "" && o.ID == selectedOutlineID {
			selected = len(items) - 1
		}
	}
	m.outlinePickList.SetItems(items)
	if len(items) > 0 {
		m.outlinePickList.Select(selected)
	}
}

func captureOutlineDisplayName(o model.Outline) string {
	if o.Name != nil && strings.TrimSpace(*o.Name) != "" {
		return strings.TrimSpace(*o.Name)
	}
	return o.ID
}

func (m captureModel) updateOutlinePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		it, ok := m.outlinePickList.SelectedItem().(captureOutlineItem)
		if !ok {
			(&m).closeModal()
			return m, nil
		}
		to := strings.TrimSpace(it.outline.ID)
		if to == "" || to == strings.TrimSpace(m.draftOutlineID) {
			(&m).closeModal()
			return m, nil
		}

		// If all draft statuses are valid in the target outline, switch immediately.
		allOK := true
		for i := range m.draftItems {
			if !statusIDInDefs(m.draftItems[i].StatusID, it.outline.StatusDefs) {
				allOK = false
				break
			}
		}
		if allOK {
			m.draftOutlineID = to
			for i := range m.draftItems {
				m.draftItems[i].OutlineID = to
				m.draftItems[i].ProjectID = it.outline.ProjectID
			}
			(&m).closeModal()
			m.refreshDraftList()
			return m, nil
		}

		// Otherwise: require picking a compatible status (mirrors move-outline flow).
		m.pendingOutlineChange = to
		m.modal = captureModalPickStatus
		m.modalForDraftID = ""
		m.openStatusPicker(store.FirstStatusID(it.outline.StatusDefs))
		return m, nil
	}

	var cmd tea.Cmd
	m.outlinePickList, cmd = m.outlinePickList.Update(msg)
	return m, cmd
}

func (m *captureModel) openStatusPicker(selectedStatusID string) {
	if m.db == nil {
		m.statusPickList.SetItems([]list.Item{})
		return
	}
	outID := strings.TrimSpace(m.draftOutlineID)
	if strings.TrimSpace(m.pendingOutlineChange) != "" {
		outID = strings.TrimSpace(m.pendingOutlineChange)
	}
	o, ok := m.db.FindOutline(outID)
	if !ok || o == nil {
		m.statusPickList.SetItems([]list.Item{})
		return
	}
	items := make([]list.Item, 0, len(o.StatusDefs))
	selected := 0
	for i, d := range o.StatusDefs {
		items = append(items, captureStatusItem{def: d})
		if strings.TrimSpace(selectedStatusID) != "" && d.ID == selectedStatusID {
			selected = i
		}
	}
	m.statusPickList.SetItems(items)
	if len(items) > 0 {
		m.statusPickList.Select(selected)
	}
}

func (m captureModel) updateStatusPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		it, ok := m.statusPickList.SelectedItem().(captureStatusItem)
		if !ok {
			(&m).closeModal()
			return m, nil
		}
		chosen := strings.TrimSpace(it.def.ID)
		if strings.TrimSpace(m.pendingOutlineChange) != "" {
			(&m).applyPendingOutlineChange(chosen)
			(&m).closeModal()
			return m, nil
		}
		id := strings.TrimSpace(m.modalForDraftID)
		if id != "" {
			if d := m.findDraftItem(id); d != nil {
				d.StatusID = chosen
				(&m).setDraftItem(*d)
				(&m).refreshDraftList()
			}
		}
		(&m).closeModal()
		return m, nil
	}
	var cmd tea.Cmd
	m.statusPickList, cmd = m.statusPickList.Update(msg)
	return m, cmd
}

func (m *captureModel) applyPendingOutlineChange(chosenStatusID string) {
	if m == nil || m.db == nil {
		return
	}
	to := strings.TrimSpace(m.pendingOutlineChange)
	if to == "" {
		return
	}
	o, ok := m.db.FindOutline(to)
	if !ok || o == nil {
		m.pendingOutlineChange = ""
		return
	}
	for i := range m.draftItems {
		m.draftItems[i].OutlineID = to
		m.draftItems[i].ProjectID = o.ProjectID
		m.draftItems[i].StatusID = strings.TrimSpace(chosenStatusID)
	}
	m.draftOutlineID = to
	m.pendingOutlineChange = ""
	m.refreshDraftList()
}

func (m *captureModel) resizeLists() {
	// Render capture like a centered modal rather than full-screen UI.
	// Keep a consistent (bounded) size so hotkey capture feels focused.
	w := modalBodyWidth(m.width)
	listH := m.height - 10
	if listH > 20 {
		listH = 20
	}
	if w < 20 {
		w = 20
	}
	if listH < 6 {
		listH = 6
	}
	m.templateList.SetSize(w, listH)
	m.templateSearchList.SetSize(w, listH)
	m.promptList.SetSize(w, listH)
	m.outlinePickList.SetSize(w, listH)
	m.statusPickList.SetSize(w, listH)
	m.assigneeList.SetSize(w, listH)
	m.draftList.SetSize(w, listH)

	// Text editor modals use a smaller area.
	m.titleInput.Width = w - 4
	m.input.Width = w - 4
	m.textarea.SetWidth(w - 4)

	// For multiline editing, use as much vertical space as is available (no arbitrary cap).
	textH := m.height - 12
	if textH < 6 {
		textH = 6
	}
	m.textarea.SetHeight(textH - 4)
}

func (m captureModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading…"
	}

	body := ""
	title := ""

	switch m.phase {
	case capturePhaseSelectTemplate:
		title = "Capture: template"
		body = m.templateList.View()
		if strings.TrimSpace(m.templateHint) != "" {
			body = body + "\n\n" + m.templateHint + "\n(backspace: up  /: search  enter: select  ctrl+t: templates  esc: cancel)"
		}
	case capturePhaseEditDraft:
		title = "Capture: draft"
		body = m.renderDraftSummary()
	default:
		title = "Capture"
		body = ""
	}

	if m.modal != captureModalNone {
		return m.placeCentered(m.renderModal())
	}

	if strings.TrimSpace(m.minibuffer) != "" {
		body += "\n\n" + metaOnHoldStyle.Render(m.minibuffer)
	}

	return m.placeCentered(renderModalBox(m.width, title, body))
}

func (m captureModel) placeCentered(s string) string {
	// If the modal fills the screen, Place will naturally have no padding; otherwise it centers.
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, s)
}

func (m captureModel) renderDraftSummary() string {
	outLabel := strings.TrimSpace(m.workspace) + " / " + strings.TrimSpace(m.draftOutlineID)
	if m.db != nil {
		if o, ok := m.db.FindOutline(m.draftOutlineID); ok && o != nil {
			pn := ""
			if p, ok := m.db.FindProject(o.ProjectID); ok && p != nil {
				pn = strings.TrimSpace(p.Name)
			}
			on := outlineDisplayName(*o)
			outLabel = strings.TrimSpace(m.workspace) + " / " + pn + " / " + on
		}
	}

	bodyW := modalBodyWidth(m.width)
	header := styleMuted().Width(bodyW).Render("Target: " + outLabel)
	help := styleMuted().Width(bodyW).Render("Keys: e title  D desc  A assign  t tags  s schedule  d due  p priority  o hold  SPACE status  m move  n sibling  N subitem  ENTER/CTRL+S save  esc/ctrl+g exit")
	return strings.Join([]string{
		header,
		"",
		m.draftList.View(),
		"",
		help,
	}, "\n")
}

func (m captureModel) renderModal() string {
	switch m.modal {
	case captureModalTemplateSearch:
		return renderModalBox(m.width, "Capture: template search", m.templateSearchList.View()+"\n\nenter: select   esc/ctrl+g: cancel")
	case captureModalEditTitle:
		bodyW := modalBodyWidth(m.width)
		inputLine := renderInputLine(bodyW, m.titleInput.View())
		body := strings.Join([]string{
			inputLine,
			"",
			"enter/ctrl+s: save   esc/ctrl+g: cancel",
		}, "\n")
		return renderModalBox(m.width, "Capture: title", body)
	case captureModalEditDescription:
		bodyW := modalBodyWidth(m.width)
		srcHelp := styleMuted().Width(bodyW).Render("{{url}}: --url or $CLARITY_CAPTURE_URL    {{selection}}: --selection or $CLARITY_CAPTURE_SELECTION")
		return renderModalBox(m.width, "Capture: description", m.textarea.View()+"\n\n"+srcHelp+"\n\nctrl+s: save   esc/ctrl+g: cancel")
	case captureModalPromptInput:
		p, ok := m.currentPrompt()
		if !ok {
			return renderModalBox(m.width, "Capture: prompt", "")
		}
		return renderModalBox(m.width, "Capture: "+capturePromptLabel(p), m.input.View()+"\n\nenter/ctrl+s: next   esc/ctrl+g: cancel")
	case captureModalPromptTextarea:
		p, ok := m.currentPrompt()
		if !ok {
			return renderModalBox(m.width, "Capture: prompt", "")
		}
		return renderModalBox(m.width, "Capture: "+capturePromptLabel(p), m.textarea.View()+"\n\nctrl+s: next   esc/ctrl+g: cancel")
	case captureModalPromptChoice:
		p, ok := m.currentPrompt()
		if !ok {
			return renderModalBox(m.width, "Capture: prompt", "")
		}
		return renderModalBox(m.width, "Capture: "+capturePromptLabel(p), m.promptList.View()+"\n\nenter: select   esc/ctrl+g: cancel")
	case captureModalPromptConfirm:
		p, ok := m.currentPrompt()
		if !ok {
			return renderModalBox(m.width, "Capture: prompt", "")
		}
		return renderModalBox(m.width, "Capture: "+capturePromptLabel(p), m.promptList.View()+"\n\nenter: select   esc/ctrl+g: cancel")
	case captureModalConfirmExit:
		bodyW := modalBodyWidth(m.width)
		desc := styleMuted().Width(bodyW).Render("This draft is not saved until you submit it.")
		body := strings.Join([]string{
			"Exit capture and discard this draft?",
			desc,
		}, "\n\n")
		return renderModalBox(m.width, "Confirm", body+"\n\nenter/y: exit   esc/n: keep editing")
	case captureModalPickOutline:
		return renderModalBox(m.width, "Capture: move to outline", m.outlinePickList.View()+"\n\nenter: select   esc/ctrl+g: cancel")
	case captureModalPickStatus:
		return renderModalBox(m.width, "Capture: set status", m.statusPickList.View()+"\n\nenter: select   esc/ctrl+g: cancel")
	case captureModalPickAssignee:
		return renderModalBox(m.width, "Capture: assign", m.assigneeList.View()+"\n\nenter: set   esc/ctrl+g: cancel")
	case captureModalEditTags:
		return m.renderTagsModal()
	case captureModalSetDue:
		return m.renderDateTimeModal("Capture: due date")
	case captureModalSetSchedule:
		return m.renderDateTimeModal("Capture: schedule")
	default:
		return renderModalBox(m.width, "Capture", "")
	}
}

func (m *captureModel) openAssigneePicker(active *model.Item) {
	if m == nil || m.db == nil {
		m.assigneeList.SetItems([]list.Item{})
		return
	}

	cur := ""
	if active != nil && active.AssignedActorID != nil {
		cur = strings.TrimSpace(*active.AssignedActorID)
	}

	opts := []list.Item{assigneeOptionItem{id: "", label: ""}}
	actors := append([]model.Actor(nil), m.db.Actors...)
	sort.Slice(actors, func(i, j int) bool {
		ai := strings.ToLower(strings.TrimSpace(actors[i].Name))
		aj := strings.ToLower(strings.TrimSpace(actors[j].Name))
		if ai == aj {
			return actors[i].ID < actors[j].ID
		}
		if ai == "" {
			return false
		}
		if aj == "" {
			return true
		}
		return ai < aj
	})
	for _, a := range actors {
		lbl := actorPickerLabel(m.db, a.ID)
		opts = append(opts, assigneeOptionItem{id: a.ID, label: lbl})
	}

	m.assigneeList.Title = ""
	m.assigneeList.SetItems(opts)
	bodyW := modalBodyWidth(m.width)
	h := len(opts) + 2
	if h > 14 {
		h = 14
	}
	if h < 6 {
		h = 6
	}
	m.assigneeList.SetSize(bodyW, h)

	selected := 0
	for i := 0; i < len(opts); i++ {
		if o, ok := opts[i].(assigneeOptionItem); ok && strings.TrimSpace(o.id) == cur {
			selected = i
			break
		}
	}
	m.assigneeList.Select(selected)
}

func (m captureModel) updateAssigneePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		id := strings.TrimSpace(m.modalForDraftID)
		if id == "" {
			(&m).closeModal()
			return m, nil
		}
		d := m.findDraftItem(id)
		if d == nil {
			(&m).closeModal()
			return m, nil
		}
		pick, _ := m.assigneeList.SelectedItem().(assigneeOptionItem)
		if strings.TrimSpace(pick.id) == "" {
			d.AssignedActorID = nil
		} else {
			tmp := strings.TrimSpace(pick.id)
			d.AssignedActorID = &tmp
		}
		(&m).setDraftItem(*d)
		(&m).refreshDraftList()
		(&m).closeModal()
		return m, nil
	}
	var cmd tea.Cmd
	m.assigneeList, cmd = m.assigneeList.Update(msg)
	return m, cmd
}

func (m *captureModel) openTagsEditor(active *model.Item, preferredTag string) {
	m.refreshTagsEditor(active, preferredTag)
	m.tagsFocus = tagsFocusInput
	if m.tagsListActive != nil {
		*m.tagsListActive = false
	}
	m.input.Placeholder = "Add tag"
	m.input.SetValue("")
	m.input.Focus()
}

func (m *captureModel) refreshTagsEditor(active *model.Item, preferredTag string) {
	if m == nil || m.db == nil {
		m.tagsList.SetItems([]list.Item{})
		return
	}

	outID := strings.TrimSpace(m.draftOutlineID)
	if outID == "" {
		m.tagsList.SetItems([]list.Item{})
		return
	}

	// Collect available tags from the outline (plus current draft tags).
	all := make([]string, 0, 32)
	for _, x := range m.db.Items {
		if x.Archived {
			continue
		}
		if strings.TrimSpace(x.OutlineID) != outID {
			continue
		}
		for _, t := range x.Tags {
			t = normalizeTag(t)
			if t == "" {
				continue
			}
			all = append(all, t)
		}
	}
	for i := range m.draftItems {
		for _, t := range m.draftItems[i].Tags {
			t = normalizeTag(t)
			if t == "" {
				continue
			}
			all = append(all, t)
		}
	}
	all = uniqueSortedStrings(all)

	checked := map[string]bool{}
	if active != nil {
		for _, t := range active.Tags {
			t = normalizeTag(t)
			if t == "" {
				continue
			}
			checked[t] = true
		}
	}

	opts := make([]list.Item, 0, len(all))
	for _, tag := range all {
		opts = append(opts, tagOptionItem{tag: tag, checked: checked[tag]})
	}

	// Preserve selection when possible.
	selectedTag := strings.TrimSpace(preferredTag)
	if selectedTag == "" {
		if cur, ok := m.tagsList.SelectedItem().(tagOptionItem); ok {
			selectedTag = strings.TrimSpace(cur.tag)
		}
	}

	m.tagsList.Title = ""
	m.tagsList.SetItems(opts)
	bodyW := modalBodyWidth(m.width)
	h := len(opts) + 2
	if h > 12 {
		h = 12
	}
	if h < 4 {
		h = 4
	}
	m.tagsList.SetSize(bodyW, h)

	selectedIdx := 0
	for i := 0; i < len(opts); i++ {
		if o, ok := opts[i].(tagOptionItem); ok && strings.TrimSpace(o.tag) == selectedTag {
			selectedIdx = i
			break
		}
	}
	m.tagsList.Select(selectedIdx)
}

func (m *captureModel) setTagCheckedForDraft(itemID string, tag string, checked bool) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return
	}
	it := m.findDraftItem(itemID)
	if it == nil {
		return
	}
	tag = normalizeTag(tag)
	if tag == "" {
		return
	}
	cur := make([]string, 0, len(it.Tags)+1)
	has := false
	for _, t := range it.Tags {
		nt := normalizeTag(t)
		if nt == "" {
			continue
		}
		if nt == tag {
			has = true
			continue
		}
		cur = append(cur, nt)
	}
	if checked && !has {
		cur = append(cur, tag)
	}
	it.Tags = uniqueSortedStrings(cur)
	m.setDraftItem(*it)
}

func (m captureModel) updateTagsModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	itemID := strings.TrimSpace(m.modalForDraftID)
	active := m.findDraftItem(itemID)
	switch msg.String() {
	case "esc":
		(&m).closeModal()
		return m, nil
	case "tab":
		if m.tagsFocus == tagsFocusInput {
			m.tagsFocus = tagsFocusList
			m.input.Blur()
			if m.tagsListActive != nil {
				*m.tagsListActive = true
			}
		} else {
			m.tagsFocus = tagsFocusInput
			m.input.Focus()
			if m.tagsListActive != nil {
				*m.tagsListActive = false
			}
		}
		return m, nil
	case "shift+tab", "backtab":
		if m.tagsFocus == tagsFocusList {
			m.tagsFocus = tagsFocusInput
			m.input.Focus()
			if m.tagsListActive != nil {
				*m.tagsListActive = false
			}
		} else {
			m.tagsFocus = tagsFocusList
			m.input.Blur()
			if m.tagsListActive != nil {
				*m.tagsListActive = true
			}
		}
		return m, nil
	}

	if m.tagsFocus == tagsFocusInput {
		switch msg.String() {
		case "enter":
			tag := normalizeTag(m.input.Value())
			if tag == "" {
				return m, nil
			}
			m.setTagCheckedForDraft(itemID, tag, true)
			active = m.findDraftItem(itemID)
			m.input.SetValue("")
			m.refreshTagsEditor(active, tag)
			m.refreshDraftList()
			return m, nil
		case "down", "j", "ctrl+n":
			if len(m.tagsList.Items()) > 0 {
				m.tagsFocus = tagsFocusList
				m.input.Blur()
				if m.tagsListActive != nil {
					*m.tagsListActive = true
				}
				return m, nil
			}
		}
	} else {
		switch msg.String() {
		case "up", "k", "ctrl+p":
			if m.tagsList.Index() <= 0 {
				m.tagsFocus = tagsFocusInput
				m.input.Focus()
				if m.tagsListActive != nil {
					*m.tagsListActive = false
				}
				return m, nil
			}
		case "enter", " ":
			pick, ok := m.tagsList.SelectedItem().(tagOptionItem)
			if !ok {
				return m, nil
			}
			tag := strings.TrimSpace(pick.tag)
			if tag == "" {
				return m, nil
			}
			m.setTagCheckedForDraft(itemID, tag, !pick.checked)
			active = m.findDraftItem(itemID)
			m.refreshTagsEditor(active, tag)
			m.refreshDraftList()
			return m, nil
		}
	}

	if m.tagsFocus == tagsFocusInput {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	var cmd tea.Cmd
	m.tagsList, cmd = m.tagsList.Update(msg)
	return m, cmd
}

func (m *captureModel) renderTagsModal() string {
	bodyW := modalBodyWidth(m.width)
	inputW := bodyW - 2 // one space padding on each side
	if inputW < 10 {
		inputW = 10
	}
	m.input.Width = inputW

	inputLine := lipgloss.PlaceHorizontal(
		bodyW,
		lipgloss.Left,
		" "+m.input.View()+" ",
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceBackground(colorInputBg),
	)
	help := styleMuted().Width(bodyW).Render("tab: focus  enter (input): add  enter/space (list): toggle  esc/ctrl+g: close")
	body := strings.Join([]string{
		inputLine,
		"",
		m.tagsList.View(),
		"",
		help,
	}, "\n")
	return renderModalBox(m.width, "Capture: tags", body)
}

func (m *captureModel) openDateModal(initial *model.DateTime) {
	m.dateFocus = dateFocusYear
	m.timeEnabled = initial != nil && initial.Time != nil && strings.TrimSpace(*initial.Time) != ""

	// Seed values from existing field (if any); default to today.
	y, mo, d, h, mi := parseDateTimeFieldsOrNow(initial)
	m.yearInput.SetValue(fmt.Sprintf("%04d", y))
	m.monthInput.SetValue(fmt.Sprintf("%02d", mo))
	m.dayInput.SetValue(fmt.Sprintf("%02d", d))
	if m.timeEnabled {
		m.hourInput.SetValue(fmt.Sprintf("%02d", h))
		m.minuteInput.SetValue(fmt.Sprintf("%02d", mi))
	} else {
		m.hourInput.SetValue("")
		m.minuteInput.SetValue("")
	}

	// Style inputs similarly to other modals.
	st := lipgloss.NewStyle().Foreground(colorSurfaceFg).Background(colorInputBg)
	for _, in := range []*textinput.Model{&m.yearInput, &m.monthInput, &m.dayInput, &m.hourInput, &m.minuteInput} {
		in.Prompt = ""
		in.TextStyle = st
		in.PromptStyle = st
		in.PlaceholderStyle = styleMuted().Background(colorInputBg)
		in.CursorStyle = lipgloss.NewStyle().Foreground(colorSelectedFg).Background(colorAccent)
	}

	// Focus day by default (clear cursor elsewhere).
	m.yearInput.Blur()
	m.monthInput.Blur()
	m.dayInput.Focus()
	m.hourInput.Blur()
	m.minuteInput.Blur()
	m.dateFocus = dateFocusDay
}

func (m *captureModel) applyDateFieldFocus() {
	m.yearInput.Blur()
	m.monthInput.Blur()
	m.dayInput.Blur()
	m.hourInput.Blur()
	m.minuteInput.Blur()
	switch m.dateFocus {
	case dateFocusYear:
		m.yearInput.Focus()
	case dateFocusMonth:
		m.monthInput.Focus()
	case dateFocusDay:
		m.dayInput.Focus()
	case dateFocusTimeToggle:
		// no input focus
	case dateFocusHour:
		if m.timeEnabled {
			m.hourInput.Focus()
		}
	case dateFocusMinute:
		if m.timeEnabled {
			m.minuteInput.Focus()
		}
	}
}

func (m *captureModel) currentDatePartsOrToday() (y int, mo int, d int) {
	now := time.Now().UTC()
	y = parseIntDefault(m.yearInput.Value(), now.Year())
	mo = parseIntDefault(m.monthInput.Value(), int(now.Month()))
	d = parseIntDefault(m.dayInput.Value(), now.Day())
	if mo < 1 {
		mo = 1
	}
	if mo > 12 {
		mo = 12
	}
	d = clampDay(y, time.Month(mo), d)
	return
}

func (m *captureModel) currentTimePartsOrZero() (h int, mi int) {
	h = parseIntDefault(m.hourInput.Value(), 0)
	mi = parseIntDefault(m.minuteInput.Value(), 0)
	if h < 0 {
		h = 0
	}
	if h > 23 {
		h = 23
	}
	if mi < 0 {
		mi = 0
	}
	if mi > 59 {
		mi = 59
	}
	return
}

func (m *captureModel) bumpDateTimeField(delta int) bool {
	switch m.dateFocus {
	case dateFocusYear:
		y, mo, d := m.currentDatePartsOrToday()
		y += delta
		d = clampDay(y, time.Month(mo), d)
		m.yearInput.SetValue(fmtYear(y))
		m.monthInput.SetValue(fmt2(mo))
		m.dayInput.SetValue(fmt2(d))
		return true
	case dateFocusMonth:
		y, mo, d := m.currentDatePartsOrToday()
		mo += delta
		for mo < 1 {
			mo += 12
			y--
		}
		for mo > 12 {
			mo -= 12
			y++
		}
		d = clampDay(y, time.Month(mo), d)
		m.yearInput.SetValue(fmtYear(y))
		m.monthInput.SetValue(fmt2(mo))
		m.dayInput.SetValue(fmt2(d))
		return true
	case dateFocusDay:
		y, mo, d := m.currentDatePartsOrToday()
		cur := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC)
		next := cur.AddDate(0, 0, delta)
		m.yearInput.SetValue(fmtYear(next.Year()))
		m.monthInput.SetValue(fmt2(int(next.Month())))
		m.dayInput.SetValue(fmt2(next.Day()))
		return true
	case dateFocusHour:
		h, mi := m.currentTimePartsOrZero()
		h += delta
		for h < 0 {
			h += 24
		}
		for h >= 24 {
			h -= 24
		}
		m.hourInput.SetValue(fmt2(h))
		m.minuteInput.SetValue(fmt2(mi))
		return true
	case dateFocusMinute:
		h, mi := m.currentTimePartsOrZero()
		mi += delta
		for mi < 0 {
			mi += 60
			h--
		}
		for mi >= 60 {
			mi -= 60
			h++
		}
		for h < 0 {
			h += 24
		}
		for h >= 24 {
			h -= 24
		}
		m.hourInput.SetValue(fmt2(h))
		m.minuteInput.SetValue(fmt2(mi))
		return true
	case dateFocusTimeToggle:
		return false
	default:
		return false
	}
}

func (m captureModel) updateDateModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	itemID := strings.TrimSpace(m.modalForDraftID)
	active := m.findDraftItem(itemID)
	switch msg.String() {
	case "tab":
		switch m.dateFocus {
		case dateFocusYear:
			m.dateFocus = dateFocusMonth
		case dateFocusMonth:
			m.dateFocus = dateFocusDay
		case dateFocusDay:
			m.dateFocus = dateFocusTimeToggle
		case dateFocusTimeToggle:
			if m.timeEnabled {
				m.dateFocus = dateFocusHour
			} else {
				m.dateFocus = dateFocusSave
			}
		case dateFocusHour:
			m.dateFocus = dateFocusMinute
		case dateFocusMinute:
			m.dateFocus = dateFocusSave
		case dateFocusSave:
			m.dateFocus = dateFocusClear
		case dateFocusClear:
			m.dateFocus = dateFocusCancel
		default:
			m.dateFocus = dateFocusYear
		}
	case "shift+tab", "backtab":
		switch m.dateFocus {
		case dateFocusYear:
			m.dateFocus = dateFocusCancel
		case dateFocusCancel:
			m.dateFocus = dateFocusClear
		case dateFocusClear:
			m.dateFocus = dateFocusSave
		case dateFocusSave:
			if m.timeEnabled {
				m.dateFocus = dateFocusMinute
			} else {
				m.dateFocus = dateFocusTimeToggle
			}
		case dateFocusMinute:
			m.dateFocus = dateFocusHour
		case dateFocusHour:
			m.dateFocus = dateFocusTimeToggle
		case dateFocusTimeToggle:
			m.dateFocus = dateFocusDay
		case dateFocusDay:
			m.dateFocus = dateFocusMonth
		case dateFocusMonth:
			m.dateFocus = dateFocusYear
		default:
			m.dateFocus = dateFocusYear
		}
	case "ctrl+c":
		if active != nil {
			if m.modal == captureModalSetDue {
				active.Due = nil
			} else {
				active.Schedule = nil
			}
			(&m).setDraftItem(*active)
			(&m).refreshDraftList()
		}
		(&m).closeModal()
		return m, nil
	case "left", "h":
		switch m.dateFocus {
		case dateFocusYear:
			if m.timeEnabled {
				m.dateFocus = dateFocusMinute
			} else {
				m.dateFocus = dateFocusTimeToggle
			}
			return m, nil
		case dateFocusMonth:
			m.dateFocus = dateFocusYear
			return m, nil
		case dateFocusDay:
			m.dateFocus = dateFocusMonth
			return m, nil
		case dateFocusTimeToggle:
			m.dateFocus = dateFocusDay
			return m, nil
		case dateFocusHour:
			m.dateFocus = dateFocusDay
			return m, nil
		case dateFocusMinute:
			m.dateFocus = dateFocusHour
			return m, nil
		}
	case "right", "l":
		switch m.dateFocus {
		case dateFocusYear:
			m.dateFocus = dateFocusMonth
			return m, nil
		case dateFocusMonth:
			m.dateFocus = dateFocusDay
			return m, nil
		case dateFocusDay:
			m.dateFocus = dateFocusTimeToggle
			return m, nil
		case dateFocusTimeToggle:
			if m.timeEnabled {
				m.dateFocus = dateFocusHour
			} else {
				m.dateFocus = dateFocusSave
			}
			return m, nil
		case dateFocusHour:
			m.dateFocus = dateFocusMinute
			return m, nil
		case dateFocusMinute:
			m.dateFocus = dateFocusYear
			return m, nil
		}
	case "up", "k", "ctrl+p":
		if m.dateFocus == dateFocusTimeToggle {
			m.timeEnabled = !m.timeEnabled
			if !m.timeEnabled {
				m.hourInput.SetValue("")
				m.minuteInput.SetValue("")
			} else {
				if strings.TrimSpace(m.hourInput.Value()) == "" {
					m.hourInput.SetValue("09")
				}
				if strings.TrimSpace(m.minuteInput.Value()) == "" {
					m.minuteInput.SetValue("00")
				}
			}
			return m, nil
		}
		if m.bumpDateTimeField(+1) {
			return m, nil
		}
	case "down", "j", "ctrl+n":
		if m.dateFocus == dateFocusTimeToggle {
			m.timeEnabled = !m.timeEnabled
			if !m.timeEnabled {
				m.hourInput.SetValue("")
				m.minuteInput.SetValue("")
			} else {
				if strings.TrimSpace(m.hourInput.Value()) == "" {
					m.hourInput.SetValue("09")
				}
				if strings.TrimSpace(m.minuteInput.Value()) == "" {
					m.minuteInput.SetValue("00")
				}
			}
			return m, nil
		}
		if m.bumpDateTimeField(-1) {
			return m, nil
		}
	case " ", "t":
		if m.dateFocus == dateFocusTimeToggle {
			m.timeEnabled = !m.timeEnabled
			if !m.timeEnabled {
				m.hourInput.SetValue("")
				m.minuteInput.SetValue("")
			} else {
				if strings.TrimSpace(m.hourInput.Value()) == "" {
					m.hourInput.SetValue("09")
				}
				if strings.TrimSpace(m.minuteInput.Value()) == "" {
					m.minuteInput.SetValue("00")
				}
			}
			return m, nil
		}
	case "enter":
		if m.dateFocus == dateFocusTimeToggle {
			m.timeEnabled = !m.timeEnabled
			if !m.timeEnabled {
				m.hourInput.SetValue("")
				m.minuteInput.SetValue("")
			} else {
				if strings.TrimSpace(m.hourInput.Value()) == "" {
					m.hourInput.SetValue("09")
				}
				if strings.TrimSpace(m.minuteInput.Value()) == "" {
					m.minuteInput.SetValue("00")
				}
			}
			return m, nil
		}
		fallthrough
	case "ctrl+s":
		// If focused on cancel, treat save as cancel.
		if m.dateFocus == dateFocusCancel {
			(&m).closeModal()
			return m, nil
		}
		// If focused on clear, clear.
		if m.dateFocus == dateFocusClear {
			if active != nil {
				if m.modal == captureModalSetDue {
					active.Due = nil
				} else {
					active.Schedule = nil
				}
				(&m).setDraftItem(*active)
				(&m).refreshDraftList()
			}
			(&m).closeModal()
			return m, nil
		}

		hv := m.hourInput.Value()
		mv := m.minuteInput.Value()
		if !m.timeEnabled {
			hv = ""
			mv = ""
		}
		dt, err := parseDateTimeInputsFields(m.yearInput.Value(), m.monthInput.Value(), m.dayInput.Value(), hv, mv)
		if err != nil {
			m.minibuffer = err.Error()
			return m, nil
		}
		if active != nil {
			if m.modal == captureModalSetDue {
				active.Due = dt
			} else {
				active.Schedule = dt
			}
			(&m).setDraftItem(*active)
			(&m).refreshDraftList()
		}
		m.minibuffer = ""
		(&m).closeModal()
		return m, nil
	}

	m.applyDateFieldFocus()
	var cmd tea.Cmd
	switch m.dateFocus {
	case dateFocusYear:
		m.yearInput, cmd = m.yearInput.Update(msg)
		return m, cmd
	case dateFocusMonth:
		m.monthInput, cmd = m.monthInput.Update(msg)
		return m, cmd
	case dateFocusDay:
		m.dayInput, cmd = m.dayInput.Update(msg)
		return m, cmd
	case dateFocusHour:
		m.hourInput, cmd = m.hourInput.Update(msg)
		return m, cmd
	case dateFocusMinute:
		m.minuteInput, cmd = m.minuteInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *captureModel) renderDateTimeModal(title string) string {
	bodyW := modalBodyWidth(m.width)

	focusBtn := func(active bool) lipgloss.Style {
		if active {
			return lipgloss.NewStyle().Padding(0, 1).Foreground(colorSelectedFg).Background(colorSelectedBg).Bold(true)
		}
		return lipgloss.NewStyle().Padding(0, 1).Foreground(colorSurfaceFg).Background(colorControlBg)
	}

	renderPill := func(active bool, content string) string {
		st := lipgloss.NewStyle().Background(colorInputBg).Foreground(colorSurfaceFg)
		if active {
			st = lipgloss.NewStyle().Foreground(colorSelectedFg).Background(colorSelectedBg).Bold(true)
		}
		return st.Render(" " + content + " ")
	}

	y := renderPill(m.dateFocus == dateFocusYear, padLeft(strings.TrimSpace(m.yearInput.Value()), 4))
	mo := renderPill(m.dateFocus == dateFocusMonth, padLeft(strings.TrimSpace(m.monthInput.Value()), 2))
	da := renderPill(m.dateFocus == dateFocusDay, padLeft(strings.TrimSpace(m.dayInput.Value()), 2))

	timeToggle := "[ ] time"
	if m.timeEnabled {
		timeToggle = "[x] time"
	}
	toggleLine := renderPill(m.dateFocus == dateFocusTimeToggle, timeToggle)

	hh := renderPill(m.dateFocus == dateFocusHour, padLeft(strings.TrimSpace(m.hourInput.Value()), 2))
	min := renderPill(m.dateFocus == dateFocusMinute, padLeft(strings.TrimSpace(m.minuteInput.Value()), 2))

	save := focusBtn(m.dateFocus == dateFocusSave).Render("Save")
	clear := focusBtn(m.dateFocus == dateFocusClear).Render("Clear")
	cancel := focusBtn(m.dateFocus == dateFocusCancel).Render("Cancel")

	help := styleMuted().Width(bodyW).Render("tab: focus  h/l: prev/next  j/k or ↓/↑: -/+  enter/ctrl+s: save  ctrl+c: clear  esc/ctrl+g: cancel")

	timeLine := toggleLine
	if m.timeEnabled {
		timeLine = toggleLine + "  " + lipgloss.JoinHorizontal(lipgloss.Left, hh, ":", min)
	}

	body := strings.Join([]string{
		styleMuted().Width(bodyW).Render("Date"),
		lipgloss.JoinHorizontal(lipgloss.Left, y, "-", mo, "-", da),
		"",
		styleMuted().Width(bodyW).Render("Time (optional)"),
		timeLine,
		"",
		lipgloss.JoinHorizontal(lipgloss.Left, save, clear, cancel),
		"",
		help,
	}, "\n")

	return renderModalBox(m.width, title, body)
}
