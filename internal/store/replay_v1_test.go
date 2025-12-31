package store

import (
        "os"
        "path/filepath"
        "testing"
)

func TestReplayEventsV1_BuildsMaterializedState(t *testing.T) {
        dir := t.TempDir()
        if err := os.MkdirAll(filepath.Join(dir, "events"), 0o755); err != nil {
                t.Fatalf("mkdir events: %v", err)
        }
        eventsPath := filepath.Join(dir, "events", "events.rep-a.jsonl")

        // Minimal happy-path stream: actor/project/outline/item + title update.
        lines := "" +
                `{"eventId":"evt-1","workspaceId":"ws-1","replicaId":"rep-a","entityKind":"actor","entityId":"act-1","entitySeq":0,"type":"identity.create","issuedAt":"2025-12-31T00:00:00Z","actorId":"act-1","payload":{"name":"A","kind":"human"}}` + "\n" +
                `{"eventId":"evt-2","workspaceId":"ws-1","replicaId":"rep-a","entityKind":"project","entityId":"proj-1","entitySeq":0,"type":"project.create","issuedAt":"2025-12-31T00:00:01Z","actorId":"act-1","payload":{"id":"proj-1","name":"P","createdBy":"act-1","createdAt":"2025-12-31T00:00:01Z","archived":false}}` + "\n" +
                `{"eventId":"evt-2b","workspaceId":"ws-1","replicaId":"rep-a","entityKind":"project","entityId":"proj-1","entitySeq":0,"type":"project.rename","issuedAt":"2025-12-31T00:00:01Z","actorId":"act-1","payload":{"name":"P2"}}` + "\n" +
                `{"eventId":"evt-3","workspaceId":"ws-1","replicaId":"rep-a","entityKind":"outline","entityId":"out-1","entitySeq":0,"type":"outline.create","issuedAt":"2025-12-31T00:00:02Z","actorId":"act-1","payload":{"id":"out-1","projectId":"proj-1","statusDefs":[{"id":"todo","label":"Todo","isEndState":false},{"id":"done","label":"Done","isEndState":true}],"createdBy":"act-1","createdAt":"2025-12-31T00:00:02Z","archived":false}}` + "\n" +
                `{"eventId":"evt-3b","workspaceId":"ws-1","replicaId":"rep-a","entityKind":"outline","entityId":"out-1","entitySeq":0,"type":"outline.rename","issuedAt":"2025-12-31T00:00:02Z","actorId":"act-1","payload":{"name":"My Outline"}}` + "\n" +
                `{"eventId":"evt-3c","workspaceId":"ws-1","replicaId":"rep-a","entityKind":"outline","entityId":"out-1","entitySeq":0,"type":"outline.set_description","issuedAt":"2025-12-31T00:00:02Z","actorId":"act-1","payload":{"description":"Hello"}}` + "\n" +
                `{"eventId":"evt-3d","workspaceId":"ws-1","replicaId":"rep-a","entityKind":"outline","entityId":"out-1","entitySeq":0,"type":"outline.status.update","issuedAt":"2025-12-31T00:00:02Z","actorId":"act-1","payload":{"id":"todo","requiresNote":true}}` + "\n" +
                `{"eventId":"evt-4","workspaceId":"ws-1","replicaId":"rep-a","entityKind":"item","entityId":"item-1","entitySeq":0,"type":"item.create","issuedAt":"2025-12-31T00:00:03Z","actorId":"act-1","payload":{"id":"item-1","projectId":"proj-1","outlineId":"out-1","rank":"h","title":"T","status":"todo","priority":false,"onHold":false,"archived":false,"ownerActorId":"act-1","createdBy":"act-1","createdAt":"2025-12-31T00:00:03Z","updatedAt":"2025-12-31T00:00:03Z"}}` + "\n" +
                `{"eventId":"evt-5","workspaceId":"ws-1","replicaId":"rep-a","entityKind":"item","entityId":"item-1","entitySeq":0,"type":"item.set_title","issuedAt":"2025-12-31T00:00:04Z","actorId":"act-1","payload":{"title":"New"}}` + "\n"

        if err := os.WriteFile(eventsPath, []byte(lines), 0o644); err != nil {
                t.Fatalf("write events: %v", err)
        }

        res, err := ReplayEventsV1(dir)
        if err != nil {
                t.Fatalf("replay: %v", err)
        }
        if res.DB == nil {
                t.Fatalf("nil db")
        }
        if len(res.DB.Actors) != 1 || res.DB.Actors[0].ID != "act-1" {
                t.Fatalf("unexpected actors: %#v", res.DB.Actors)
        }
        if len(res.DB.Projects) != 1 || res.DB.Projects[0].ID != "proj-1" {
                t.Fatalf("unexpected projects: %#v", res.DB.Projects)
        }
        if res.DB.Projects[0].Name != "P2" {
                t.Fatalf("unexpected project name: %#v", res.DB.Projects[0])
        }
        if len(res.DB.Outlines) != 1 || res.DB.Outlines[0].ID != "out-1" {
                t.Fatalf("unexpected outlines: %#v", res.DB.Outlines)
        }
        if res.DB.Outlines[0].Name == nil || *res.DB.Outlines[0].Name != "My Outline" {
                t.Fatalf("unexpected outline name: %#v", res.DB.Outlines[0])
        }
        if res.DB.Outlines[0].Description != "Hello" {
                t.Fatalf("unexpected outline description: %#v", res.DB.Outlines[0])
        }
        if len(res.DB.Outlines[0].StatusDefs) < 1 || res.DB.Outlines[0].StatusDefs[0].RequiresNote != true {
                t.Fatalf("unexpected outline status defs: %#v", res.DB.Outlines[0].StatusDefs)
        }
        if len(res.DB.Items) != 1 || res.DB.Items[0].Title != "New" {
                t.Fatalf("unexpected items: %#v", res.DB.Items)
        }
}

func TestReplayEventsV1_ReordersStatusDefsByLabel(t *testing.T) {
        dir := t.TempDir()
        if err := os.MkdirAll(filepath.Join(dir, "events"), 0o755); err != nil {
                t.Fatalf("mkdir events: %v", err)
        }
        eventsPath := filepath.Join(dir, "events", "events.rep-a.jsonl")

        lines := "" +
                `{"eventId":"evt-1","workspaceId":"ws-1","replicaId":"rep-a","entityKind":"outline","entityId":"out-1","entitySeq":0,"type":"outline.create","issuedAt":"2025-12-31T00:00:00Z","actorId":"act-1","payload":{"id":"out-1","projectId":"proj-1","statusDefs":[{"id":"todo","label":"Todo","isEndState":false},{"id":"doing","label":"Doing","isEndState":false},{"id":"done","label":"Done","isEndState":true}],"createdBy":"act-1","createdAt":"2025-12-31T00:00:00Z","archived":false}}` + "\n" +
                `{"eventId":"evt-2","workspaceId":"ws-1","replicaId":"rep-a","entityKind":"outline","entityId":"out-1","entitySeq":0,"type":"outline.status.reorder","issuedAt":"2025-12-31T00:00:01Z","actorId":"act-1","payload":{"labels":["Done","Todo","Doing"]}}` + "\n"

        if err := os.WriteFile(eventsPath, []byte(lines), 0o644); err != nil {
                t.Fatalf("write events: %v", err)
        }

        res, err := ReplayEventsV1(dir)
        if err != nil {
                t.Fatalf("replay: %v", err)
        }
        if len(res.DB.Outlines) != 1 {
                t.Fatalf("expected 1 outline, got %d", len(res.DB.Outlines))
        }
        o := res.DB.Outlines[0]
        if len(o.StatusDefs) != 3 {
                t.Fatalf("expected 3 statuses, got %d", len(o.StatusDefs))
        }
        if o.StatusDefs[0].Label != "Done" || o.StatusDefs[1].Label != "Todo" || o.StatusDefs[2].Label != "Doing" {
                t.Fatalf("unexpected status order: %#v", o.StatusDefs)
        }
}

func TestReplayEventsV1_ItemMoveBeforeAfterWithoutRank(t *testing.T) {
        dir := t.TempDir()
        if err := os.MkdirAll(filepath.Join(dir, "events"), 0o755); err != nil {
                t.Fatalf("mkdir events: %v", err)
        }
        eventsPath := filepath.Join(dir, "events", "events.rep-a.jsonl")

        lines := "" +
                `{"eventId":"evt-1","workspaceId":"ws-1","replicaId":"rep-a","entityKind":"item","entityId":"a","entitySeq":0,"type":"item.create","issuedAt":"2025-12-31T00:00:00Z","actorId":"act-1","payload":{"id":"a","projectId":"proj-1","outlineId":"out-1","rank":"a","title":"A","status":"todo","priority":false,"onHold":false,"archived":false,"ownerActorId":"act-1","createdBy":"act-1","createdAt":"2025-12-31T00:00:00Z","updatedAt":"2025-12-31T00:00:00Z"}}` + "\n" +
                `{"eventId":"evt-2","workspaceId":"ws-1","replicaId":"rep-a","entityKind":"item","entityId":"b","entitySeq":0,"type":"item.create","issuedAt":"2025-12-31T00:00:01Z","actorId":"act-1","payload":{"id":"b","projectId":"proj-1","outlineId":"out-1","rank":"b","title":"B","status":"todo","priority":false,"onHold":false,"archived":false,"ownerActorId":"act-1","createdBy":"act-1","createdAt":"2025-12-31T00:00:01Z","updatedAt":"2025-12-31T00:00:01Z"}}` + "\n" +
                `{"eventId":"evt-3","workspaceId":"ws-1","replicaId":"rep-a","entityKind":"item","entityId":"c","entitySeq":0,"type":"item.create","issuedAt":"2025-12-31T00:00:02Z","actorId":"act-1","payload":{"id":"c","projectId":"proj-1","outlineId":"out-1","rank":"c","title":"C","status":"todo","priority":false,"onHold":false,"archived":false,"ownerActorId":"act-1","createdBy":"act-1","createdAt":"2025-12-31T00:00:02Z","updatedAt":"2025-12-31T00:00:02Z"}}` + "\n" +
                `{"eventId":"evt-4","workspaceId":"ws-1","replicaId":"rep-a","entityKind":"item","entityId":"c","entitySeq":0,"type":"item.move","issuedAt":"2025-12-31T00:00:03Z","actorId":"act-1","payload":{"before":"a"}}` + "\n"

        if err := os.WriteFile(eventsPath, []byte(lines), 0o644); err != nil {
                t.Fatalf("write events: %v", err)
        }

        res, err := ReplayEventsV1(dir)
        if err != nil {
                t.Fatalf("replay: %v", err)
        }
        if len(res.DB.Items) != 3 {
                t.Fatalf("expected 3 items, got %d", len(res.DB.Items))
        }

        // Lowest rank should now belong to "c".
        ids := []string{"", "", ""}
        ranks := []string{"", "", ""}
        for i := range res.DB.Items {
                ids[i] = res.DB.Items[i].ID
                ranks[i] = res.DB.Items[i].Rank
        }
        // Find lowest rank item.
        lowID := ""
        lowRank := ""
        for i := range ranks {
                if lowID == "" || ranks[i] < lowRank {
                        lowID = ids[i]
                        lowRank = ranks[i]
                }
        }
        if lowID != "c" {
                t.Fatalf("expected c to be first by rank, got %q (%q)", lowID, lowRank)
        }
}
