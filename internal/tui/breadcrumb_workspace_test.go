package tui

import (
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

func TestBreadcrumb_ShowsWorkspaceName(t *testing.T) {
        t.Parallel()

        now := time.Date(2025, 12, 22, 0, 0, 0, 0, time.UTC)
        db := &store.DB{
                Version:        1,
                NextIDs:        map[string]int{},
                Actors:         []model.Actor{},
                Projects:       []model.Project{},
                Outlines:       []model.Outline{},
                Items:          []model.Item{},
                Deps:           []model.Dependency{},
                Comments:       []model.Comment{},
                Worklog:        []model.WorklogEntry{},
                CurrentActorID: "act-test",
        }
        // Keep timestamps non-zero if any code assumes them.
        db.Projects = append(db.Projects, model.Project{ID: "proj-test", Name: "P", CreatedBy: "act-test", CreatedAt: now})
        db.CurrentProjectID = "proj-test"

        m := newAppModelWithWorkspace(t.TempDir(), db, "my-ws")
        m.view = viewProjects
        if got := m.breadcrumbText(); got != "my-ws" {
                t.Fatalf("expected breadcrumb %q; got %q", "my-ws", got)
        }

        m.view = viewAgenda
        if got := m.breadcrumbText(); got != "my-ws > agenda" {
                t.Fatalf("expected breadcrumb %q; got %q", "my-ws > agenda", got)
        }
}
