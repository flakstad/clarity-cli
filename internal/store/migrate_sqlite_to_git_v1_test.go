package store

import (
        "bufio"
        "context"
        "encoding/json"
        "os"
        "path/filepath"
        "strings"
        "testing"
)

func TestMigrateSQLiteToGitBackedV1_WritesMetaDeviceAndEvents(t *testing.T) {
        ctx := context.Background()

        from := t.TempDir()
        to := t.TempDir()
        // Ensure `to` is empty (TempDir exists). We'll create a fresh empty subdir.
        to = filepath.Join(to, "out")

        src := Store{Dir: from}
        // Seed a legacy pre-sqlite events.jsonl entry (workspace-root).
        legacyPath := filepath.Join(from, "events.jsonl")
        legacyLine := `{"id":"evt-1","ts":"2025-12-19T20:45:17.610874Z","actorId":"act-a","type":"identity.create","entityId":"act-a","payload":{"id":"act-a","kind":"human","name":"Legacy"}}` + "\n"
        if err := os.WriteFile(legacyPath, []byte(legacyLine), 0o644); err != nil {
                t.Fatalf("write legacy events.jsonl: %v", err)
        }

        // Create at least one event in legacy SQLite event log mode.
        var evs []EventV1
        withEnv(t, envEventLogBackend, string(EventLogBackendSQLite), func() {
                if err := src.AppendEvent("act-a", "identity.create", "act-a", map[string]any{"id": "act-a"}); err != nil {
                        t.Fatalf("AppendEvent: %v", err)
                }
                got, err := src.ReadEventsV1(ctx, 0)
                if err != nil {
                        t.Fatalf("ReadEventsV1: %v", err)
                }
                evs = got
        })
        if len(evs) == 0 {
                t.Fatalf("expected events")
        }

        res, err := MigrateSQLiteToGitBackedV1(ctx, from, to)
        if err != nil {
                t.Fatalf("MigrateSQLiteToGitBackedV1: %v", err)
        }
        if res.EventsWritten != len(evs)+1 {
                t.Fatalf("EventsWritten=%d want %d", res.EventsWritten, len(evs)+1)
        }
        if strings.TrimSpace(res.WorkspaceID) == "" || strings.TrimSpace(res.ReplicaID) == "" {
                t.Fatalf("expected workspaceId/replicaId")
        }

        // Meta/workspace.json
        metaRaw, err := os.ReadFile(filepath.Join(to, "meta", "workspace.json"))
        if err != nil {
                t.Fatalf("read workspace.json: %v", err)
        }
        var meta WorkspaceMetaFile
        if err := json.Unmarshal(metaRaw, &meta); err != nil {
                t.Fatalf("unmarshal workspace.json: %v", err)
        }
        if meta.WorkspaceID != res.WorkspaceID {
                t.Fatalf("workspaceId=%q want %q", meta.WorkspaceID, res.WorkspaceID)
        }

        // .clarity/device.json
        devRaw, err := os.ReadFile(filepath.Join(to, ".clarity", "device.json"))
        if err != nil {
                t.Fatalf("read device.json: %v", err)
        }
        var device DeviceFile
        if err := json.Unmarshal(devRaw, &device); err != nil {
                t.Fatalf("unmarshal device.json: %v", err)
        }
        if device.ReplicaID != res.ReplicaID {
                t.Fatalf("device replicaId=%q want %q", device.ReplicaID, res.ReplicaID)
        }

        // Shard file contains the migrated events.
        shard := filepath.Join(to, "events", "events."+res.ReplicaID+".jsonl")
        f, err := os.Open(shard)
        if err != nil {
                t.Fatalf("open shard: %v", err)
        }
        defer f.Close()

        sc := bufio.NewScanner(f)
        var got []EventV1
        for sc.Scan() {
                var ev EventV1
                if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
                        t.Fatalf("unmarshal event: %v", err)
                }
                got = append(got, ev)
        }
        if err := sc.Err(); err != nil {
                t.Fatalf("scan shard: %v", err)
        }
        if len(got) != len(evs)+1 {
                t.Fatalf("shard events=%d want %d", len(got), len(evs)+1)
        }
        // The legacy event should be present.
        foundLegacy := false
        for _, ev := range got {
                if strings.TrimSpace(ev.EventID) == "evt-1" {
                        foundLegacy = true
                        if strings.TrimSpace(ev.WorkspaceID) != strings.TrimSpace(res.WorkspaceID) {
                                t.Fatalf("legacy workspaceId=%q want %q", ev.WorkspaceID, res.WorkspaceID)
                        }
                        if strings.TrimSpace(ev.ReplicaID) != strings.TrimSpace(res.ReplicaID) {
                                t.Fatalf("legacy replicaId=%q want %q", ev.ReplicaID, res.ReplicaID)
                        }
                        if strings.TrimSpace(ev.Type) != "identity.create" {
                                t.Fatalf("legacy type=%q", ev.Type)
                        }
                }
        }
        if !foundLegacy {
                t.Fatalf("expected legacy event to be migrated")
        }

        // .gitignore should include .clarity/
        gi, err := os.ReadFile(filepath.Join(to, ".gitignore"))
        if err != nil {
                t.Fatalf("read .gitignore: %v", err)
        }
        if !strings.Contains(string(gi), ".clarity/") {
                t.Fatalf("expected .gitignore to contain .clarity/")
        }
}
