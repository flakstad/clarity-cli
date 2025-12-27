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
        m.selectedProjectID = "proj-a"
        m.selectedOutlineID = "out-a"
        m.selectedOutline = &db.Outlines[0]
        m.refreshItems(db.Outlines[0])

        // Open panel then run 'v' (cycle view mode => list+preview).
        mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
        m2 := mAny.(appModel)
        if m2.modal != modalActionPanel {
                t.Fatalf("expected modalActionPanel, got %v", m2.modal)
        }
        if m2.showPreview {
                t.Fatalf("expected showPreview=false initially")
        }

        mAny, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
        m3 := mAny.(appModel)
        if m3.modal != modalNone {
                t.Fatalf("expected modalNone after executing action, got %v", m3.modal)
        }
        if !m3.showPreview {
                t.Fatalf("expected showPreview=true after executing 'v' from action panel")
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

func TestActionPanel_GoTo_IncludesArchivedDestination(t *testing.T) {
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
        m.openActionPanel(actionPanelNav)

        out := m.renderActionPanel()
        // Labels may be truncated to fit the action panel layout (e.g. "Archiv…").
        if !strings.Contains(out, "A") || !strings.Contains(out, "Archiv") {
                t.Fatalf("expected Go to panel to include Archived destination key/label; got:\n%s", out)
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

func TestActionPanel_GoTo_ShowsRecentItemsWithDigitShortcuts(t *testing.T) {
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
                Items: []model.Item{
                        {
                                ID:           "item-a",
                                ProjectID:    "proj-a",
                                OutlineID:    "out-a",
                                Rank:         "a",
                                Title:        "A",
                                StatusID:     "todo",
                                OwnerActorID: actorID,
                                CreatedBy:    actorID,
                                CreatedAt:    now,
                                UpdatedAt:    now,
                        },
                        {
                                ID:           "item-b",
                                ProjectID:    "proj-a",
                                OutlineID:    "out-a",
                                Rank:         "b",
                                Title:        "B",
                                StatusID:     "todo",
                                OwnerActorID: actorID,
                                CreatedBy:    actorID,
                                CreatedAt:    now,
                                UpdatedAt:    now,
                        },
                        {
                                ID:           "item-c",
                                ProjectID:    "proj-a",
                                OutlineID:    "out-a",
                                Rank:         "c",
                                Title:        "C",
                                StatusID:     "todo",
                                OwnerActorID: actorID,
                                CreatedBy:    actorID,
                                CreatedAt:    now,
                                UpdatedAt:    now,
                        },
                },
        }
        if err := s.Save(db); err != nil {
                t.Fatalf("save db: %v", err)
        }

        m := newAppModel(dir, db)

        // Visit items in order: A, B, C (C should be most recent).
        if err := (&m).jumpToItemByID("item-a"); err != nil {
                t.Fatalf("jump a: %v", err)
        }
        if err := (&m).jumpToItemByID("item-b"); err != nil {
                t.Fatalf("jump b: %v", err)
        }
        if err := (&m).jumpToItemByID("item-c"); err != nil {
                t.Fatalf("jump c: %v", err)
        }

        m.openActionPanel(actionPanelNav)
        out := m.renderActionPanel()
        if !strings.Contains(out, "RECENT ITEMS") {
                t.Fatalf("expected Recent items section to render; got:\n%s", out)
        }

        // Pressing '1' should navigate to the most recent item and close the panel.
        mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
        m2 := mAny.(appModel)
        if m2.modal != modalNone {
                t.Fatalf("expected modalNone after selecting recent item; got %v", m2.modal)
        }
        if m2.view != viewItem {
                t.Fatalf("expected viewItem after selecting recent item; got %v", m2.view)
        }
        if got := strings.TrimSpace(m2.openItemID); got != "item-c" {
                t.Fatalf("expected openItemID=item-c; got %q", got)
        }
}

func TestActionPanel_DetailPane_X_ShowsFocusedItemGroupsAndItemActions(t *testing.T) {
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
        m.selectedProjectID = "proj-a"
        m.selectedOutlineID = "out-a"
        m.showPreview = true
        m.pane = paneDetail
        if o, ok := m.db.FindOutline("out-a"); ok && o != nil {
                m.selectedOutline = o
                m.refreshItems(*o)
        }
        selectListItemByID(&m.itemsList, "item-a")

        m.openActionPanel(actionPanelContext)
        out := m.renderActionPanel()

        // Should use the same focused-item grouped layout as outline pane focus.
        for _, h := range []string{"OUTLINE VIEW", "ITEM", "GLOBAL"} {
                if !strings.Contains(out, h) {
                        t.Fatalf("expected action panel to contain header %q; got:\n%s", h, out)
                }
        }
        // And include key item actions.
        for _, want := range []string{"Edit title", "Toggle priority", "Toggle on hold", "Assign", "Tags", "Add comment"} {
                if !strings.Contains(out, want) {
                        t.Fatalf("expected action panel to contain %q; got:\n%s", want, out)
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
        for _, h := range []string{"ITEM", "GLOBAL"} {
                if !strings.Contains(out, h) {
                        t.Fatalf("expected action panel to contain header %q; got:\n%s", h, out)
                }
        }
        // Key actions should be discoverable from the item view action panel.
        for _, want := range []string{"Edit title", "Edit description", "Toggle priority", "Toggle on hold", "Assign", "Tags", "Add comment", "Move to outline", "Archive item"} {
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
        for _, h := range []string{"OUTLINE VIEW", "ITEM", "GLOBAL"} {
                if !strings.Contains(out, h) {
                        t.Fatalf("expected action panel to contain header %q; got:\n%s", h, out)
                }
        }
        for _, want := range []string{"Toggle focus (outline/detail)", "Edit title", "Toggle priority", "Toggle on hold", "Assign", "Tags", "Add comment"} {
                if !strings.Contains(out, want) {
                        t.Fatalf("expected action panel to contain %q; got:\n%s", want, out)
                }
        }
}
