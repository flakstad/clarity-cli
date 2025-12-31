package store

import (
        "os"
        "path/filepath"
        "testing"
)

func TestDoctorEventsV1_DetectsDuplicateEvents(t *testing.T) {
        dir := t.TempDir()
        if err := os.MkdirAll(filepath.Join(dir, "events"), 0o755); err != nil {
                t.Fatalf("mkdir events: %v", err)
        }
        path := filepath.Join(dir, "events", "events.rep-a.jsonl")
        line := `{"eventId":"evt-1","workspaceId":"ws-1","replicaId":"rep-a","entityKind":"item","entityId":"item-1","entitySeq":1,"type":"item.create","issuedAt":"2025-12-31T00:00:00Z","actorId":"act-1","payload":{}}` + "\n"
        if err := os.WriteFile(path, []byte(line+line), 0o644); err != nil {
                t.Fatalf("write file: %v", err)
        }

        r := DoctorEventsV1(dir)
        if !r.HasErrors() {
                t.Fatalf("expected errors; got %#v", r)
        }
        found := false
        for _, it := range r.Issues {
                if it.Code == "duplicate_event" {
                        found = true
                        break
                }
        }
        if !found {
                t.Fatalf("expected duplicate_event issue; got %#v", r.Issues)
        }
}

func TestDoctorEventsV1_ReportsParseErrorsWithFileLine(t *testing.T) {
        dir := t.TempDir()
        if err := os.MkdirAll(filepath.Join(dir, "events"), 0o755); err != nil {
                t.Fatalf("mkdir events: %v", err)
        }
        path := filepath.Join(dir, "events", "events.rep-a.jsonl")
        if err := os.WriteFile(path, []byte("{not json}\n"), 0o644); err != nil {
                t.Fatalf("write file: %v", err)
        }

        r := DoctorEventsV1(dir)
        if len(r.Issues) == 0 {
                t.Fatalf("expected issues; got %#v", r.Issues)
        }
        found := false
        for _, it := range r.Issues {
                if it.Code != "malformed_json" {
                        continue
                }
                if it.Path != path || it.Line != 1 {
                        t.Fatalf("expected path+line on malformed_json; got %#v", it)
                }
                found = true
                break
        }
        if !found {
                t.Fatalf("expected malformed_json issue; got %#v", r.Issues)
        }
}
