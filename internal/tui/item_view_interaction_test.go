package tui

import (
        "strings"
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        tea "github.com/charmbracelet/bubbletea"
)

func TestItemView_TabCyclesFocus_EnterOpensModal(t *testing.T) {
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
                        Description:  "Old description",
                        StatusID:     "todo",
                        Priority:     false,
                        OnHold:       false,
                        Archived:     false,
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
        m.view = viewItem
        m.selectedProjectID = "proj-a"
        m.selectedOutlineID = "out-a"
        m.selectedOutline = &db.Outlines[0]
        m.openItemID = "item-a"
        m.itemFocus = itemFocusTitle

        // tab => status
        mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
        m2 := mAny.(appModel)
        if m2.itemFocus != itemFocusStatus {
                t.Fatalf("expected focus=%v, got %v", itemFocusStatus, m2.itemFocus)
        }

        // tab => description
        mAny, _ = m2.Update(tea.KeyMsg{Type: tea.KeyTab})
        m3 := mAny.(appModel)
        if m3.itemFocus != itemFocusDescription {
                t.Fatalf("expected focus=%v, got %v", itemFocusDescription, m3.itemFocus)
        }

        // enter => edit description modal, seeded with existing description.
        mAny, _ = m3.Update(tea.KeyMsg{Type: tea.KeyEnter})
        m4 := mAny.(appModel)
        if m4.modal != modalEditDescription {
                t.Fatalf("expected modalEditDescription, got %v", m4.modal)
        }
        if got := strings.TrimSpace(m4.textarea.Value()); got != "Old description" {
                t.Fatalf("expected textarea seeded with old description; got %q", got)
        }
}

func TestItemView_ShortcutsOpenModals(t *testing.T) {
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
                        Description:  "Old description",
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
        m.view = viewItem
        m.selectedProjectID = "proj-a"
        m.selectedOutlineID = "out-a"
        m.selectedOutline = &db.Outlines[0]
        m.openItemID = "item-a"

        // Shift+D => edit description modal.
        mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
        m2 := mAny.(appModel)
        if m2.modal != modalEditDescription {
                t.Fatalf("expected modalEditDescription, got %v", m2.modal)
        }

        // ESC closes modal via modal handler; keep it simple here by manually clearing.
        m2.modal = modalNone
        m2.modalForID = ""

        // C => add comment modal (and focus comments so the side panel stays open after save).
        mAny, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}})
        m3 := mAny.(appModel)
        if m3.modal != modalAddComment {
                t.Fatalf("expected modalAddComment, got %v", m3.modal)
        }
        if m3.itemFocus != itemFocusComments {
                t.Fatalf("expected focus=%v, got %v", itemFocusComments, m3.itemFocus)
        }
}
