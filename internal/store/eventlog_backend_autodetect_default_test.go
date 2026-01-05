package store

import (
        "context"
        "testing"
)

func TestEventLogBackend_DefaultsToJSONLWhenNoBackendDetected(t *testing.T) {
        withEnv(t, envEventLogBackend, "", func() {
                withEnv(t, "CLARITY_CONFIG_DIR", t.TempDir(), func() {
                        dir := t.TempDir()
                        s := Store{Dir: dir}
                        if got := s.eventLogBackend(); got != EventLogBackendJSONL {
                                t.Fatalf("expected default %q, got %q", EventLogBackendJSONL, got)
                        }
                })
        })
}

func TestEventLogBackend_AutoDetectsSQLiteWhenLegacyEventLogTablesExist(t *testing.T) {
        withEnv(t, "CLARITY_CONFIG_DIR", t.TempDir(), func() {
                dir := t.TempDir()
                s := Store{Dir: dir}

                // Create legacy SQLite event log tables via an append (forced by env).
                withEnv(t, envEventLogBackend, string(EventLogBackendSQLite), func() {
                        if err := s.AppendEvent("actor-1", "project.create", "project-1", map[string]any{
                                "name": "Test",
                        }); err != nil {
                                t.Fatalf("AppendEvent (sqlite): %v", err)
                        }
                })

                // Now auto-detect without env and ensure we stick with SQLite.
                withEnv(t, envEventLogBackend, "", func() {
                        if got := s.eventLogBackend(); got != EventLogBackendSQLite {
                                t.Fatalf("expected %q, got %q", EventLogBackendSQLite, got)
                        }

                        // Sanity: reads still work under autodetected backend.
                        if evs, err := s.ReadEventsV1(context.Background(), 0); err != nil {
                                t.Fatalf("ReadEventsV1: %v", err)
                        } else if len(evs) == 0 {
                                t.Fatalf("expected events")
                        }
                })
        })
}
