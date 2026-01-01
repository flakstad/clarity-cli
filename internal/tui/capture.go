package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
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
)

var ErrCaptureCanceled = errors.New("capture canceled")

type CaptureResult struct {
	Workspace string
	Dir       string
	ItemID    string
}

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
	captureModalEditTitle
	captureModalEditDescription
	captureModalPickOutline
	captureModalPickStatus
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

	templateTree      *captureTemplateNode
	templatePrefix    []string
	templateList      list.Model
	templateHint      string
	outlineLabelByRef map[string]string

	workspace string
	dir       string
	st        store.Store
	db        *store.DB

	draftOutlineID string
	draftStatusID  string
	draftTitle     string
	draftDesc      string

	modal                captureModalKind
	pendingOutlineChange string

	outlinePickList list.Model
	statusPickList  list.Model

	input    textinput.Model
	textarea textarea.Model

	minibuffer string

	canceled bool
	result   CaptureResult

	autoCommit *gitrepo.DebouncedCommitter
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

func newCaptureModel(cfg *store.GlobalConfig, actorOverride string) (captureModel, error) {
	if cfg == nil {
		cfg = &store.GlobalConfig{}
	}
	if err := store.ValidateCaptureTemplates(cfg); err != nil {
		return captureModel{}, err
	}
	root := buildCaptureTemplateTree(cfg.CaptureTemplates)

	m := captureModel{
		phase:             capturePhaseSelectTemplate,
		cfg:               cfg,
		actorOverride:     actorOverride,
		templateTree:      root,
		outlineLabelByRef: map[string]string{},
	}

	m.templateList = newList("Capture Templates", "Select a template", []list.Item{})
	m.templateList.SetFilteringEnabled(false)
	m.templateList.SetShowFilter(false)
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

	m.input = textinput.New()
	m.input.Prompt = ""
	m.input.CharLimit = 256
	m.input.Width = 48

	m.textarea = textarea.New()
	m.textarea.ShowLineNumbers = false
	m.textarea.FocusedStyle.CursorLine = m.textarea.BlurredStyle.CursorLine

	m.refreshTemplateList()
	return m, nil
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

func (m captureModel) Init() tea.Cmd { return nil }

func (m captureModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeLists()
		return m, nil

	case tea.KeyMsg:
		// Global cancel.
		switch msg.String() {
		case "ctrl+g", "esc":
			if m.modal != captureModalNone {
				// Cancel modal only.
				m.modal = captureModalNone
				m.pendingOutlineChange = ""
				m.minibuffer = ""
				return m, nil
			}
			m.canceled = true
			return m, tea.Quit
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

func (m captureModel) updateTemplateSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "backspace":
		if len(m.templatePrefix) > 0 {
			m.templatePrefix = m.templatePrefix[:len(m.templatePrefix)-1]
			m.refreshTemplateList()
		}
		return m, nil
	case "enter":
		cur := m.currentTemplateNode()
		if cur != nil && cur.template != nil {
			if err := m.startDraftFromTemplate(*cur.template); err != nil {
				m.minibuffer = err.Error()
				return m, nil
			}
			return m, nil
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
			_ = m.startDraftFromTemplate(*next.template)
		}
		return m, nil
	}

	return m, nil
}

func (m *captureModel) startDraftFromTemplate(t store.CaptureTemplate) error {
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

	m.workspace = ws
	m.dir = dir
	m.st = st
	m.db = db
	if strings.TrimSpace(os.Getenv("CLARITY_AUTOCOMMIT")) != "" || strings.TrimSpace(os.Getenv("CLARITY_GIT_AUTOCOMMIT")) != "" {
		m.autoCommit = gitrepo.NewDebouncedCommitter(gitrepo.DebouncedCommitterOpts{
			WorkspaceDir: dir,
			Debounce:     2 * time.Second,
			Message:      func() string { return "clarity: auto-commit (capture)" },
		})
	}
	m.draftOutlineID = o.ID
	m.draftStatusID = store.FirstStatusID(o.StatusDefs)
	m.draftTitle = ""
	m.draftDesc = ""
	m.phase = capturePhaseEditDraft
	m.modal = captureModalEditTitle
	m.minibuffer = warn
	m.openTitleModal("")
	return nil
}

func (m captureModel) appendEvent(actorID string, eventType string, entityID string, payload any) error {
	if err := m.st.AppendEvent(actorID, eventType, entityID, payload); err != nil {
		return err
	}
	if m.autoCommit != nil {
		m.autoCommit.Notify()
	}
	return nil
}

func (m captureModel) updateDraft(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "e":
		m.modal = captureModalEditTitle
		m.openTitleModal(m.draftTitle)
		return m, nil
	case "D":
		m.modal = captureModalEditDescription
		m.openDescriptionModal(m.draftDesc)
		return m, nil
	case " ":
		m.modal = captureModalPickStatus
		m.openStatusPicker(m.draftStatusID)
		return m, nil
	case "m":
		m.modal = captureModalPickOutline
		m.openOutlinePicker(m.draftOutlineID)
		return m, nil
	case "enter":
		if strings.TrimSpace(m.draftTitle) == "" {
			m.minibuffer = "Title is empty (press e to edit)"
			return m, nil
		}
		id, err := m.createDraftItem()
		if err != nil {
			m.minibuffer = err.Error()
			return m, nil
		}
		m.result = CaptureResult{Workspace: m.workspace, Dir: m.dir, ItemID: id}
		return m, tea.Quit
	}

	return m, nil
}

func (m *captureModel) createDraftItem() (string, error) {
	if m.db == nil {
		return "", errors.New("no workspace loaded")
	}
	outID := strings.TrimSpace(m.draftOutlineID)
	if outID == "" {
		return "", errors.New("no outline selected")
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

	statusID := strings.TrimSpace(m.draftStatusID)
	if !statusIDInDefs(statusID, out.StatusDefs) {
		statusID = store.FirstStatusID(out.StatusDefs)
	}

	now := time.Now().UTC()
	newItem := model.Item{
		ID:              m.st.NextID(db, "item"),
		ProjectID:       out.ProjectID,
		OutlineID:       out.ID,
		ParentID:        nil,
		Rank:            nextSiblingRank(db, out.ID, nil),
		Title:           m.draftTitle,
		Description:     m.draftDesc,
		StatusID:        statusID,
		Priority:        false,
		OnHold:          false,
		Due:             nil,
		Schedule:        nil,
		Tags:            nil,
		Archived:        false,
		OwnerActorID:    actorID,
		AssignedActorID: defaultAssignedActorID(db, actorID),
		CreatedBy:       actorID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	db.Items = append(db.Items, newItem)
	if err := m.appendEvent(actorID, "item.create", newItem.ID, newItem); err != nil {
		return "", err
	}
	if err := m.st.Save(db); err != nil {
		return "", err
	}
	return newItem.ID, nil
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
	case captureModalEditTitle:
		return m.updateTitleModal(msg)
	case captureModalEditDescription:
		return m.updateDescriptionModal(msg)
	case captureModalPickOutline:
		return m.updateOutlinePicker(msg)
	case captureModalPickStatus:
		return m.updateStatusPicker(msg)
	default:
		return *m, nil
	}
}

func (m *captureModel) openTitleModal(initial string) {
	m.input.SetValue(strings.TrimSpace(initial))
	m.input.Placeholder = "Title"
	m.input.Focus()
}

func (m captureModel) updateTitleModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.draftTitle = strings.TrimSpace(m.input.Value())
		m.modal = captureModalNone
		m.minibuffer = ""
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
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
		m.draftDesc = m.textarea.Value()
		m.modal = captureModalNone
		m.minibuffer = ""
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
			m.modal = captureModalNone
			return m, nil
		}
		to := strings.TrimSpace(it.outline.ID)
		if to == "" || to == strings.TrimSpace(m.draftOutlineID) {
			m.modal = captureModalNone
			return m, nil
		}

		// If the current status is valid in the target outline, switch immediately.
		if statusIDInDefs(m.draftStatusID, it.outline.StatusDefs) {
			m.draftOutlineID = to
			m.modal = captureModalNone
			return m, nil
		}

		// Otherwise: require picking a compatible status (mirrors move-outline flow).
		m.pendingOutlineChange = to
		m.modal = captureModalPickStatus
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
			m.modal = captureModalNone
			m.pendingOutlineChange = ""
			return m, nil
		}
		chosen := strings.TrimSpace(it.def.ID)
		if strings.TrimSpace(m.pendingOutlineChange) != "" {
			m.draftOutlineID = strings.TrimSpace(m.pendingOutlineChange)
			m.pendingOutlineChange = ""
		}
		m.draftStatusID = chosen
		m.modal = captureModalNone
		return m, nil
	}
	var cmd tea.Cmd
	m.statusPickList, cmd = m.statusPickList.Update(msg)
	return m, cmd
}

func (m *captureModel) resizeLists() {
	w := m.width - 6
	h := m.height - 8
	if w < 20 {
		w = 20
	}
	if h < 6 {
		h = 6
	}
	m.templateList.SetSize(w, h)
	m.outlinePickList.SetSize(w, h)
	m.statusPickList.SetSize(w, h)

	// Text editor modals use a smaller area.
	m.input.Width = w - 4
	m.textarea.SetWidth(w - 4)
	m.textarea.SetHeight(h - 4)
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
			body = body + "\n\n" + m.templateHint + "\n(backspace: up  enter: select  esc: cancel)"
		}
	case capturePhaseEditDraft:
		title = "Capture: draft"
		body = m.renderDraftSummary()
	default:
		title = "Capture"
		body = ""
	}

	if m.modal != captureModalNone {
		return m.renderModal()
	}

	if strings.TrimSpace(m.minibuffer) != "" {
		body += "\n\n" + metaOnHoldStyle.Render(m.minibuffer)
	}

	return renderModalBox(m.width, title, body)
}

func (m captureModel) renderDraftSummary() string {
	outLabel := m.draftOutlineID
	statusLabel := m.draftStatusID

	if m.db != nil {
		if o, ok := m.db.FindOutline(m.draftOutlineID); ok && o != nil {
			pn := ""
			if p, ok := m.db.FindProject(o.ProjectID); ok && p != nil {
				pn = strings.TrimSpace(p.Name)
			}
			on := outlineDisplayName(*o)
			outLabel = strings.TrimSpace(m.workspace) + " / " + pn + " / " + on

			for _, sd := range o.StatusDefs {
				if sd.ID == m.draftStatusID {
					if strings.TrimSpace(sd.Label) != "" {
						statusLabel = strings.TrimSpace(sd.Label)
					}
					break
				}
			}
		}
	}

	descPreview := strings.TrimSpace(m.draftDesc)
	if descPreview == "" {
		descPreview = "(empty)"
	}
	lines := strings.Split(descPreview, "\n")
	if len(lines) > 4 {
		lines = lines[:4]
		lines = append(lines, "…")
	}
	descPreview = strings.Join(lines, "\n")

	title := strings.TrimSpace(m.draftTitle)
	if title == "" {
		title = "(empty)"
	}

	return strings.TrimSpace(fmt.Sprintf(
		"Target: %s\nStatus: %s\n\nTitle:\n%s\n\nDescription:\n%s\n\nKeys: e edit title  D edit description  SPACE status  m move outline  ENTER save  esc cancel",
		outLabel,
		statusLabel,
		title,
		descPreview,
	))
}

func (m captureModel) renderModal() string {
	switch m.modal {
	case captureModalEditTitle:
		return renderModalBox(m.width, "Capture: title", m.input.View()+"\n\nenter: save   esc/ctrl+g: cancel")
	case captureModalEditDescription:
		return renderModalBox(m.width, "Capture: description", m.textarea.View()+"\n\nctrl+s: save   esc/ctrl+g: cancel")
	case captureModalPickOutline:
		return renderModalBox(m.width, "Capture: move to outline", m.outlinePickList.View()+"\n\nenter: select   esc/ctrl+g: cancel")
	case captureModalPickStatus:
		return renderModalBox(m.width, "Capture: set status", m.statusPickList.View()+"\n\nenter: select   esc/ctrl+g: cancel")
	default:
		return renderModalBox(m.width, "Capture", "")
	}
}
