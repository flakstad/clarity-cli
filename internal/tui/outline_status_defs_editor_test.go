package tui

import (
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
        tea "github.com/charmbracelet/bubbletea"
)

func TestOutlineStatusDefsEditor_AddRenameToggleReorderRemove(t *testing.T) {
        t.Parallel()

        dir := t.TempDir()
        now := time.Now().UTC()

        actorID := "act-human"
        projectID := "proj-a"
        outlineID := "out-a"

        db := &store.DB{
                Version:          1,
                CurrentActorID:   actorID,
                CurrentProjectID: projectID,
                NextIDs:          map[string]int{},
                Actors:           []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "Human"}},
                Projects:         []model.Project{{ID: projectID, Name: "P", CreatedBy: actorID, CreatedAt: now}},
                Outlines: []model.Outline{{
                        ID:         outlineID,
                        ProjectID:  projectID,
                        Name:       nil,
                        StatusDefs: store.DefaultOutlineStatusDefs(),
                        CreatedBy:  actorID,
                        CreatedAt:  now,
                }},
                Items:    []model.Item{},
                Deps:     []model.Dependency{},
                Comments: []model.Comment{},
                Worklog:  []model.WorklogEntry{},
        }

        s := store.Store{Dir: dir}
        if err := s.Save(db); err != nil {
                t.Fatalf("save db: %v", err)
        }

        m := newAppModel(dir, db)
        m.view = viewOutline
        m.selectedProjectID = projectID
        m.selectedOutlineID = outlineID
        m.selectedOutline = &db.Outlines[0]

        // Open editor via "S".
        mAny, _ := m.updateOutline(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
        m = mAny.(appModel)
        if m.modal != modalEditOutlineStatuses {
                t.Fatalf("expected modalEditOutlineStatuses, got %v", m.modal)
        }
        if got := len(m.outlineStatusDefsList.Items()); got != 3 {
                t.Fatalf("expected 3 default statuses, got %d", got)
        }

        // Add a status.
        mAny, _ = m.updateOutline(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
        m = mAny.(appModel)
        if m.modal != modalAddOutlineStatus {
                t.Fatalf("expected modalAddOutlineStatus, got %v", m.modal)
        }
        m.input.SetValue("IN REVIEW")
        mAny, _ = m.updateOutline(tea.KeyMsg{Type: tea.KeyEnter})
        m = mAny.(appModel)
        if m.modal != modalEditOutlineStatuses {
                t.Fatalf("expected to return to editor, got %v", m.modal)
        }

        db2, err := s.Load()
        if err != nil {
                t.Fatalf("load db: %v", err)
        }
        o2, ok := db2.FindOutline(outlineID)
        if !ok {
                t.Fatalf("expected outline to exist")
        }
        if len(o2.StatusDefs) != 4 {
                t.Fatalf("expected 4 statuses after add, got %d", len(o2.StatusDefs))
        }
        if o2.StatusDefs[3].ID != "in-review" {
                t.Fatalf("expected new status id in-review, got %q", o2.StatusDefs[3].ID)
        }

        // Select the new status and rename it.
        m.outlineStatusDefsList.Select(3)
        mAny, _ = m.updateOutline(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
        m = mAny.(appModel)
        if m.modal != modalRenameOutlineStatus {
                t.Fatalf("expected modalRenameOutlineStatus, got %v", m.modal)
        }
        m.input.SetValue("REVIEW")
        mAny, _ = m.updateOutline(tea.KeyMsg{Type: tea.KeyEnter})
        m = mAny.(appModel)
        if m.modal != modalEditOutlineStatuses {
                t.Fatalf("expected to return to editor, got %v", m.modal)
        }

        db3, err := s.Load()
        if err != nil {
                t.Fatalf("load db: %v", err)
        }
        o3, _ := db3.FindOutline(outlineID)
        if got := o3.StatusDefs[3].Label; got != "REVIEW" {
                t.Fatalf("expected label REVIEW, got %q", got)
        }

        // Toggle end-state on the new status.
        mAny, _ = m.updateOutline(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
        m = mAny.(appModel)
        db4, err := s.Load()
        if err != nil {
                t.Fatalf("load db: %v", err)
        }
        o4, _ := db4.FindOutline(outlineID)
        if !o4.StatusDefs[3].IsEndState {
                t.Fatalf("expected status to become end-state")
        }

        // Reorder: move the new status up one slot.
        mAny, _ = m.updateOutline(tea.KeyMsg{Type: tea.KeyCtrlK})
        m = mAny.(appModel)
        db5, err := s.Load()
        if err != nil {
                t.Fatalf("load db: %v", err)
        }
        o5, _ := db5.FindOutline(outlineID)
        if got := o5.StatusDefs[2].ID; got != "in-review" {
                t.Fatalf("expected moved status at index 2, got %q", got)
        }

        // Remove it.
        // Refresh selection to wherever it is now (index 2).
        m.outlineStatusDefsList.Select(2)
        mAny, _ = m.updateOutline(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
        m = mAny.(appModel)
        db6, err := s.Load()
        if err != nil {
                t.Fatalf("load db: %v", err)
        }
        o6, _ := db6.FindOutline(outlineID)
        if got := len(o6.StatusDefs); got != 3 {
                t.Fatalf("expected 3 statuses after remove, got %d", got)
        }
}

func TestOutlineStatusDefsEditor_RemoveBlockedWhenInUse(t *testing.T) {
        t.Parallel()

        dir := t.TempDir()
        now := time.Now().UTC()

        actorID := "act-human"
        projectID := "proj-a"
        outlineID := "out-a"

        db := &store.DB{
                Version:          1,
                CurrentActorID:   actorID,
                CurrentProjectID: projectID,
                NextIDs:          map[string]int{},
                Actors:           []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "Human"}},
                Projects:         []model.Project{{ID: projectID, Name: "P", CreatedBy: actorID, CreatedAt: now}},
                Outlines: []model.Outline{{
                        ID:         outlineID,
                        ProjectID:  projectID,
                        Name:       nil,
                        StatusDefs: store.DefaultOutlineStatusDefs(),
                        CreatedBy:  actorID,
                        CreatedAt:  now,
                }},
                Items: []model.Item{{
                        ID:           "item-a",
                        ProjectID:    projectID,
                        OutlineID:    outlineID,
                        Rank:         "h",
                        Title:        "A",
                        StatusID:     "doing",
                        OwnerActorID: actorID,
                        CreatedBy:    actorID,
                        CreatedAt:    now,
                        UpdatedAt:    now,
                }},
                Deps:     []model.Dependency{},
                Comments: []model.Comment{},
                Worklog:  []model.WorklogEntry{},
        }

        s := store.Store{Dir: dir}
        if err := s.Save(db); err != nil {
                t.Fatalf("save db: %v", err)
        }

        m := newAppModel(dir, db)
        m.view = viewOutline
        m.selectedProjectID = projectID
        m.selectedOutlineID = outlineID
        m.selectedOutline = &db.Outlines[0]

        // Open editor.
        mAny, _ := m.updateOutline(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
        m = mAny.(appModel)
        m.outlineStatusDefsList.Select(1) // doing

        // Attempt delete.
        mAny, _ = m.updateOutline(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
        m = mAny.(appModel)

        db2, err := s.Load()
        if err != nil {
                t.Fatalf("load db: %v", err)
        }
        o2, _ := db2.FindOutline(outlineID)
        if got := len(o2.StatusDefs); got != 3 {
                t.Fatalf("expected statuses to remain unchanged, got %d", got)
        }
}
