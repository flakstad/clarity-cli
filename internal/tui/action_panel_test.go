package tui

import (
        "strings"
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        tea "github.com/charmbracelet/bubbletea"
)

func TestActionPanel_X_OpensAndEscNavigatesStack(t *testing.T) {
        dir := t.TempDir()
        s := store.Store{Dir: dir}

        actorID := "act-human"
        now := time.Now().UTC()
        db := &store.DB{
                CurrentActorID: actorID,
                Actors:         []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "human"}},
                Projects: []model.Project{{
                        ID:        "proj-a",
                        Name:      "Project A",
                        CreatedBy: actorID,
                        CreatedAt: now,
                }},
                Outlines: []model.Outline{{
                        ID:         "out-a",
                        ProjectID:  "proj-a",
                        StatusDefs: store.DefaultOutlineStatusDefs(),
                        CreatedBy:  actorID,
                        CreatedAt:  now,
                }},
                Items: []model.Item{{
                        ID:           "item-a",
                        ProjectID:    "proj-a",
                        OutlineID:    "out-a",
                        Rank:         "h",
                        Title:        "Title",
                        StatusID:     "todo",
                        OwnerActorID: actorID,
                        CreatedBy:    actorID,
                        CreatedAt:    now,
                        UpdatedAt:    now,
                }},
        }
        if err := s.Save(db); err != nil {
                t.Fatalf("save db: %v", err)
        }

        m := newAppModel(dir, db)
        m.view = viewOutline
        m.selectedProjectID = "proj-a"
        m.selectedOutlineID = "out-a"

        // Open panel with x
        mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
        m2 := mAny.(appModel)
        if m2.modal != modalActionPanel {
                t.Fatalf("expected modalActionPanel, got %v", m2.modal)
        }
        if len(m2.actionPanelStack) != 1 || m2.actionPanelStack[0] != actionPanelContext {
                t.Fatalf("expected stack=[context], got %#v", m2.actionPanelStack)
        }

        // Enter nav subpanel with g
        mAny, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
        m3 := mAny.(appModel)
        if m3.modal != modalActionPanel {
                t.Fatalf("expected modalActionPanel, got %v", m3.modal)
        }
        if got := m3.curActionPanelKind(); got != actionPanelNav {
                t.Fatalf("expected top panel=nav, got %v", got)
        }

        // ESC goes back to root panel (still open)
        mAny, _ = m3.Update(tea.KeyMsg{Type: tea.KeyEsc})
        m4 := mAny.(appModel)
        if m4.modal != modalActionPanel {
                t.Fatalf("expected modalActionPanel after esc back, got %v", m4.modal)
        }
        if got := m4.curActionPanelKind(); got != actionPanelContext {
                t.Fatalf("expected top panel=context after esc back, got %v", got)
        }

        // ESC at root closes
        mAny, _ = m4.Update(tea.KeyMsg{Type: tea.KeyEsc})
        m5 := mAny.(appModel)
        if m5.modal != modalNone {
                t.Fatalf("expected modalNone after esc at root, got %v", m5.modal)
        }
}

func TestActionPanel_ExecutesActionAndCloses(t *testing.T) {
        dir := t.TempDir()
        s := store.Store{Dir: dir}

        actorID := "act-human"
        now := time.Now().UTC()
        db := &store.DB{
                CurrentActorID: actorID,
                Actors:         []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "human"}},
                Projects: []model.Project{{
                        ID:        "proj-a",
                        Name:      "Project A",
                        CreatedBy: actorID,
                        CreatedAt: now,
                }},
                Outlines: []model.Outline{{
                        ID:         "out-a",
                        ProjectID:  "proj-a",
                        StatusDefs: store.DefaultOutlineStatusDefs(),
                        CreatedBy:  actorID,
                        CreatedAt:  now,
                }},
        }
        if err := s.Save(db); err != nil {
                t.Fatalf("save db: %v", err)
        }

        m := newAppModel(dir, db)
        m.view = viewOutline

        // Open panel then run 'o' (toggle preview).
        mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
        m2 := mAny.(appModel)
        if m2.modal != modalActionPanel {
                t.Fatalf("expected modalActionPanel, got %v", m2.modal)
        }
        if m2.showPreview {
                t.Fatalf("expected showPreview=false initially")
        }

        mAny, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
        m3 := mAny.(appModel)
        if m3.modal != modalNone {
                t.Fatalf("expected modalNone after executing action, got %v", m3.modal)
        }
        if !m3.showPreview {
                t.Fatalf("expected showPreview=true after executing 'o' from action panel")
        }
}

func TestActionPanel_GlobalKeys_OpenPanels(t *testing.T) {
        dir := t.TempDir()
        s := store.Store{Dir: dir}

        actorID := "act-human"
        db := &store.DB{
                CurrentActorID: actorID,
                Actors:         []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "human"}},
        }
        if err := s.Save(db); err != nil {
                t.Fatalf("save db: %v", err)
        }

        m := newAppModel(dir, db)

        // Global g opens Go to.
        mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
        m2 := mAny.(appModel)
        if m2.modal != modalActionPanel {
                t.Fatalf("expected modalActionPanel, got %v", m2.modal)
        }
        if got := m2.curActionPanelKind(); got != actionPanelNav {
                t.Fatalf("expected nav panel, got %v", got)
        }

        // Global a opens Agenda commands panel.
        mAny, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
        m3 := mAny.(appModel)
        if m3.modal != modalActionPanel {
                t.Fatalf("expected modalActionPanel, got %v", m3.modal)
        }
        if got := m3.curActionPanelKind(); got != actionPanelAgenda {
                t.Fatalf("expected agenda panel, got %v", got)
        }

        // Global c opens Capture.
        mAny, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
        m4 := mAny.(appModel)
        if m4.modal != modalActionPanel {
                t.Fatalf("expected modalActionPanel, got %v", m4.modal)
        }
        if got := m4.curActionPanelKind(); got != actionPanelCapture {
                t.Fatalf("expected capture panel, got %v", got)
        }
}

func TestActionPanel_ItemFocus_ShowsGroupedSectionsWithHeaders(t *testing.T) {
        dir := t.TempDir()
        s := store.Store{Dir: dir}

        actorID := "act-human"
        now := time.Now().UTC()
        db := &store.DB{
                CurrentActorID: actorID,
                Actors:         []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "human"}},
                Projects: []model.Project{{
                        ID:        "proj-a",
                        Name:      "Project A",
                        CreatedBy: actorID,
                        CreatedAt: now,
                }},
                Outlines: []model.Outline{{
                        ID:         "out-a",
                        ProjectID:  "proj-a",
                        StatusDefs: store.DefaultOutlineStatusDefs(),
                        CreatedBy:  actorID,
                        CreatedAt:  now,
                }},
                Items: []model.Item{{
                        ID:           "item-a",
                        ProjectID:    "proj-a",
                        OutlineID:    "out-a",
                        Rank:         "h",
                        Title:        "Title",
                        StatusID:     "todo",
                        OwnerActorID: actorID,
                        CreatedBy:    actorID,
                        CreatedAt:    now,
                        UpdatedAt:    now,
                }},
        }
        if err := s.Save(db); err != nil {
                t.Fatalf("save db: %v", err)
        }

        m := newAppModel(dir, db)
        m.width = 120
        m.view = viewOutline
        m.pane = paneOutline
        m.selectedProjectID = "proj-a"
        m.selectedOutlineID = "out-a"
        m.openActionPanel(actionPanelContext)

        out := m.renderActionPanel()

        // Ensure outline-status editing is advertised in the action panel.
        if !strings.Contains(out, "Edit outline statuses") {
                t.Fatalf("expected action panel to contain outline status editor entry; got:\n%s", out)
        }

        // Descriptive section headers.
        wantHeaders := []string{
                "OUTLINE VIEW",
                "ITEM",
                "GLOBAL",
        }
        for _, h := range wantHeaders {
                if !strings.Contains(out, h) {
                        t.Fatalf("expected action panel to contain header %q; got:\n%s", h, out)
                }
        }
        if strings.Contains(out, "NAVIGATE") || strings.Contains(out, "DESTINATIONS") {
                t.Fatalf("expected focused-item action panel not to include navigate grouping header; got:\n%s", out)
        }

        // For wide layouts, we should use multiple columns, meaning at least one line
        // should contain two headers (sections are atomic and placed as whole blocks).
        foundTwoHeadersInOneLine := false
        for _, ln := range strings.Split(out, "\n") {
                seen := 0
                for _, h := range wantHeaders {
                        if strings.Contains(ln, h) {
                                seen++
                        }
                }
                if seen >= 2 {
                        foundTwoHeadersInOneLine = true
                        break
                }
        }
        if !foundTwoHeadersInOneLine {
                t.Fatalf("expected at least one multi-column line containing 2+ section headers (section blocks); got:\n%s", out)
        }

        // Since groups are placed as whole blocks, actions inside a group should be listed
        // vertically (not packed side-by-side within the same group).
        for _, ln := range strings.Split(out, "\n") {
                if strings.Contains(ln, "Open item") && strings.Contains(ln, "Toggle preview") {
                        t.Fatalf("expected group actions not to be packed into a single line; got:\n%s", out)
                }
        }
}

func TestActionPanel_ItemView_ShowsItemSectionAndItemActions(t *testing.T) {
        dir := t.TempDir()
        s := store.Store{Dir: dir}

        actorID := "act-human"
        now := time.Now().UTC()
        db := &store.DB{
                CurrentActorID: actorID,
                Actors:         []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "human"}},
                Projects: []model.Project{{
                        ID:        "proj-a",
                        Name:      "Project A",
                        CreatedBy: actorID,
                        CreatedAt: now,
                }},
                Outlines: []model.Outline{{
                        ID:         "out-a",
                        ProjectID:  "proj-a",
                        StatusDefs: store.DefaultOutlineStatusDefs(),
                        CreatedBy:  actorID,
                        CreatedAt:  now,
                }},
                Items: []model.Item{{
                        ID:           "item-a",
                        ProjectID:    "proj-a",
                        OutlineID:    "out-a",
                        Rank:         "h",
                        Title:        "Title",
                        StatusID:     "todo",
                        OwnerActorID: actorID,
                        CreatedBy:    actorID,
                        CreatedAt:    now,
                        UpdatedAt:    now,
                }},
        }
        if err := s.Save(db); err != nil {
                t.Fatalf("save db: %v", err)
        }

        m := newAppModel(dir, db)
        m.width = 120
        m.view = viewItem
        m.openItemID = "item-a"
        m.selectedProjectID = "proj-a"
        m.selectedOutlineID = "out-a"
        m.openActionPanel(actionPanelContext)

        out := m.renderActionPanel()
        for _, h := range []string{"NAVIGATE", "OTHER"} {
                if !strings.Contains(out, h) {
                        t.Fatalf("expected action panel to contain header %q; got:\n%s", h, out)
                }
        }
        // Key actions should be discoverable from the item view action panel.
        for _, want := range []string{"Copy item ref", "Copy CLI show command", "Move to outline", "Archive item"} {
                if !strings.Contains(out, want) {
                        t.Fatalf("expected action panel to contain %q; got:\n%s", want, out)
                }
        }
}

func TestActionPanel_DetailPane_FocusedItemGrouping(t *testing.T) {
        dir := t.TempDir()
        s := store.Store{Dir: dir}

        actorID := "act-human"
        now := time.Now().UTC()
        db := &store.DB{
                CurrentActorID: actorID,
                Actors:         []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "human"}},
                Projects: []model.Project{{
                        ID:        "proj-a",
                        Name:      "Project A",
                        CreatedBy: actorID,
                        CreatedAt: now,
                }},
                Outlines: []model.Outline{{
                        ID:         "out-a",
                        ProjectID:  "proj-a",
                        StatusDefs: store.DefaultOutlineStatusDefs(),
                        CreatedBy:  actorID,
                        CreatedAt:  now,
                }},
                Items: []model.Item{{
                        ID:           "item-a",
                        ProjectID:    "proj-a",
                        OutlineID:    "out-a",
                        Rank:         "h",
                        Title:        "Title",
                        StatusID:     "todo",
                        OwnerActorID: actorID,
                        CreatedBy:    actorID,
                        CreatedAt:    now,
                        UpdatedAt:    now,
                }},
        }
        if err := s.Save(db); err != nil {
                t.Fatalf("save db: %v", err)
        }

        m := newAppModel(dir, db)
        m.width = 120
        m.view = viewOutline
        m.showPreview = true
        m.pane = paneDetail
        m.selectedProjectID = "proj-a"
        m.selectedOutlineID = "out-a"
        if o, ok := m.db.FindOutline("out-a"); ok && o != nil {
                m.selectedOutline = o
                m.refreshItems(*o)
        }
        selectListItemByID(&m.itemsList, "item-a")
        m.openActionPanel(actionPanelContext)

        out := m.renderActionPanel()
        for _, h := range []string{"NAVIGATE", "OTHER"} {
                if !strings.Contains(out, h) {
                        t.Fatalf("expected action panel to contain header %q; got:\n%s", h, out)
                }
        }
        for _, want := range []string{"Toggle focus (outline/detail)", "Open item", "Toggle preview"} {
                if !strings.Contains(out, want) {
                        t.Fatalf("expected action panel to contain %q; got:\n%s", want, out)
                }
        }
}
