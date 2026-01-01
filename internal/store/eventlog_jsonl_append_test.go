package store

import (
        "bufio"
        "encoding/json"
        "os"
        "strings"
        "testing"
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
