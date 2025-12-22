package store

import (
        "os"
        "testing"
)

func withEnv(t *testing.T, k, v string, fn func()) {
        t.Helper()
        old, had := os.LookupEnv(k)
        if err := os.Setenv(k, v); err != nil {
                t.Fatalf("setenv %s: %v", k, err)
        }
        t.Cleanup(func() {
                if had {
                        _ = os.Setenv(k, old)
                } else {
                        _ = os.Unsetenv(k)
                }
        })
        fn()
}

func TestSQLiteEventLog_AppendAndRead(t *testing.T) {
        withEnv(t, envEventLogBackend, string(EventLogBackendSQLite), func() {
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

                        tail, err := ReadEventsTail(dir, 1)
                        if err != nil {
                                t.Fatalf("read tail: %v", err)
                        }
                        if len(tail) != 1 || tail[0].Type != "item.set_title" {
                                t.Fatalf("unexpected tail: %+v", tail)
                        }
                })
        })
}
