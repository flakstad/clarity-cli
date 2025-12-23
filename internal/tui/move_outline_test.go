package tui

import (
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
        tea "github.com/charmbracelet/bubbletea"
)

func TestMoveOutlinePicker_Enter_MovesItem_WhenStatusValidInTargetOutline(t *testing.T) {
        dir := t.TempDir()
        now := time.Now().UTC()

        humanID := "act-human"
        agentID := "act-agent"

        db := &store.DB{
                CurrentActorID: agentID, // simulate being "stuck" on an agent actor
                Actors: []model.Actor{
                        {ID: humanID, Kind: model.ActorKindHuman, Name: "human"},
                        {ID: agentID, Kind: model.ActorKindAgent, Name: "agent", UserID: &humanID},
                },
                Projects: []model.Project{{ID: "proj-a", Name: "P", CreatedBy: humanID, CreatedAt: now}},
                Outlines: []model.Outline{
                        {
                                ID:        "out-a",
                                ProjectID: "proj-a",
                                StatusDefs: []model.OutlineStatusDef{
                                        {ID: "todo", Label: "TODO", IsEndState: false},
                                        {ID: "done", Label: "DONE", IsEndState: true},
                                },
                                CreatedBy: humanID,
                                CreatedAt: now,
                        },
                        {
                                ID:        "out-b",
                                ProjectID: "proj-a",
                                StatusDefs: []model.OutlineStatusDef{
                                        {ID: "todo", Label: "TODO", IsEndState: false},
                                        {ID: "done", Label: "DONE", IsEndState: true},
                                },
                                CreatedBy: humanID,
                                CreatedAt: now,
                        },
                },
                Items: []model.Item{
                        {
                                ID:           "item-a",
                                ProjectID:    "proj-a",
                                OutlineID:    "out-a",
                                Rank:         "h",
                                Title:        "A",
                                StatusID:     "todo",
                                OwnerActorID: humanID, // owned by human
                                CreatedBy:    humanID,
                                CreatedAt:    now,
                                UpdatedAt:    now,
                        },
                        {
                                ID:           "item-child",
                                ProjectID:    "proj-a",
                                OutlineID:    "out-a",
                                ParentID:     strPtr("item-a"),
                                Rank:         "i",
                                Title:        "child",
                                StatusID:     "todo",
                                OwnerActorID: humanID,
                                CreatedBy:    humanID,
                                CreatedAt:    now,
                                UpdatedAt:    now,
                        },
                },
        }

        s := store.Store{Dir: dir}
        if err := s.Save(db); err != nil {
                t.Fatalf("save db: %v", err)
        }

        m := newAppModel(dir, db)
        m.view = viewOutline
        m.selectedProjectID = "proj-a"
        m.selectedOutlineID = "out-a"
        m.selectedOutline = &db.Outlines[0]
        m.collapsed = map[string]bool{}
        m.collapseInitialized = false
        m.refreshItems(db.Outlines[0])
        selectListItemByID(&m.itemsList, "item-a")

        // Open the outline picker (as 'm' does).
        mmAny, _ := m.updateOutline(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
        m2 := mmAny.(appModel)
        if m2.modal != modalPickOutline {
                t.Fatalf("expected modalPickOutline; got %v", m2.modal)
        }

        // Select out-b and confirm.
        m2.outlinePickList.Select(1)
        mmAny, _ = m2.updateOutline(tea.KeyMsg{Type: tea.KeyEnter})
        m3 := mmAny.(appModel)

        db2, err := s.Load()
        if err != nil {
                t.Fatalf("load db: %v", err)
        }
        it2, ok := db2.FindItem("item-a")
        if !ok {
                t.Fatalf("expected item-a to exist")
        }
        if it2.OutlineID != "out-b" {
                t.Fatalf("expected outline to be out-b; got %q", it2.OutlineID)
        }
        if it2.ParentID != nil {
                t.Fatalf("expected parent to be nil after move; got %v", *it2.ParentID)
        }
        if it2.StatusID != "todo" {
                t.Fatalf("expected status to remain todo; got %q (minibuffer=%q)", it2.StatusID, m3.minibufferText)
        }

        // Child should move too (and keep its parent link).
        ch2, ok := db2.FindItem("item-child")
        if !ok {
                t.Fatalf("expected item-child to exist")
        }
        if ch2.OutlineID != "out-b" {
                t.Fatalf("expected child outline to be out-b; got %q", ch2.OutlineID)
        }
        if ch2.ParentID == nil || *ch2.ParentID != "item-a" {
                got := "<nil>"
                if ch2.ParentID != nil {
                        got = *ch2.ParentID
                }
                t.Fatalf("expected child parent to remain item-a; got %s", got)
        }
}

func TestMoveOutlinePicker_WhenStatusInvalid_PromptsForStatusThenMoves(t *testing.T) {
        dir := t.TempDir()
        now := time.Now().UTC()

        humanID := "act-human"
        agentID := "act-agent"

        db := &store.DB{
                CurrentActorID: agentID,
                Actors: []model.Actor{
                        {ID: humanID, Kind: model.ActorKindHuman, Name: "human"},
                        {ID: agentID, Kind: model.ActorKindAgent, Name: "agent", UserID: &humanID},
                },
                Projects: []model.Project{{ID: "proj-a", Name: "P", CreatedBy: humanID, CreatedAt: now}},
                Outlines: []model.Outline{
                        {
                                ID:        "out-a",
                                ProjectID: "proj-a",
                                StatusDefs: []model.OutlineStatusDef{
                                        {ID: "todo", Label: "TODO", IsEndState: false},
                                        {ID: "done", Label: "DONE", IsEndState: true},
                                },
                                CreatedBy: humanID,
                                CreatedAt: now,
                        },
                        {
                                ID:        "out-b",
                                ProjectID: "proj-a",
                                StatusDefs: []model.OutlineStatusDef{
                                        {ID: "backlog", Label: "BACKLOG", IsEndState: false},
                                },
                                CreatedBy: humanID,
                                CreatedAt: now,
                        },
                },
                Items: []model.Item{{
                        ID:           "item-a",
                        ProjectID:    "proj-a",
                        OutlineID:    "out-a",
                        Rank:         "h",
                        Title:        "A",
                        StatusID:     "todo", // invalid in out-b
                        OwnerActorID: humanID,
                        CreatedBy:    humanID,
                        CreatedAt:    now,
                        UpdatedAt:    now,
                }},
        }

        s := store.Store{Dir: dir}
        if err := s.Save(db); err != nil {
                t.Fatalf("save db: %v", err)
        }

        m := newAppModel(dir, db)
        m.view = viewOutline
        m.selectedProjectID = "proj-a"
        m.selectedOutlineID = "out-a"
        m.selectedOutline = &db.Outlines[0]
        m.collapsed = map[string]bool{}
        m.collapseInitialized = false
        m.refreshItems(db.Outlines[0])
        selectListItemByID(&m.itemsList, "item-a")

        // Open outline picker.
        mmAny, _ := m.updateOutline(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
        m2 := mmAny.(appModel)
        if m2.modal != modalPickOutline {
                t.Fatalf("expected modalPickOutline; got %v", m2.modal)
        }

        // Choose out-b.
        m2.outlinePickList.Select(1)
        mmAny, _ = m2.updateOutline(tea.KeyMsg{Type: tea.KeyEnter})
        m3 := mmAny.(appModel)

        // Should now be prompting for a valid status to complete the move.
        if m3.modal != modalPickStatus {
                t.Fatalf("expected modalPickStatus; got %v", m3.modal)
        }
        if got := m3.pendingMoveOutlineTo; got != "out-b" {
                t.Fatalf("expected pendingMoveOutlineTo out-b; got %q", got)
        }

        // Only option should be BACKLOG (no "(no status)" in this flow).
        m3.statusList.Select(0)
        mmAny, _ = m3.updateOutline(tea.KeyMsg{Type: tea.KeyEnter})
        _ = mmAny.(appModel)

        db2, err := s.Load()
        if err != nil {
                t.Fatalf("load db: %v", err)
        }
        it2, ok := db2.FindItem("item-a")
        if !ok {
                t.Fatalf("expected item-a to exist")
        }
        if it2.OutlineID != "out-b" {
                t.Fatalf("expected outline to be out-b; got %q", it2.OutlineID)
        }
        if it2.StatusID != "backlog" {
                t.Fatalf("expected status to be backlog; got %q", it2.StatusID)
        }
        if it2.ParentID != nil {
                t.Fatalf("expected parent to be nil after move; got %v", *it2.ParentID)
        }
}

func TestMoveOutlinePicker_AllowsMovingToOutlineInAnotherProject(t *testing.T) {
        dir := t.TempDir()
        now := time.Now().UTC()

        humanID := "act-human"
        agentID := "act-agent"

        db := &store.DB{
                CurrentActorID: agentID, // simulate being "stuck" on an agent actor
                Actors: []model.Actor{
                        {ID: humanID, Kind: model.ActorKindHuman, Name: "human"},
                        {ID: agentID, Kind: model.ActorKindAgent, Name: "agent", UserID: &humanID},
                },
                Projects: []model.Project{
                        {ID: "proj-a", Name: "A", CreatedBy: humanID, CreatedAt: now},
                        {ID: "proj-b", Name: "B", CreatedBy: humanID, CreatedAt: now},
                },
                Outlines: []model.Outline{
                        {
                                ID:        "out-a",
                                ProjectID: "proj-a",
                                StatusDefs: []model.OutlineStatusDef{
                                        {ID: "todo", Label: "TODO", IsEndState: false},
                                        {ID: "done", Label: "DONE", IsEndState: true},
                                },
                                CreatedBy: humanID,
                                CreatedAt: now,
                        },
                        {
                                ID:        "out-b",
                                ProjectID: "proj-b",
                                StatusDefs: []model.OutlineStatusDef{
                                        {ID: "todo", Label: "TODO", IsEndState: false},
                                        {ID: "done", Label: "DONE", IsEndState: true},
                                },
                                CreatedBy: humanID,
                                CreatedAt: now,
                        },
                },
                Items: []model.Item{
                        {
                                ID:           "item-a",
                                ProjectID:    "proj-a",
                                OutlineID:    "out-a",
                                Rank:         "h",
                                Title:        "A",
                                StatusID:     "todo",
                                OwnerActorID: humanID,
                                CreatedBy:    humanID,
                                CreatedAt:    now,
                                UpdatedAt:    now,
                        },
                        {
                                ID:           "item-child",
                                ProjectID:    "proj-a",
                                OutlineID:    "out-a",
                                ParentID:     strPtr("item-a"),
                                Rank:         "i",
                                Title:        "child",
                                StatusID:     "todo",
                                OwnerActorID: humanID,
                                CreatedBy:    humanID,
                                CreatedAt:    now,
                                UpdatedAt:    now,
                        },
                },
        }

        s := store.Store{Dir: dir}
        if err := s.Save(db); err != nil {
                t.Fatalf("save db: %v", err)
        }

        m := newAppModel(dir, db)
        m.view = viewOutline
        m.selectedProjectID = "proj-a"
        m.selectedOutlineID = "out-a"
        m.selectedOutline = &db.Outlines[0]
        m.collapsed = map[string]bool{}
        m.collapseInitialized = false
        m.refreshItems(db.Outlines[0])
        selectListItemByID(&m.itemsList, "item-a")

        // Open the outline picker (as 'm' does).
        mmAny, _ := m.updateOutline(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
        m2 := mmAny.(appModel)
        if m2.modal != modalPickOutline {
                t.Fatalf("expected modalPickOutline; got %v", m2.modal)
        }

        // Select out-b (in proj-b) and confirm.
        m2.outlinePickList.Select(1)
        mmAny, _ = m2.updateOutline(tea.KeyMsg{Type: tea.KeyEnter})
        _ = mmAny.(appModel)

        db2, err := s.Load()
        if err != nil {
                t.Fatalf("load db: %v", err)
        }
        it2, ok := db2.FindItem("item-a")
        if !ok {
                t.Fatalf("expected item-a to exist")
        }
        if it2.ProjectID != "proj-b" {
                t.Fatalf("expected project to be proj-b; got %q", it2.ProjectID)
        }
        if it2.OutlineID != "out-b" {
                t.Fatalf("expected outline to be out-b; got %q", it2.OutlineID)
        }
        if it2.ParentID != nil {
                t.Fatalf("expected parent to be nil after move; got %v", *it2.ParentID)
        }

        // Child should move too (and keep its parent link).
        ch2, ok := db2.FindItem("item-child")
        if !ok {
                t.Fatalf("expected item-child to exist")
        }
        if ch2.ProjectID != "proj-b" {
                t.Fatalf("expected child project to be proj-b; got %q", ch2.ProjectID)
        }
        if ch2.OutlineID != "out-b" {
                t.Fatalf("expected child outline to be out-b; got %q", ch2.OutlineID)
        }
        if ch2.ParentID == nil || *ch2.ParentID != "item-a" {
                got := "<nil>"
                if ch2.ParentID != nil {
                        got = *ch2.ParentID
                }
                t.Fatalf("expected child parent to remain item-a; got %s", got)
        }
}

func strPtr(s string) *string { return &s }
