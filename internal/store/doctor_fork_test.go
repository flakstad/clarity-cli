package store

import (
        "testing"
        "time"
)

func TestDoctorEventsV1_DetectsForkHeads(t *testing.T) {
        dir := t.TempDir()

        res, err := EnsureGitBackedV1Layout(dir)
        if err != nil {
                t.Fatalf("EnsureGitBackedV1Layout: %v", err)
        }

        // Overwrite shard with a simple fork:
        // e1 -> e2
        //  \\-> e3
        now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
        e1 := EventV1{
                EventID:           "e1",
                WorkspaceID:       res.WorkspaceID,
                ReplicaID:         res.ReplicaID,
                EntityKind:        EntityKindItem,
                EntityID:          "item-1",
                EntitySeq:         1,
                Type:              "item.create",
                Parents:           nil,
                IssuedAt:          now,
                ActorID:           "act-1",
                Payload:           []byte(`{"title":"a"}`),
                LocalStatus:       "local",
                ServerStatus:      "pending",
                PublishedAtUnixMs: nil,
        }
        e2 := e1
        e2.EventID = "e2"
        e2.EntitySeq = 2
        e2.Type = "item.set_description"
        e2.Parents = []string{"e1"}
        e2.IssuedAt = now.Add(time.Second)
        e2.Payload = []byte(`{"description":"b"}`)
        e3 := e2
        e3.EventID = "e3"
        e3.EntitySeq = 2
        e3.Parents = []string{"e1"}
        e3.IssuedAt = now.Add(2 * time.Second)
        e3.Payload = []byte(`{"description":"c"}`)

        if err := WriteEventsJSONL(res.ShardPath, []EventV1{e1, e2, e3}); err != nil {
                t.Fatalf("WriteEventsJSONL: %v", err)
        }

        rep := DoctorEventsV1(dir)
        if !rep.HasErrors() {
                t.Fatalf("expected doctor to report errors")
        }
        found := false
        for _, it := range rep.Issues {
                if it.Code == "fork_detected" && it.EntityKind == "item" && it.EntityID == "item-1" {
                        found = true
                        break
                }
        }
        if !found {
                t.Fatalf("expected fork_detected issue for item/item-1, got %+v", rep.Issues)
        }
}
