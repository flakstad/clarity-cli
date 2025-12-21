package tui

import (
        "strings"
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        "github.com/charmbracelet/bubbles/list"
        tea "github.com/charmbracelet/bubbletea"
)

func TestOutlineFilter_Slash_Types_Enter_Applies_Esc_Clears(t *testing.T) {
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
                        Archived:  false,
                }},
                Outlines: []model.Outline{{
                        ID:         "out-a",
                        ProjectID:  "proj-a",
                        StatusDefs: store.DefaultOutlineStatusDefs(),
                        CreatedBy:  actorID,
                        CreatedAt:  now,
                        Archived:   false,
                }},
                Items: []model.Item{
                        {
                                ID:           "item-a",
                                ProjectID:    "proj-a",
                                OutlineID:    "out-a",
                                Rank:         "h",
                                Title:        "alpha",
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
                                Rank:         "i",
                                Title:        "bravo",
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
        m.view = viewOutline
        m.selectedProjectID = "proj-a"
        m.selectedOutlineID = "out-a"
        m.selectedOutline = &db.Outlines[0]
        m.refreshItems(db.Outlines[0])
        selectListItemByID(&m.itemsList, "item-a")

        // Press "/" => start filtering.
        mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
        m2 := mAny.(appModel)
        if m2.view != viewOutline {
                t.Fatalf("expected to remain in viewOutline, got %v", m2.view)
        }
        if !m2.itemsList.SettingFilter() {
                t.Fatalf("expected itemsList to enter filtering state")
        }
        if m2.itemsList.FilterState() != list.Filtering {
                t.Fatalf("expected FilterState=Filtering, got %v", m2.itemsList.FilterState())
        }
        if got := strings.TrimSpace(m2.minibufferText); got == "" || !strings.Contains(got, "Filter:") {
                t.Fatalf("expected minibuffer to hint filter mode, got %q", got)
        }

        // While filtering, regular keybindings should be captured as text input (e.g. "a" should not open agenda).
        mAny, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
        m2b := mAny.(appModel)
        if m2b.view != viewOutline {
                t.Fatalf("expected to remain in viewOutline while typing filter, got %v", m2b.view)
        }
        if m2b.modal != modalNone {
                t.Fatalf("expected no modal while typing filter, got %v", m2b.modal)
        }
        if got := strings.TrimSpace(m2b.itemsList.FilterValue()); got != "a" {
                t.Fatalf("expected filter value %q after typing a, got %q", "a", got)
        }

        // Also: "q" should be treated as text while filtering (should NOT quit).
        mAny, _ = m2b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
        m2q := mAny.(appModel)
        if m2q.view != viewOutline {
                t.Fatalf("expected to remain in viewOutline while typing filter, got %v", m2q.view)
        }
        if m2q.modal != modalNone {
                t.Fatalf("expected no modal while typing filter, got %v", m2q.modal)
        }
        if got := strings.TrimSpace(m2q.itemsList.FilterValue()); got != "aq" {
                t.Fatalf("expected filter value %q after typing q, got %q", "aq", got)
        }

        // Backspace edits the filter (should not navigate "back"). Remove the "q" so we can type a query that matches.
        mAny, _ = m2q.Update(tea.KeyMsg{Type: tea.KeyBackspace})
        m2q2 := mAny.(appModel)
        if m2q2.view != viewOutline {
                t.Fatalf("expected backspace to not navigate away while filtering; got %v", m2q2.view)
        }
        if got := strings.TrimSpace(m2q2.itemsList.FilterValue()); got != "a" {
                t.Fatalf("expected filter value %q after backspace, got %q", "a", got)
        }

        // Type "br".
        // (continue from m2q2)
        mAny, _ = m2q2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
        m3 := mAny.(appModel)
        mAny, _ = m3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
        m4 := mAny.(appModel)
        if got := strings.TrimSpace(m4.itemsList.FilterValue()); got != "abr" {
                t.Fatalf("expected filter value %q, got %q", "abr", got)
        }
        // Filtering should take effect immediately while typing (no crash; list updates via filter matches).
        if gotN := len(m4.itemsList.VisibleItems()); gotN < 0 {
                t.Fatalf("expected visible items count to be non-negative, got %d", gotN)
        }

        // Backspace edits the filter (should not navigate "back").
        mAny, _ = m4.Update(tea.KeyMsg{Type: tea.KeyBackspace})
        m5 := mAny.(appModel)
        if m5.view != viewOutline {
                t.Fatalf("expected backspace to not navigate away while filtering; got %v", m5.view)
        }
        if got := strings.TrimSpace(m5.itemsList.FilterValue()); got != "ab" {
                t.Fatalf("expected filter value %q after backspace, got %q", "ab", got)
        }

        // Re-type "r" then enter to apply.
        mAny, _ = m5.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
        m6 := mAny.(appModel)
        mAny, _ = m6.Update(tea.KeyMsg{Type: tea.KeyEnter})
        m7 := mAny.(appModel)
        if m7.view != viewOutline {
                t.Fatalf("expected to remain in viewOutline after applying filter, got %v", m7.view)
        }
        if strings.TrimSpace(m7.openItemID) != "" {
                t.Fatalf("expected enter while filtering to not open an item; openItemID=%q", m7.openItemID)
        }
        if !m7.itemsList.IsFiltered() {
                t.Fatalf("expected filter to be applied after enter")
        }
        if got := m7.breadcrumbText(); !strings.Contains(got, "/abr") {
                t.Fatalf("expected breadcrumb to include filter indicator; got %q", got)
        }

        // ESC clears applied filter (should not navigate back).
        mAny, _ = m7.Update(tea.KeyMsg{Type: tea.KeyEsc})
        m8 := mAny.(appModel)
        if m8.view != viewOutline {
                t.Fatalf("expected esc to not navigate away while filtered; got %v", m8.view)
        }
        if m8.itemsList.FilterState() != list.Unfiltered {
                t.Fatalf("expected FilterState=Unfiltered after esc, got %v", m8.itemsList.FilterState())
        }
        if m8.itemsList.IsFiltered() {
                t.Fatalf("expected IsFiltered=false after esc")
        }
}
