package cli

import (
        "encoding/json"
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

func TestItemsShow_IncludesChildrenAndDeps(t *testing.T) {
        t.Parallel()

        dir := t.TempDir()

        now := time.Date(2025, 12, 20, 0, 0, 0, 0, time.UTC)
        actorID := "act-testhuman"
        projectID := "proj-test"
        outlineID := "out-test"

        parentID := "item-parent"
        childID := "item-child"
        blockerID := "item-blocker"
        blockedID := "item-blocked"

        db := &store.DB{
                Version:        1,
                CurrentActorID: actorID,
                NextIDs:        map[string]int{},
                Actors: []model.Actor{
                        {ID: actorID, Kind: model.ActorKindHuman, Name: "Test Human"},
                },
                Projects: []model.Project{
                        {ID: projectID, Name: "Test Project", CreatedBy: actorID, CreatedAt: now},
                },
                Outlines: []model.Outline{
                        {ID: outlineID, ProjectID: projectID, Name: nil, StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: actorID, CreatedAt: now},
                },
                Items: []model.Item{
                        {
                                ID:           parentID,
                                ProjectID:    projectID,
                                OutlineID:    outlineID,
                                ParentID:     nil,
                                Rank:         "h",
                                Title:        "Parent",
                                Description:  "",
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
                                ID:           childID,
                                ProjectID:    projectID,
                                OutlineID:    outlineID,
                                ParentID:     &parentID,
                                Rank:         "h00000000g0h01",
                                Title:        "Child",
                                Description:  "",
                                StatusID:     "done",
                                Priority:     false,
                                OnHold:       false,
                                Archived:     false,
                                OwnerActorID: actorID,
                                CreatedBy:    actorID,
                                CreatedAt:    now,
                                UpdatedAt:    now,
                        },
                        {
                                ID:           blockerID,
                                ProjectID:    projectID,
                                OutlineID:    outlineID,
                                ParentID:     nil,
                                Rank:         "h00000000g0h02",
                                Title:        "Blocker",
                                Description:  "",
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
                                ID:           blockedID,
                                ProjectID:    projectID,
                                OutlineID:    outlineID,
                                ParentID:     nil,
                                Rank:         "h00000000g0h03",
                                Title:        "Blocked",
                                Description:  "",
                                StatusID:     "todo",
                                Priority:     false,
                                OnHold:       false,
                                Archived:     false,
                                OwnerActorID: actorID,
                                CreatedBy:    actorID,
                                CreatedAt:    now,
                                UpdatedAt:    now,
                        },
                },
                Deps: []model.Dependency{
                        // blocker -> parent (in blocker edge for parent)
                        {ID: "dep-1", FromItemID: blockerID, ToItemID: parentID, Type: model.DependencyBlocks},
                        // parent -> blocked (out blocked edge for parent)
                        {ID: "dep-2", FromItemID: parentID, ToItemID: blockedID, Type: model.DependencyBlocks},
                },
                Comments: []model.Comment{},
                Worklog:  []model.WorklogEntry{},
        }

        if err := (store.Store{Dir: dir}).Save(db); err != nil {
                t.Fatalf("seed store: %v", err)
        }

        out, errOut, err := runCLI(t, []string{"--dir", dir, "--actor", actorID, "items", "show", parentID})
        if err != nil {
                t.Fatalf("items show error: %v\nstderr:\n%s", err, string(errOut))
        }

        var v map[string]any
        if err := json.Unmarshal(out, &v); err != nil {
                t.Fatalf("unmarshal output: %v\nstdout:\n%s", err, string(out))
        }
        data, ok := v["data"].(map[string]any)
        if !ok {
                t.Fatalf("expected data object; got: %#v", v["data"])
        }

        // data.item
        item, ok := data["item"].(map[string]any)
        if !ok {
                t.Fatalf("expected data.item object; got: %#v", data["item"])
        }
        if got, _ := item["id"].(string); got != parentID {
                t.Fatalf("expected data.item.id=%q; got %q", parentID, got)
        }

        // data.children.items
        children, ok := data["children"].(map[string]any)
        if !ok {
                t.Fatalf("expected data.children object; got: %#v", data["children"])
        }
        items, ok := children["items"].([]any)
        if !ok {
                t.Fatalf("expected data.children.items array; got: %#v", children["items"])
        }
        if len(items) != 1 {
                t.Fatalf("expected 1 child item; got %d", len(items))
        }
        child0, ok := items[0].(map[string]any)
        if !ok {
                t.Fatalf("expected child object; got: %#v", items[0])
        }
        if got, _ := child0["id"].(string); got != childID {
                t.Fatalf("expected child.id=%q; got %q", childID, got)
        }

        // data.deps.blocks in/out
        deps, ok := data["deps"].(map[string]any)
        if !ok {
                t.Fatalf("expected data.deps object; got: %#v", data["deps"])
        }
        blocks, ok := deps["blocks"].(map[string]any)
        if !ok {
                t.Fatalf("expected data.deps.blocks object; got: %#v", deps["blocks"])
        }
        inEdges, ok := blocks["in"].([]any)
        if !ok || len(inEdges) != 1 {
                t.Fatalf("expected exactly 1 deps.blocks.in edge; got: %#v", blocks["in"])
        }
        outEdges, ok := blocks["out"].([]any)
        if !ok || len(outEdges) != 1 {
                t.Fatalf("expected exactly 1 deps.blocks.out edge; got: %#v", blocks["out"])
        }
}
