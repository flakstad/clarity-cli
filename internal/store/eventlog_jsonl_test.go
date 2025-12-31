package store

import (
        "os"
        "path/filepath"
        "strings"
        "testing"
)

func TestJSONLEventLog_ShardsAppendAndRead(t *testing.T) {
        withEnv(t, envEventLogBackend, string(EventLogBackendJSONL), func() {
                withEnv(t, "CLARITY_CONFIG_DIR", t.TempDir(), func() {
                        dir := t.TempDir()
                        s := Store{Dir: dir}

                        if err := s.AppendEvent("act-a", "item.create", "item-1", map[string]any{"id": "item-1"}); err != nil {
                                t.Fatalf("append 1: %v", err)
                        }
                        if err := s.AppendEvent("act-a", "item.set_title", "item-1", map[string]any{"title": "A"}); err != nil {
                                t.Fatalf("append 2: %v", err)
                        }

                        evs, err := ReadEvents(dir, 0)
                        if err != nil {
                                t.Fatalf("read events: %v", err)
                        }
                        if len(evs) != 2 {
                                t.Fatalf("expected 2 events, got %d", len(evs))
                        }
                        if evs[0].Type != "item.create" || evs[1].Type != "item.set_title" {
                                t.Fatalf("unexpected types: %q then %q", evs[0].Type, evs[1].Type)
                        }

                        byEntity, err := ReadEventsForEntity(dir, "item-1", 0)
                        if err != nil {
                                t.Fatalf("read entity events: %v", err)
                        }
                        if len(byEntity) != 2 {
                                t.Fatalf("expected 2 entity events, got %d", len(byEntity))
                        }

                        // Ensure we wrote to a shard file in events/.
                        entries, err := os.ReadDir(filepath.Join(dir, "events"))
                        if err != nil {
                                t.Fatalf("readdir events/: %v", err)
                        }
                        var shardFiles []string
                        for _, ent := range entries {
                                if ent.IsDir() {
                                        continue
                                }
                                name := ent.Name()
                                if strings.HasPrefix(name, "events.") && strings.HasSuffix(name, ".jsonl") {
                                        shardFiles = append(shardFiles, name)
                                }
                        }
                        if len(shardFiles) != 1 {
                                t.Fatalf("expected 1 shard file, got %d (%v)", len(shardFiles), shardFiles)
                        }
                })
        })
}
