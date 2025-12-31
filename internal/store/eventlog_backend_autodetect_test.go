package store

import (
        "os"
        "path/filepath"
        "testing"
)

func TestEventLogBackend_AutoDetectsJSONLWhenEventsExist(t *testing.T) {
        withEnv(t, envEventLogBackend, "", func() {
                withEnv(t, "CLARITY_CONFIG_DIR", t.TempDir(), func() {
                        dir := t.TempDir()
                        if err := os.MkdirAll(filepath.Join(dir, "events"), 0o755); err != nil {
                                t.Fatalf("mkdir events: %v", err)
                        }
                        if err := os.WriteFile(filepath.Join(dir, "events", "events.rep-a.jsonl"), []byte("{}\n"), 0o644); err != nil {
                                t.Fatalf("write events file: %v", err)
                        }

                        s := Store{Dir: dir}
                        if got := s.eventLogBackend(); got != EventLogBackendJSONL {
                                t.Fatalf("expected %q, got %q", EventLogBackendJSONL, got)
                        }
                })
        })
}
