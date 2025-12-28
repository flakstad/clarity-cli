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
)

type captureTemplateEditState struct {
        idx   int // -1 for new
        stage captureTemplateEditStage
        tmpl  store.CaptureTemplate
}

type captureTemplateItem struct {
        idx  int
        tmpl store.CaptureTemplate
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
        target := strings.TrimSpace(i.tmpl.Target.Workspace) + "/" + strings.TrimSpace(i.tmpl.Target.OutlineID)
        return fmt.Sprintf("%s  [%s]  →  %s", name, keys, target)
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

func (m *appModel) openCaptureTemplatesModal() {
        if m == nil {
                return
        }
        m.captureTemplateEdit = nil
        m.captureTemplateDeleteIdx = -1
        m.modalForID = ""
        m.modalForKey = ""
        m.refreshCaptureTemplatesList("")
        m.modal = modalCaptureTemplates
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
                items = append(items, captureTemplateItem{idx: i, tmpl: t})
                if preferKeys != "" && strings.Join(t.Keys, "") == preferKeys {
                        selected = i
                }
        }

        m.captureTemplatesList.SetItems(items)
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
        h := "\n\nenter/e: edit   n:new   d:delete   esc/ctrl+g: close"
        return renderModalBox(m.width, "Capture templates", m.captureTemplatesList.View()+h)
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

func (m *appModel) openCaptureTemplateWorkspacePicker(selected string) {
        if m == nil {
                return
        }
        wss, err := store.ListWorkspaces()
        if err != nil {
                m.showMinibuffer("Workspaces: " + err.Error())
                return
        }
        items := make([]list.Item, 0, len(wss))
        selectedIdx := 0
        for i, name := range wss {
                items = append(items, captureTemplateWorkspaceItem{name: name})
                if strings.TrimSpace(selected) != "" && name == selected {
                        selectedIdx = i
                }
        }
        m.captureTemplateWorkspaceList.SetItems(items)
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
        for i, o := range outs {
                label := strings.TrimSpace(projName[o.ProjectID]) + " / " + outlineDisplayName(o)
                items = append(items, captureTemplateOutlineItem{outline: o, label: label})
                if strings.TrimSpace(selectedOutlineID) != "" && o.ID == selectedOutlineID {
                        selectedIdx = i
                }
        }
        m.captureTemplateOutlineList.SetItems(items)
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
