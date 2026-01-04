package tui

import (
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        tea "github.com/charmbracelet/bubbletea"
)

func TestProjectsAndOutlinesHaveAddRow(t *testing.T) {
        t.Parallel()

        now := time.Date(2025, 12, 22, 0, 0, 0, 0, time.UTC)
        actorID := "act-a"
        projectID := "proj-a"
        outlineID := "out-a"

        db := &store.DB{
                Version:          1,
                CurrentActorID:   actorID,
                CurrentProjectID: projectID,
                NextIDs:          map[string]int{},
                Actors:           []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "A"}},
                Projects:         []model.Project{{ID: projectID, Name: "P", CreatedBy: actorID, CreatedAt: now}},
                Outlines:         []model.Outline{{ID: outlineID, ProjectID: projectID, StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: actorID, CreatedAt: now}},
                Items:            []model.Item{},
                Deps:             []model.Dependency{},
                Comments:         []model.Comment{},
                Worklog:          []model.WorklogEntry{},
        }

        m := newAppModelWithWorkspace(t.TempDir(), db, "ws")
        m.refreshProjects()
        if len(m.projectsList.Items()) == 0 {
                t.Fatalf("expected projects list items")
        }
        for _, it := range m.projectsList.Items() {
                if _, ok := it.(addProjectRow); ok {
                        t.Fatalf("did not expect addProjectRow in projects list")
                }
        }

        m.selectedProjectID = projectID
        m.view = viewOutlines
        m.refreshOutlines(projectID)
        if len(m.outlinesList.Items()) == 0 {
                t.Fatalf("expected outlines list items")
        }
        for _, it := range m.outlinesList.Items() {
                if _, ok := it.(addOutlineRow); ok {
                        t.Fatalf("did not expect addOutlineRow in outlines list")
                }
        }
}

func TestAddRowEnterOpensSameModalAsN(t *testing.T) {
        t.Parallel()

        now := time.Date(2025, 12, 22, 0, 0, 0, 0, time.UTC)
        actorID := "act-a"
        projectID := "proj-a"

        db := &store.DB{
                Version:          1,
                CurrentActorID:   actorID,
                CurrentProjectID: projectID,
                NextIDs:          map[string]int{},
                Actors:           []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "A"}},
                Projects:         []model.Project{{ID: projectID, Name: "P", CreatedBy: actorID, CreatedAt: now}},
                Outlines:         []model.Outline{},
                Items:            []model.Item{},
                Deps:             []model.Dependency{},
                Comments:         []model.Comment{},
                Worklog:          []model.WorklogEntry{},
        }

        // Projects view: "n" opens modalNewProject.
        mp := newAppModelWithWorkspace(t.TempDir(), db, "ws")
        mp.view = viewProjects
        mp.refreshProjects()
        nm, _ := mp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
        mp2 := nm.(appModel)
        if mp2.modal != modalNewProject {
                t.Fatalf("expected modalNewProject; got %v", mp2.modal)
        }

        // Outlines view: "n" opens modalNewOutline.
        mo := newAppModelWithWorkspace(t.TempDir(), db, "ws")
        mo.view = viewOutlines
        mo.selectedProjectID = projectID
        mo.refreshOutlines(projectID)
        nm2, _ := mo.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
        mo2 := nm2.(appModel)
        if mo2.modal != modalNewOutline {
                t.Fatalf("expected modalNewOutline; got %v", mo2.modal)
        }
}
