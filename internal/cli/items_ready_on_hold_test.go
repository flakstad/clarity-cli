package cli

import (
        "encoding/json"
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

func TestItemsReady_ExcludesOnHoldByDefault_AndCanIncludeWithFlag(t *testing.T) {
        t.Parallel()

        dir := t.TempDir()
        now := time.Date(2025, 12, 26, 0, 0, 0, 0, time.UTC)

        actorID := "act-testhuman"
        projectID := "proj-test"
        outlineID := "out-test"

        db := &store.DB{
                Version:        1,
                CurrentActorID: actorID,
                NextIDs:        map[string]int{},
                Actors:         []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "Test Human"}},
                Projects:       []model.Project{{ID: projectID, Name: "Test Project", CreatedBy: actorID, CreatedAt: now}},
                Outlines:       []model.Outline{{ID: outlineID, ProjectID: projectID, StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: actorID, CreatedAt: now}},
                Items: []model.Item{
                        {
                                ID:           "item-ready",
                                ProjectID:    projectID,
                                OutlineID:    outlineID,
                                Rank:         "h",
                                Title:        "Ready",
                                StatusID:     "todo",
                                Priority:     false,
                                OnHold:       false,
                                Archived:     false,
                                OwnerActorID: actorID,
                                CreatedBy:    actorID,
                                CreatedAt:    now,
                                UpdatedAt:    now,
                        },
                        {
                                ID:           "item-hold",
                                ProjectID:    projectID,
                                OutlineID:    outlineID,
                                Rank:         "i",
                                Title:        "On hold",
                                StatusID:     "todo",
                                Priority:     false,
                                OnHold:       true,
                                Archived:     false,
                                OwnerActorID: actorID,
                                CreatedBy:    actorID,
                                CreatedAt:    now,
                                UpdatedAt:    now,
                        },
                },
        }
        if err := (store.Store{Dir: dir}).Save(db); err != nil {
                t.Fatalf("seed store: %v", err)
        }

        // Default: on-hold excluded.
        stdout, stderr, err := runCLI(t, []string{"--dir", dir, "--actor", actorID, "items", "ready", "--include-assigned"})
        if err != nil {
                t.Fatalf("items ready error: %v\nstderr:\n%s", err, string(stderr))
        }
        var env map[string]any
        if err := json.Unmarshal(stdout, &env); err != nil {
                t.Fatalf("unmarshal ready output: %v\nstdout:\n%s", err, string(stdout))
        }
        data, ok := env["data"].([]any)
        if !ok {
                t.Fatalf("expected data to be a list; got %#v", env["data"])
        }
        ids := map[string]bool{}
        for _, it := range data {
                m, _ := it.(map[string]any)
                id, _ := m["id"].(string)
                if id != "" {
                        ids[id] = true
                }
        }
        if !ids["item-ready"] {
                t.Fatalf("expected item-ready to be in ready list; got ids=%v", ids)
        }
        if ids["item-hold"] {
                t.Fatalf("did not expect on-hold item in ready list by default; got ids=%v", ids)
        }

        // Explicit include.
        stdout2, stderr2, err := runCLI(t, []string{"--dir", dir, "--actor", actorID, "items", "ready", "--include-assigned", "--include-on-hold"})
        if err != nil {
                t.Fatalf("items ready --include-on-hold error: %v\nstderr:\n%s", err, string(stderr2))
        }
        var env2 map[string]any
        if err := json.Unmarshal(stdout2, &env2); err != nil {
                t.Fatalf("unmarshal ready output: %v\nstdout:\n%s", err, string(stdout2))
        }
        data2, ok := env2["data"].([]any)
        if !ok {
                t.Fatalf("expected data to be a list; got %#v", env2["data"])
        }
        ids2 := map[string]bool{}
        for _, it := range data2 {
                m, _ := it.(map[string]any)
                id, _ := m["id"].(string)
                if id != "" {
                        ids2[id] = true
                }
        }
        if !ids2["item-hold"] {
                t.Fatalf("expected item-hold to be included with --include-on-hold; got ids=%v", ids2)
        }
}
