package store

import (
        "context"
        "os"
        "path/filepath"
        "testing"
)

func TestBackupEventsJSONLRoundTripAndRestore(t *testing.T) {
        ctx := context.Background()

        dir1 := t.TempDir()
        s1 := Store{Dir: dir1}
        if err := s1.Save(&DB{Version: 1, NextIDs: map[string]int{}}); err != nil {
                t.Fatalf("save state: %v", err)
        }

        if err := s1.AppendEvent("actor-1", "item.create", "item-1", map[string]any{"id": "item-1"}); err != nil {
                t.Fatalf("append event: %v", err)
        }

        evs1, err := s1.ReadEventsV1(ctx, 0)
        if err != nil {
                t.Fatalf("read events v1: %v", err)
        }
        if len(evs1) != 1 {
                t.Fatalf("expected 1 event, got %d", len(evs1))
        }
        if evs1[0].WorkspaceID == "" {
                t.Fatalf("expected workspaceId to be set")
        }

        eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
        if err := WriteEventsJSONL(eventsPath, evs1); err != nil {
                t.Fatalf("write events jsonl: %v", err)
        }
        if _, err := os.Stat(eventsPath); err != nil {
                t.Fatalf("expected events file to exist: %v", err)
        }

        evs2, err := ReadEventsJSONL(eventsPath)
        if err != nil {
                t.Fatalf("read events jsonl: %v", err)
        }
        if len(evs2) != 1 {
                t.Fatalf("expected 1 event from jsonl, got %d", len(evs2))
        }
        if evs2[0].EventID != evs1[0].EventID {
                t.Fatalf("eventId mismatch after jsonl roundtrip")
        }
        if evs2[0].WorkspaceID != evs1[0].WorkspaceID {
                t.Fatalf("workspaceId mismatch after jsonl roundtrip")
        }

        dir2 := t.TempDir()
        s2 := Store{Dir: dir2}
        if err := s2.Save(&DB{Version: 1, NextIDs: map[string]int{}}); err != nil {
                t.Fatalf("save state (dest): %v", err)
        }
        if err := s2.ReplaceEventsV1(ctx, evs2[0].WorkspaceID, evs2); err != nil {
                t.Fatalf("replace events: %v", err)
        }

        evs3, err := s2.ReadEventsV1(ctx, 0)
        if err != nil {
                t.Fatalf("read events v1 (dest): %v", err)
        }
        if len(evs3) != 1 {
                t.Fatalf("expected 1 restored event, got %d", len(evs3))
        }
        if evs3[0].EventID != evs1[0].EventID {
                t.Fatalf("restored eventId mismatch")
        }
}
