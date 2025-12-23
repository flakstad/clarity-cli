package tui

import (
        "strings"
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        tea "github.com/charmbracelet/bubbletea"
)

func TestArchivedView_OpenArchivedItem_IsReadOnly(t *testing.T) {
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
                        Archived:  true,
                        CreatedBy: actorID,
                        CreatedAt: now,
                }},
                Outlines: []model.Outline{{
                        ID:         "out-a",
                        ProjectID:  "proj-a",
                        Archived:   true,
                        StatusDefs: store.DefaultOutlineStatusDefs(),
                        CreatedBy:  actorID,
                        CreatedAt:  now,
                }},
                Items: []model.Item{{
                        ID:           "item-a",
                        ProjectID:    "proj-a",
                        OutlineID:    "out-a",
                        Rank:         "h",
                        Title:        "Archived item",
                        StatusID:     "todo",
                        Archived:     true,
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

        // Open Go to and pick Archived.
        mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
        m2 := mAny.(appModel)
        if m2.modal != modalActionPanel || m2.curActionPanelKind() != actionPanelNav {
                t.Fatalf("expected nav action panel open")
        }
        mAny, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
        m3 := mAny.(appModel)
        if m3.modal != modalNone {
                t.Fatalf("expected action panel to close after selecting Archived, got %v", m3.modal)
        }
        if m3.view != viewArchived {
                t.Fatalf("expected viewArchived, got %v", m3.view)
        }

        // Enter opens the selected archived item (refreshArchived selects first item row).
        mAny, _ = m3.Update(tea.KeyMsg{Type: tea.KeyEnter})
        m4 := mAny.(appModel)
        if m4.view != viewItem {
                t.Fatalf("expected viewItem after enter, got %v", m4.view)
        }
        if got := strings.TrimSpace(m4.openItemID); got != "item-a" {
                t.Fatalf("expected openItemID=item-a, got %q", got)
        }
        if !m4.itemArchivedReadOnly {
                t.Fatalf("expected itemArchivedReadOnly=true")
        }
        if !m4.hasReturnView || m4.returnView != viewArchived {
                t.Fatalf("expected returnView=viewArchived")
        }

        // Mutations should be blocked.
        mAny, _ = m4.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
        m5 := mAny.(appModel)
        if m5.modal != modalNone {
                t.Fatalf("expected no modal in read-only item view, got %v", m5.modal)
        }
        if !strings.Contains(strings.ToLower(m5.minibufferText), "read-only") {
                t.Fatalf("expected read-only minibuffer message, got %q", m5.minibufferText)
        }

        acts := m5.actionPanelActions()
        if _, ok := acts["e"]; ok {
                t.Fatalf("expected action panel not to include edit action in read-only item view")
        }
        if _, ok := acts["y"]; !ok {
                t.Fatalf("expected action panel to include copy action in read-only item view")
        }
}
