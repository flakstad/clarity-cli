package tui

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

type captureTemplateEditStage int

const (
	captureTemplateEditName captureTemplateEditStage = iota
	captureTemplateEditKeys
	captureTemplateEditWorkspace
	captureTemplateEditOutline
	captureTemplateEditPrompts
	captureTemplateEditDefaultTitle
	captureTemplateEditDefaultDescription
	captureTemplateEditDefaultTags
)

type captureTemplateEditState struct {
	idx   int // -1 for new
	stage captureTemplateEditStage
	tmpl  store.CaptureTemplate
}

type captureTemplatePromptEditStage int

const (
	captureTemplatePromptEditName captureTemplatePromptEditStage = iota
	captureTemplatePromptEditLabel
	captureTemplatePromptEditType
	captureTemplatePromptEditRequired
	captureTemplatePromptEditDefault
	captureTemplatePromptEditOptions
)

type captureTemplatePromptEditState struct {
	idx   int // -1 for new
	stage captureTemplatePromptEditStage
	p     store.CaptureTemplatePrompt
}

type captureTemplateItem struct {
	idx         int
	tmpl        store.CaptureTemplate
	targetLabel string
}

func (i captureTemplateItem) FilterValue() string {
	return strings.TrimSpace(i.tmpl.Name) + " " + strings.Join(i.tmpl.Keys, " ")
}
func (i captureTemplateItem) Title() string {
	name := strings.TrimSpace(i.tmpl.Name)
	if name == "" {
		name = "(unnamed)"
	}
	keys := strings.Join(i.tmpl.Keys, " ")
	if keys == "" {
		keys = "(no keys)"
	}
	target := strings.TrimSpace(i.targetLabel)
	if target == "" {
		target = strings.TrimSpace(i.tmpl.Target.Workspace) + "/" + strings.TrimSpace(i.tmpl.Target.OutlineID)
	}
	return fmt.Sprintf("%s  [%s]  %s  %s", name, keys, glyphArrow(), target)
}
func (i captureTemplateItem) Description() string { return "" }

type captureTemplateWorkspaceItem struct{ name string }

func (i captureTemplateWorkspaceItem) FilterValue() string { return strings.TrimSpace(i.name) }
func (i captureTemplateWorkspaceItem) Title() string       { return i.name }
func (i captureTemplateWorkspaceItem) Description() string { return "" }

type captureTemplateOutlineItem struct {
	outline model.Outline
	label   string
}

func (i captureTemplateOutlineItem) FilterValue() string { return strings.TrimSpace(i.label) }
func (i captureTemplateOutlineItem) Title() string       { return i.label }
func (i captureTemplateOutlineItem) Description() string { return "" }

type captureTemplatePromptItem struct {
	idx int
	p   store.CaptureTemplatePrompt
}

func (i captureTemplatePromptItem) FilterValue() string {
	return strings.TrimSpace(i.p.Name) + " " + strings.TrimSpace(i.p.Label) + " " + strings.TrimSpace(i.p.Type)
}
func (i captureTemplatePromptItem) Title() string {
	name := strings.TrimSpace(i.p.Name)
	if name == "" {
		name = "(unnamed)"
	}
	label := strings.TrimSpace(i.p.Label)
	if label == "" {
		label = name
	}
	typ := strings.TrimSpace(i.p.Type)
	if typ == "" {
		typ = "string"
	}
	req := ""
	if i.p.Required {
		req = ", required"
	}
	def := ""
	if strings.TrimSpace(i.p.Default) != "" {
		def = " default=" + fmt.Sprintf("%q", strings.TrimSpace(i.p.Default))
	}
	return fmt.Sprintf("{{%s}} (%s%s)  %s%s", name, typ, req, label, def)
}
func (i captureTemplatePromptItem) Description() string { return "" }

type captureTemplatePromptTypeItem struct {
	value string
	label string
}

func (i captureTemplatePromptTypeItem) FilterValue() string { return strings.TrimSpace(i.label) }
func (i captureTemplatePromptTypeItem) Title() string {
	if strings.TrimSpace(i.label) != "" {
		return strings.TrimSpace(i.label)
	}
	return strings.TrimSpace(i.value)
}
func (i captureTemplatePromptTypeItem) Description() string { return "" }

func (m *appModel) openCaptureTemplatesModal() {
	if m == nil {
		return
	}
	m.captureTemplateEdit = nil
	m.captureTemplateDeleteIdx = -1
	m.modalForID = ""
	m.modalForKey = ""
	m.refreshCaptureTemplatesList("")
	m.sizeCaptureTemplatesModalLists()
	m.modal = modalCaptureTemplates
}

func (m *appModel) sizeCaptureTemplatesModalLists() {
	if m == nil {
		return
	}
	modalW := m.width - 12
	if modalW > m.width-4 {
		modalW = m.width - 4
	}
	if modalW < 20 {
		modalW = 20
	}
	if modalW > 96 {
		modalW = 96
	}
	w := modalW - 6
	if w < 20 {
		w = 20
	}
	h := 14
	if m.height > 0 {
		h = m.height - 10
		if h > 16 {
			h = 16
		}
	}
	if h < 6 {
		h = 6
	}
	m.captureTemplatesList.SetSize(w, h)
	m.captureTemplateWorkspaceList.SetSize(w, h)
	m.captureTemplateOutlineList.SetSize(w, h)
	m.captureTemplatePromptsList.SetSize(w, h)
	m.captureTemplatePromptTypeList.SetSize(w, h)
	m.captureTemplatePromptRequiredList.SetSize(w, h)
}

func (m *appModel) refreshCaptureTemplatesList(preferKeys string) {
	if m == nil {
		return
	}
	cfg, err := store.LoadConfig()
	if err != nil {
		m.showMinibuffer("Capture templates: " + err.Error())
		m.captureTemplatesList.SetItems([]list.Item{})
		return
	}
	if err := store.ValidateCaptureTemplates(cfg); err != nil {
		m.showMinibuffer("Capture templates: " + err.Error())
		m.captureTemplatesList.SetItems([]list.Item{})
		return
	}

	items := make([]list.Item, 0, len(cfg.CaptureTemplates))
	selected := 0
	for i, t := range cfg.CaptureTemplates {
		items = append(items, captureTemplateItem{
			idx:         i,
			tmpl:        t,
			targetLabel: captureTemplateTargetLabel(t.Target.Workspace, t.Target.OutlineID),
		})
		if preferKeys != "" && strings.Join(t.Keys, "") == preferKeys {
			selected = i
		}
	}

	m.captureTemplatesList.SetItems(items)
	m.sizeCaptureTemplatesModalLists()
	if len(items) > 0 {
		if selected < 0 {
			selected = 0
		}
		if selected >= len(items) {
			selected = len(items) - 1
		}
		m.captureTemplatesList.Select(selected)
	}
}

func (m appModel) renderCaptureTemplatesModal() string {
	desc := "Create, edit, and delete org-capture style templates (keys + target + prompts + optional defaults)."
	h := "\n\nenter/e: edit   n:new   d:delete   esc/ctrl+g: close"
	return renderModalBox(m.width, "Capture templates", desc+"\n\n"+m.captureTemplatesList.View()+h)
}

func (m appModel) renderCaptureTemplatePromptsModal() string {
	desc := "Prompts are asked before capture starts. Answers become template variables like {{project}}."
	h := "\n\nenter/e: edit   n:new   d:delete   ctrl+j/k: reorder   ctrl+s: next   esc/ctrl+g: cancel"
	return renderModalBox(m.width, "Capture template: prompts", desc+"\n\n"+m.captureTemplatePromptsList.View()+h)
}

func (m *appModel) openCaptureTemplatePromptsModal(selectName string) {
	if m == nil || m.captureTemplateEdit == nil {
		return
	}
	m.captureTemplatePromptEdit = nil
	m.captureTemplatePromptDeleteIdx = -1
	m.modalForID = ""
	m.modalForKey = ""
	m.refreshCaptureTemplatePromptsList(selectName)
	m.sizeCaptureTemplatesModalLists()
	m.modal = modalCaptureTemplatePrompts
}

func (m *appModel) refreshCaptureTemplatePromptsList(selectName string) {
	if m == nil || m.captureTemplateEdit == nil {
		return
	}
	prompts := m.captureTemplateEdit.tmpl.Prompts
	items := make([]list.Item, 0, len(prompts))
	selected := 0
	for i, p := range prompts {
		items = append(items, captureTemplatePromptItem{idx: i, p: p})
		if selectName != "" && strings.TrimSpace(p.Name) == strings.TrimSpace(selectName) {
			selected = i
		}
	}
	m.captureTemplatePromptsList.SetItems(items)
	m.sizeCaptureTemplatesModalLists()
	if len(items) > 0 {
		if selected < 0 {
			selected = 0
		}
		if selected >= len(items) {
			selected = len(items) - 1
		}
		m.captureTemplatePromptsList.Select(selected)
	}
}

func (m *appModel) startCaptureTemplatePromptEditNew() {
	if m == nil || m.captureTemplateEdit == nil {
		return
	}
	m.captureTemplatePromptEdit = &captureTemplatePromptEditState{
		idx:   -1,
		stage: captureTemplatePromptEditName,
		p: store.CaptureTemplatePrompt{
			Type: "string",
		},
	}
	m.openInputModal(modalCaptureTemplatePromptName, "", "Prompt variable name", "")
}

func (m *appModel) startCaptureTemplatePromptEditSelected() {
	if m == nil || m.captureTemplateEdit == nil {
		return
	}
	it, ok := m.captureTemplatePromptsList.SelectedItem().(captureTemplatePromptItem)
	if !ok {
		return
	}
	m.captureTemplatePromptEdit = &captureTemplatePromptEditState{
		idx:   it.idx,
		stage: captureTemplatePromptEditName,
		p:     it.p,
	}
	m.openInputModal(modalCaptureTemplatePromptName, "", "Prompt variable name", it.p.Name)
}

func (m *appModel) openCaptureTemplatePromptTypePicker(selected string) {
	if m == nil {
		return
	}
	opts := []list.Item{
		captureTemplatePromptTypeItem{value: "string", label: "string (single-line)"},
		captureTemplatePromptTypeItem{value: "multiline", label: "multiline (textarea)"},
		captureTemplatePromptTypeItem{value: "choice", label: "choice (pick one)"},
		captureTemplatePromptTypeItem{value: "confirm", label: "confirm (yes/no)"},
	}
	m.captureTemplatePromptTypeList.Title = ""
	m.captureTemplatePromptTypeList.SetItems(opts)
	m.sizeCaptureTemplatesModalLists()
	sel := 0
	selected = strings.TrimSpace(selected)
	if selected != "" {
		for i := 0; i < len(opts); i++ {
			if it, ok := opts[i].(captureTemplatePromptTypeItem); ok && strings.TrimSpace(it.value) == selected {
				sel = i
				break
			}
		}
	}
	m.captureTemplatePromptTypeList.Select(sel)
	m.modal = modalCaptureTemplatePromptPickType
}

func (m *appModel) openCaptureTemplatePromptRequiredPicker(required bool) {
	if m == nil {
		return
	}
	opts := []list.Item{
		captureTemplatePromptTypeItem{value: "true", label: "Yes (required)"},
		captureTemplatePromptTypeItem{value: "false", label: "No (optional)"},
	}
	m.captureTemplatePromptRequiredList.Title = ""
	m.captureTemplatePromptRequiredList.SetItems(opts)
	m.sizeCaptureTemplatesModalLists()
	sel := 1
	if required {
		sel = 0
	}
	m.captureTemplatePromptRequiredList.Select(sel)
	m.modal = modalCaptureTemplatePromptPickRequired
}

func (m *appModel) openCaptureTemplatePromptOptionsModal(initial []string) {
	if m == nil {
		return
	}
	m.modal = modalCaptureTemplatePromptOptions
	m.modalForID = ""
	m.modalForKey = ""
	m.textFocus = textFocusBody

	bodyW := modalBodyWidth(m.width)
	h := m.height - 12
	if h < 6 {
		h = 6
	}
	if h > 16 {
		h = 16
	}

	m.textarea.Placeholder = "Options (one per line)"
	m.textarea.SetWidth(bodyW)
	m.textarea.SetHeight(h)
	m.textarea.SetValue(strings.Join(initial, "\n"))
	m.textarea.Focus()
}

func (m *appModel) saveCaptureTemplatePromptEdit() error {
	if m == nil || m.captureTemplateEdit == nil || m.captureTemplatePromptEdit == nil {
		return nil
	}
	pe := *m.captureTemplatePromptEdit

	prompts := append([]store.CaptureTemplatePrompt{}, m.captureTemplateEdit.tmpl.Prompts...)
	if pe.idx < 0 {
		prompts = append(prompts, pe.p)
	} else if pe.idx < len(prompts) {
		prompts[pe.idx] = pe.p
	} else {
		return fmt.Errorf("prompt edit index out of range: %d", pe.idx)
	}
	if err := store.ValidateCaptureTemplatePrompts(prompts); err != nil {
		return err
	}
	m.captureTemplateEdit.tmpl.Prompts = prompts
	return nil
}

func (m *appModel) deleteSelectedCaptureTemplatePrompt() {
	if m == nil || m.captureTemplateEdit == nil {
		return
	}
	it, ok := m.captureTemplatePromptsList.SelectedItem().(captureTemplatePromptItem)
	if !ok {
		return
	}
	m.captureTemplatePromptDeleteIdx = it.idx
	m.modalForID = strings.TrimSpace(it.p.Name)
	m.modalForKey = strings.TrimSpace(it.p.Type)
	m.modal = modalConfirmDeleteCaptureTemplatePrompt
}

func (m *appModel) confirmDeleteCaptureTemplatePrompt() error {
	if m == nil || m.captureTemplateEdit == nil {
		return nil
	}
	idx := m.captureTemplatePromptDeleteIdx
	m.captureTemplatePromptDeleteIdx = -1
	if idx < 0 || idx >= len(m.captureTemplateEdit.tmpl.Prompts) {
		return nil
	}
	prompts := append([]store.CaptureTemplatePrompt{}, m.captureTemplateEdit.tmpl.Prompts...)
	prompts = append(prompts[:idx], prompts[idx+1:]...)
	if err := store.ValidateCaptureTemplatePrompts(prompts); err != nil {
		return err
	}
	m.captureTemplateEdit.tmpl.Prompts = prompts
	return nil
}

func (m *appModel) moveCaptureTemplatePrompt(delta int) error {
	if m == nil || m.captureTemplateEdit == nil || delta == 0 {
		return nil
	}
	it, ok := m.captureTemplatePromptsList.SelectedItem().(captureTemplatePromptItem)
	if !ok {
		return nil
	}
	idx := it.idx
	nextIdx := idx + delta
	if idx < 0 || nextIdx < 0 || nextIdx >= len(m.captureTemplateEdit.tmpl.Prompts) {
		return nil
	}
	prompts := m.captureTemplateEdit.tmpl.Prompts
	prompts[idx], prompts[nextIdx] = prompts[nextIdx], prompts[idx]
	m.captureTemplateEdit.tmpl.Prompts = prompts
	return nil
}

func parseCaptureTemplatePromptOptionsInput(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		out = append(out, ln)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (m *appModel) startCaptureTemplateEditNew() {
	if m == nil {
		return
	}
	m.captureTemplateEdit = &captureTemplateEditState{
		idx:   -1,
		stage: captureTemplateEditName,
		tmpl:  store.CaptureTemplate{},
	}
	m.openInputModal(modalCaptureTemplateName, "", "Template name", "")
}

func (m *appModel) startCaptureTemplateEditSelected() {
	if m == nil {
		return
	}
	it, ok := m.captureTemplatesList.SelectedItem().(captureTemplateItem)
	if !ok {
		return
	}
	m.captureTemplateEdit = &captureTemplateEditState{
		idx:   it.idx,
		stage: captureTemplateEditName,
		tmpl:  it.tmpl,
	}
	m.openInputModal(modalCaptureTemplateName, "", "Template name", it.tmpl.Name)
}

func parseCaptureKeysInput(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("keys is empty")
	}
	// Allow either:
	// - whitespace-separated: "w i"
	// - compact: "wi" (split into runes)
	if strings.ContainsAny(s, " \t") {
		parts := strings.Fields(s)
		return store.NormalizeCaptureTemplateKeys(parts)
	}
	parts := make([]string, 0, len([]rune(s)))
	for _, r := range []rune(s) {
		parts = append(parts, string(r))
	}
	return store.NormalizeCaptureTemplateKeys(parts)
}

func parseCaptureTemplateTagsInput(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	if len(parts) == 0 {
		return nil
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.TrimPrefix(p, "#")
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (m *appModel) openCaptureTemplateDefaultDescriptionModal(initial string) {
	if m == nil {
		return
	}
	m.modal = modalCaptureTemplateDefaultDescription
	m.modalForID = ""
	m.modalForKey = ""
	m.textFocus = textFocusBody

	bodyW := modalBodyWidth(m.width)
	h := m.height - 12
	if h < 6 {
		h = 6
	}
	if h > 16 {
		h = 16
	}

	m.textarea.Placeholder = "Default description (optional)"
	m.textarea.SetWidth(bodyW)
	m.textarea.SetHeight(h)
	m.textarea.SetValue(initial)
	m.textarea.Focus()
}

func (m *appModel) openCaptureTemplateWorkspacePicker(selected string) {
	if m == nil {
		return
	}
	ents, err := store.ListWorkspaceEntries()
	if err != nil {
		m.showMinibuffer("Workspaces: " + err.Error())
		return
	}
	// Ensure we always have at least the current/default workspace as an option.
	cur := strings.TrimSpace(m.workspace)
	if cur == "" {
		if cfg, err := store.LoadConfig(); err == nil {
			cur = strings.TrimSpace(cfg.CurrentWorkspace)
		}
	}
	if cur == "" {
		cur = "default"
	}
	seen := map[string]bool{}
	names := []string{}
	for _, e := range ents {
		n := strings.TrimSpace(e.Name)
		if n == "" || seen[n] {
			continue
		}
		if e.Archived && n != cur {
			continue
		}
		seen[n] = true
		names = append(names, n)
	}
	if !seen[cur] {
		names = append([]string{cur}, names...)
	}

	items := make([]list.Item, 0, len(names))
	selectedIdx := 0
	for i, name := range names {
		items = append(items, captureTemplateWorkspaceItem{name: name})
		if strings.TrimSpace(selected) != "" && name == selected {
			selectedIdx = i
		}
	}
	m.captureTemplateWorkspaceList.SetItems(items)
	m.sizeCaptureTemplatesModalLists()
	if len(items) > 0 {
		m.captureTemplateWorkspaceList.Select(selectedIdx)
	}
	m.modal = modalCaptureTemplatePickWorkspace
}

func (m *appModel) openCaptureTemplateOutlinePicker(workspace, selectedOutlineID string) {
	if m == nil {
		return
	}
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		m.showMinibuffer("Outline pick: no workspace selected")
		return
	}
	dir, err := store.WorkspaceDir(workspace)
	if err != nil {
		m.showMinibuffer("Outline pick: " + err.Error())
		return
	}
	st := store.Store{Dir: dir}
	db, err := st.Load()
	if err != nil {
		m.showMinibuffer("Outline pick: " + err.Error())
		return
	}

	projName := map[string]string{}
	for _, p := range db.Projects {
		projName[p.ID] = strings.TrimSpace(p.Name)
	}
	outs := append([]model.Outline{}, db.Outlines...)
	sort.Slice(outs, func(i, j int) bool {
		a, b := outs[i], outs[j]
		pa, pb := projName[a.ProjectID], projName[b.ProjectID]
		if pa != pb {
			return pa < pb
		}
		na, nb := outlineDisplayName(a), outlineDisplayName(b)
		if na != nb {
			return na < nb
		}
		return a.ID < b.ID
	})

	items := make([]list.Item, 0, len(outs))
	selectedIdx := 0
	for _, o := range outs {
		if o.Archived {
			continue
		}
		label := strings.TrimSpace(projName[o.ProjectID]) + " / " + outlineDisplayName(o)
		items = append(items, captureTemplateOutlineItem{outline: o, label: label})
		if strings.TrimSpace(selectedOutlineID) != "" && o.ID == selectedOutlineID {
			selectedIdx = len(items) - 1
		}
	}
	m.captureTemplateOutlineList.SetItems(items)
	m.sizeCaptureTemplatesModalLists()
	if len(items) > 0 {
		m.captureTemplateOutlineList.Select(selectedIdx)
	}
	m.modal = modalCaptureTemplatePickOutline
}

func (m *appModel) saveCaptureTemplateEdit() error {
	if m == nil || m.captureTemplateEdit == nil {
		return nil
	}
	cfg, err := store.LoadConfig()
	if err != nil {
		return err
	}
	if cfg == nil {
		cfg = &store.GlobalConfig{}
	}

	templates := append([]store.CaptureTemplate{}, cfg.CaptureTemplates...)
	if m.captureTemplateEdit.idx < 0 {
		templates = append(templates, m.captureTemplateEdit.tmpl)
	} else if m.captureTemplateEdit.idx < len(templates) {
		templates[m.captureTemplateEdit.idx] = m.captureTemplateEdit.tmpl
	} else {
		return fmt.Errorf("edit index out of range: %d", m.captureTemplateEdit.idx)
	}
	cfg.CaptureTemplates = templates
	if err := store.ValidateCaptureTemplates(cfg); err != nil {
		return err
	}
	return store.SaveConfig(cfg)
}

func (m appModel) renderConfirmDeleteCaptureTemplatePromptModal() string {
	name := strings.TrimSpace(m.modalForID)
	typ := strings.TrimSpace(m.modalForKey)
	if name == "" {
		name = "this prompt"
	} else {
		name = fmt.Sprintf("{{%s}}", name)
	}
	body := fmt.Sprintf("Delete %s?\n\nType: %s\n\nenter/y: delete   esc/n: cancel", name, typ)
	return renderModalBox(m.width, "Confirm", body)
}

func (m *appModel) updateCaptureTemplatePromptsModal(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m == nil {
		return appModel{}, nil
	}
	switch km := msg.(type) {
	case tea.KeyMsg:
		switch km.String() {
		case "esc", "ctrl+g":
			// Cancel template editing.
			m.captureTemplateEdit = nil
			m.captureTemplatePromptEdit = nil
			m.captureTemplatePromptDeleteIdx = -1
			m.modalForID = ""
			m.modalForKey = ""
			m.modal = modalCaptureTemplates
			return *m, nil
		case "ctrl+s":
			if m.captureTemplateEdit == nil {
				m.modal = modalCaptureTemplates
				return *m, nil
			}
			m.captureTemplateEdit.stage = captureTemplateEditDefaultTitle
			m.openInputModal(modalCaptureTemplateDefaultTitle, "", "Default title (optional)", m.captureTemplateEdit.tmpl.Defaults.Title)
			return *m, nil
		case "n":
			m.startCaptureTemplatePromptEditNew()
			return *m, nil
		case "e", "enter":
			m.startCaptureTemplatePromptEditSelected()
			return *m, nil
		case "d":
			m.deleteSelectedCaptureTemplatePrompt()
			return *m, nil
		case "ctrl+k":
			if err := m.moveCaptureTemplatePrompt(-1); err != nil {
				m.showMinibuffer("Reorder failed: " + err.Error())
				return *m, nil
			}
			nm := *m
			nm.refreshCaptureTemplatePromptsList("")
			nm.captureTemplatePromptsList.CursorUp()
			return nm, nil
		case "ctrl+j":
			if err := m.moveCaptureTemplatePrompt(+1); err != nil {
				m.showMinibuffer("Reorder failed: " + err.Error())
				return *m, nil
			}
			nm := *m
			nm.refreshCaptureTemplatePromptsList("")
			nm.captureTemplatePromptsList.CursorDown()
			return nm, nil
		}
	}
	var cmd tea.Cmd
	m.captureTemplatePromptsList, cmd = m.captureTemplatePromptsList.Update(msg)
	return *m, cmd
}

func (m *appModel) deleteSelectedCaptureTemplate() {
	if m == nil {
		return
	}
	it, ok := m.captureTemplatesList.SelectedItem().(captureTemplateItem)
	if !ok {
		return
	}
	m.captureTemplateDeleteIdx = it.idx
	m.modalForID = strings.TrimSpace(it.tmpl.Name)
	m.modalForKey = strings.Join(it.tmpl.Keys, " ")
	m.modal = modalConfirmDeleteCaptureTemplate
}

func (m *appModel) confirmDeleteCaptureTemplate() error {
	if m == nil {
		return nil
	}
	idx := m.captureTemplateDeleteIdx
	m.captureTemplateDeleteIdx = -1

	cfg, err := store.LoadConfig()
	if err != nil {
		return err
	}
	if cfg == nil {
		cfg = &store.GlobalConfig{}
	}
	if idx < 0 || idx >= len(cfg.CaptureTemplates) {
		return nil
	}
	cfg.CaptureTemplates = append(cfg.CaptureTemplates[:idx], cfg.CaptureTemplates[idx+1:]...)
	if err := store.ValidateCaptureTemplates(cfg); err != nil {
		return err
	}
	return store.SaveConfig(cfg)
}

func (m appModel) renderConfirmDeleteCaptureTemplateModal() string {
	name := strings.TrimSpace(m.modalForID)
	keys := strings.TrimSpace(m.modalForKey)
	if name == "" {
		name = "this template"
	} else {
		name = fmt.Sprintf("%q", name)
	}
	body := fmt.Sprintf("Delete %s?\n\nKeys: %s\n\nenter/y: delete   esc/n: cancel", name, keys)
	return renderModalBox(m.width, "Confirm", body)
}

func (m *appModel) updateCaptureTemplatesModal(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m == nil {
		return appModel{}, nil
	}
	switch km := msg.(type) {
	case tea.KeyMsg:
		switch km.String() {
		case "esc", "ctrl+g":
			if m.returnToCaptureAfterTemplates && m.capture != nil {
				m.returnToCaptureAfterTemplates = false
				cfg, err := store.LoadConfig()
				if err != nil {
					m.showMinibuffer("Capture templates: " + err.Error())
				} else {
					m.capture.reloadTemplates(cfg)
				}
				m.modal = modalCapture
				return *m, nil
			}
			m.modal = modalNone
			return *m, nil
		case "n":
			m.startCaptureTemplateEditNew()
			return *m, nil
		case "e", "enter":
			m.startCaptureTemplateEditSelected()
			return *m, nil
		case "d":
			m.deleteSelectedCaptureTemplate()
			return *m, nil
		}
	}
	var cmd tea.Cmd
	m.captureTemplatesList, cmd = m.captureTemplatesList.Update(msg)
	return *m, cmd
}

func captureTemplateTargetLabel(workspace, outlineID string) string {
	ws := strings.TrimSpace(workspace)
	oid := strings.TrimSpace(outlineID)
	if ws == "" || oid == "" {
		return strings.TrimSpace(ws) + "/" + strings.TrimSpace(oid)
	}
	// Best-effort: resolve outline name (and project name) from the target workspace.
	dir, err := store.WorkspaceDir(ws)
	if err != nil {
		return ws + "/" + oid
	}
	st := store.Store{Dir: dir}
	db, err := st.Load()
	if err != nil || db == nil {
		return ws + "/" + oid
	}
	o, ok := db.FindOutline(oid)
	if !ok || o == nil {
		return ws + "/" + oid
	}
	pn := ""
	if p, ok := db.FindProject(o.ProjectID); ok && p != nil {
		pn = strings.TrimSpace(p.Name)
	}
	on := outlineDisplayName(*o)
	if strings.TrimSpace(pn) != "" {
		return ws + " / " + pn + " / " + on
	}
	return ws + " / " + on
}

func startCaptureItemFromTemplate(m appModel, t store.CaptureTemplate) (appModel, tea.Cmd) {
	ws := strings.TrimSpace(t.Target.Workspace)
	if ws == "" {
		m.showMinibuffer("Capture: template target workspace is empty")
		return m, nil
	}

	if strings.TrimSpace(m.workspace) != ws {
		nm, err := m.switchWorkspaceTo(ws)
		if err != nil {
			m.showMinibuffer("Workspace error: " + err.Error())
			return m, nil
		}
		m = nm
	}

	if m.db == nil {
		m.showMinibuffer("Capture: no workspace loaded")
		return m, nil
	}

	oid := strings.TrimSpace(t.Target.OutlineID)
	if oid == "" {
		m.showMinibuffer("Capture: template target outline is empty")
		return m, nil
	}
	o, ok := m.db.FindOutline(oid)
	if !ok || o == nil {
		m.showMinibuffer("Capture: outline not found")
		return m, nil
	}
	if o.Archived {
		m.showMinibuffer("Capture: outline is archived")
		return m, nil
	}

	m.view = viewOutline
	m.pane = paneOutline
	m.showPreview = false
	m.openItemID = ""
	m.itemArchivedReadOnly = false

	m.selectedProjectID = o.ProjectID
	m.selectedOutlineID = o.ID
	m.selectedOutline = o
	m.refreshItems(*o)

	// Start a normal "new root item" flow in that outline.
	m.openInputModal(modalNewSibling, "", "Title", "")
	return m, nil
}
