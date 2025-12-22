package cli

import (
        "encoding/json"
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

func TestItemsSetStatus_AppendsTransitionEvent(t *testing.T) {
        t.Parallel()

        dir := t.TempDir()
        now := time.Date(2025, 12, 20, 0, 0, 0, 0, time.UTC)
        actorID := "act-testhuman"
        projectID := "proj-test"
        outlineID := "out-test"
        itemID := "item-test"

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
                                ID:           itemID,
                                ProjectID:    projectID,
                                OutlineID:    outlineID,
                                Rank:         "h",
                                Title:        "Test Item",
                                StatusID:     "todo",
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

        _, errOut, err := runCLI(t, []string{"--dir", dir, "--actor", actorID, "items", "set-status", itemID, "--status", "done"})
        if err != nil {
                t.Fatalf("items set-status error: %v\nstderr:\n%s", err, string(errOut))
        }

        evs, err := store.ReadEventsForEntity(dir, itemID, 0)
        if err != nil {
                t.Fatalf("read events: %v", err)
        }
        if len(evs) == 0 {
                t.Fatalf("expected at least 1 event for %s", itemID)
        }
        last := evs[len(evs)-1]
        if last.Type != "item.set_status" {
                t.Fatalf("expected last event type item.set_status; got %q", last.Type)
        }
        payload, ok := last.Payload.(map[string]any)
        if !ok {
                t.Fatalf("expected payload object; got %#v", last.Payload)
        }
        if got, _ := payload["from"].(string); got != "todo" {
                t.Fatalf("expected from=todo; got %q (payload=%#v)", got, payload)
        }
        if got, _ := payload["to"].(string); got != "done" {
                t.Fatalf("expected to=done; got %q (payload=%#v)", got, payload)
        }
        // Backwards-compat key.
        if got, _ := payload["status"].(string); got != "done" {
                t.Fatalf("expected status=done; got %q (payload=%#v)", got, payload)
        }
}

func TestItemsEvents_FiltersByItem(t *testing.T) {
        t.Parallel()

        dir := t.TempDir()
        now := time.Date(2025, 12, 20, 0, 0, 0, 0, time.UTC)
        actorID := "act-testhuman"

        db := &store.DB{
                Version:        1,
                CurrentActorID: actorID,
                NextIDs:        map[string]int{},
                Actors: []model.Actor{
                        {ID: actorID, Kind: model.ActorKindHuman, Name: "Test Human"},
                },
                Projects: []model.Project{
                        {ID: "proj-test", Name: "Test Project", CreatedBy: actorID, CreatedAt: now},
                },
                Outlines: []model.Outline{
                        {ID: "out-test", ProjectID: "proj-test", Name: nil, StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: actorID, CreatedAt: now},
                },
                Items: []model.Item{
                        {ID: "item-a", ProjectID: "proj-test", OutlineID: "out-test", Rank: "h", Title: "A", StatusID: "todo", OwnerActorID: actorID, CreatedBy: actorID, CreatedAt: now, UpdatedAt: now},
                        {ID: "item-b", ProjectID: "proj-test", OutlineID: "out-test", Rank: "h0", Title: "B", StatusID: "todo", OwnerActorID: actorID, CreatedBy: actorID, CreatedAt: now, UpdatedAt: now},
                },
        }
        st := store.Store{Dir: dir}
        if err := st.Save(db); err != nil {
                t.Fatalf("seed store: %v", err)
        }

        _ = st.AppendEvent(actorID, "item.set_title", "item-a", map[string]any{"title": "A2"})
        _ = st.AppendEvent(actorID, "item.set_status", "item-a", map[string]any{"from": "todo", "to": "doing", "status": "doing"})
        _ = st.AppendEvent(actorID, "item.set_title", "item-b", map[string]any{"title": "B2"})

        out, errOut, err := runCLI(t, []string{"--dir", dir, "--actor", actorID, "items", "events", "item-a", "--limit", "0"})
        if err != nil {
                t.Fatalf("items events error: %v\nstderr:\n%s", err, string(errOut))
        }

        var v map[string]any
        if err := json.Unmarshal(out, &v); err != nil {
                t.Fatalf("unmarshal output: %v\nstdout:\n%s", err, string(out))
        }
        data, ok := v["data"].([]any)
        if !ok {
                t.Fatalf("expected data array; got %#v", v["data"])
        }
        if len(data) != 2 {
                t.Fatalf("expected 2 events for item-a; got %d", len(data))
        }
        for _, raw := range data {
                ev, ok := raw.(map[string]any)
                if !ok {
                        t.Fatalf("expected event object; got %#v", raw)
                }
                if got, _ := ev["entityId"].(string); got != "item-a" {
                        t.Fatalf("expected entityId=item-a; got %q (ev=%#v)", got, ev)
                }
        }
}
