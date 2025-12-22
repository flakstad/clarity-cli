package tui

import (
        "path/filepath"
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

func TestWorkspaceSwitch_SeedsIdentityForNewWorkspace(t *testing.T) {
        cfgDir := t.TempDir()
        t.Setenv("CLARITY_CONFIG_DIR", cfgDir)

        now := time.Date(2025, 12, 22, 0, 0, 0, 0, time.UTC)

        // Seed a "from" workspace with a current actor.
        fromDir := filepath.Join(cfgDir, "workspaces", "from")
        fromStore := store.Store{Dir: fromDir}
        fromDB := &store.DB{
                Version:          1,
                CurrentActorID:   "act-a",
                CurrentProjectID: "",
                NextIDs:          map[string]int{},
                Actors:           []model.Actor{{ID: "act-a", Kind: model.ActorKindHuman, Name: "Andreas"}},
                Projects:         []model.Project{},
                Outlines:         []model.Outline{},
                Items:            []model.Item{},
                Deps:             []model.Dependency{},
                Comments:         []model.Comment{},
                Worklog:          []model.WorklogEntry{},
        }
        // Give a non-zero timestamp to avoid any accidental zero-time assumptions.
        fromDB.Projects = append(fromDB.Projects, model.Project{ID: "proj-x", Name: "P", CreatedBy: "act-a", CreatedAt: now})
        if err := fromStore.Save(fromDB); err != nil {
                t.Fatalf("seed from workspace: %v", err)
        }

        m := newAppModelWithWorkspace(fromDir, fromDB, "from")

        // Switch to a brand new workspace; it should inherit the current identity.
        nm, err := m.switchWorkspaceTo("to")
        if err != nil {
                t.Fatalf("switchWorkspaceTo: %v", err)
        }
        if nm.db == nil {
                t.Fatalf("expected db")
        }
        if nm.db.CurrentActorID != "act-a" {
                t.Fatalf("expected CurrentActorID %q; got %q", "act-a", nm.db.CurrentActorID)
        }
        if _, ok := nm.db.FindActor("act-a"); !ok {
                t.Fatalf("expected actor act-a to exist in new workspace")
        }
}
