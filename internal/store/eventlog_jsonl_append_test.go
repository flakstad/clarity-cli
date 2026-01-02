package store

import (
        "bufio"
        "encoding/json"
        "os"
        "strings"
        "testing"
        "time"
)

func TestAppendEventJSONL_SetsParentsAndSeq(t *testing.T) {
        dir := t.TempDir()

        res, err := EnsureGitBackedV1Layout(dir)
        if err != nil {
                t.Fatalf("EnsureGitBackedV1Layout: %v", err)
        }

        s := Store{Dir: dir}
        if err := s.AppendEvent("act-1", "item.create", "item-1", map[string]any{"title": "a"}); err != nil {
                t.Fatalf("AppendEvent (1): %v", err)
        }
        if err := s.AppendEvent("act-1", "item.set_description", "item-1", map[string]any{"description": "b"}); err != nil {
                t.Fatalf("AppendEvent (2): %v", err)
        }

        f, err := os.Open(res.ShardPath)
        if err != nil {
                t.Fatalf("open shard: %v", err)
        }
        defer f.Close()

        sc := bufio.NewScanner(f)
        var evs []EventV1
        for sc.Scan() {
                line := strings.TrimSpace(sc.Text())
                if line == "" {
                        continue
                }
                var ev EventV1
                if err := json.Unmarshal([]byte(line), &ev); err != nil {
                        t.Fatalf("parse event: %v", err)
                }
                evs = append(evs, ev)
        }
        if err := sc.Err(); err != nil {
                t.Fatalf("scan shard: %v", err)
        }

        if len(evs) != 2 {
                t.Fatalf("expected 2 events, got %d", len(evs))
        }

        if evs[0].EntitySeq != 1 {
                t.Fatalf("expected first seq=1, got %d", evs[0].EntitySeq)
        }
        if evs[1].EntitySeq != 2 {
                t.Fatalf("expected second seq=2, got %d", evs[1].EntitySeq)
        }

        if len(evs[0].Parents) != 0 {
                t.Fatalf("expected first parents empty, got %v", evs[0].Parents)
        }
        if len(evs[1].Parents) != 1 || strings.TrimSpace(evs[1].Parents[0]) != strings.TrimSpace(evs[0].EventID) {
                t.Fatalf("expected second parent=%q, got %v", evs[0].EventID, evs[1].Parents)
        }
}

func TestAppendEventJSONL_AutoMergesForkHeads(t *testing.T) {
        dir := t.TempDir()

        res, err := EnsureGitBackedV1Layout(dir)
        if err != nil {
                t.Fatalf("EnsureGitBackedV1Layout: %v", err)
        }

        // Create an artificial fork: two head events for the same entity, with no parents.
        ev1 := EventV1{
                EventID:      "ev-1",
                WorkspaceID:  "ws-1",
                ReplicaID:    "rep-1",
                EntityKind:   EntityKindItem,
                EntityID:     "item-1",
                EntitySeq:    1,
                Type:         "item.set_title",
                IssuedAt:     time.Now().Add(-2 * time.Minute).UTC(),
                ActorID:      "act-1",
                Payload:      json.RawMessage(`{"title":"a"}`),
                LocalStatus:  "local",
                ServerStatus: "pending",
        }
        ev2 := EventV1{
                EventID:      "ev-2",
                WorkspaceID:  "ws-1",
                ReplicaID:    "rep-1",
                EntityKind:   EntityKindItem,
                EntityID:     "item-1",
                EntitySeq:    2,
                Type:         "item.set_description",
                IssuedAt:     time.Now().Add(-1 * time.Minute).UTC(),
                ActorID:      "act-1",
                Payload:      json.RawMessage(`{"description":"b"}`),
                LocalStatus:  "local",
                ServerStatus: "pending",
        }

        f, err := os.OpenFile(res.ShardPath, os.O_APPEND|os.O_WRONLY, 0o644)
        if err != nil {
                t.Fatalf("open shard: %v", err)
        }
        defer f.Close()
        for _, ev := range []EventV1{ev1, ev2} {
                b, _ := json.Marshal(ev)
                if _, err := f.Write(append(b, '\n')); err != nil {
                        t.Fatalf("write event: %v", err)
                }
        }

        s := Store{Dir: dir}
        if err := s.AppendEvent("act-1", "item.set_title", "item-1", map[string]any{"title": "c"}); err != nil {
                t.Fatalf("AppendEvent: %v", err)
        }

        // Expect: 2 fork events + 1 merge marker + 1 new event.
        ff, err := os.Open(res.ShardPath)
        if err != nil {
                t.Fatalf("open shard (read): %v", err)
        }
        defer ff.Close()

        sc := bufio.NewScanner(ff)
        var evs []EventV1
        for sc.Scan() {
                line := strings.TrimSpace(sc.Text())
                if line == "" {
                        continue
                }
                var ev EventV1
                if err := json.Unmarshal([]byte(line), &ev); err != nil {
                        t.Fatalf("parse event: %v", err)
                }
                evs = append(evs, ev)
        }
        if err := sc.Err(); err != nil {
                t.Fatalf("scan shard: %v", err)
        }
        if len(evs) != 4 {
                t.Fatalf("expected 4 events, got %d", len(evs))
        }

        merge := evs[2]
        if merge.Type != "item.merge" {
                t.Fatalf("expected merge type item.merge, got %q", merge.Type)
        }
        if len(merge.Parents) != 2 || merge.Parents[0] != "ev-1" || merge.Parents[1] != "ev-2" {
                t.Fatalf("expected merge parents [ev-1 ev-2], got %v", merge.Parents)
        }

        last := evs[3]
        if len(last.Parents) != 1 || strings.TrimSpace(last.Parents[0]) != strings.TrimSpace(merge.EventID) {
                t.Fatalf("expected last parent=%q, got %v", merge.EventID, last.Parents)
        }
}
