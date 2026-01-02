package store

import (
        "context"
        "encoding/json"
        "os"
        "strings"
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

func TestSQLiteEventLog_AutoMergesForksOnAppend(t *testing.T) {
        withEnv(t, envEventLogBackend, string(EventLogBackendSQLite), func() {
                withEnv(t, "CLARITY_CONFIG_DIR", t.TempDir(), func() {
                        dir := t.TempDir()
                        s := Store{Dir: dir}

                        ctx := context.Background()
                        db, err := s.openSQLite(ctx)
                        if err != nil {
                                t.Fatalf("open sqlite: %v", err)
                        }
                        defer db.Close()

                        wsID, err := ensureMetaUUID(ctx, db, "workspace_id")
                        if err != nil {
                                t.Fatalf("workspace_id: %v", err)
                        }
                        repID, err := ensureMetaUUID(ctx, db, "replica_id")
                        if err != nil {
                                t.Fatalf("replica_id: %v", err)
                        }

                        kind := EntityKindItem.String()
                        entityID := "item-1"
                        nowMs := int64(1700000000000)
                        actorID := "act-a"

                        // Create an artificial fork: two events with no parent, both marked as heads.
                        head1, err := newUUIDv4()
                        if err != nil {
                                t.Fatalf("uuid1: %v", err)
                        }
                        head2, err := newUUIDv4()
                        if err != nil {
                                t.Fatalf("uuid2: %v", err)
                        }
                        payloadJSON, _ := json.Marshal(map[string]any{"id": entityID})
                        emptyParents := "[]"

                        insertEvent := func(eventID string, seq int64, typ string) {
                                t.Helper()
                                if _, err := db.ExecContext(ctx, `
                                        INSERT INTO events(
                                                event_id, workspace_id, replica_id,
                                                entity_kind, entity_id, entity_seq,
                                                type, parents_json,
                                                issued_at_unixms, actor_id, payload_json,
                                                local_status, server_status,
                                                rejection_reason, published_at_unixms, created_at_unixms
                                        ) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, ?)
                                `, eventID, wsID, repID, kind, entityID, seq, typ, emptyParents, nowMs, actorID, string(payloadJSON), "local", "pending", nowMs); err != nil {
                                        t.Fatalf("insert event %s: %v", eventID, err)
                                }
                        }

                        insertEvent(head1, 1, "item.create")
                        insertEvent(head2, 2, "item.set_title")

                        if _, err := db.ExecContext(ctx, `INSERT INTO entity_seq(entity_kind, entity_id, next_seq) VALUES(?, ?, ?)`, kind, entityID, int64(3)); err != nil {
                                t.Fatalf("insert entity_seq: %v", err)
                        }
                        if _, err := db.ExecContext(ctx, `INSERT INTO entity_heads(entity_kind, entity_id, head_event_id) VALUES(?, ?, ?)`, kind, entityID, head1); err != nil {
                                t.Fatalf("insert head1: %v", err)
                        }
                        if _, err := db.ExecContext(ctx, `INSERT INTO entity_heads(entity_kind, entity_id, head_event_id) VALUES(?, ?, ?)`, kind, entityID, head2); err != nil {
                                t.Fatalf("insert head2: %v", err)
                        }

                        if err := s.AppendEvent(actorID, "item.set_title", entityID, map[string]any{"title": "merged"}); err != nil {
                                t.Fatalf("append after fork: %v", err)
                        }

                        rows, err := db.QueryContext(ctx, `SELECT event_id, type, parents_json FROM events WHERE entity_kind = ? AND entity_id = ? ORDER BY entity_seq ASC`, kind, entityID)
                        if err != nil {
                                t.Fatalf("query events: %v", err)
                        }
                        defer rows.Close()

                        type row struct {
                                id      string
                                typ     string
                                parents string
                        }
                        var got []row
                        for rows.Next() {
                                var r row
                                if err := rows.Scan(&r.id, &r.typ, &r.parents); err != nil {
                                        t.Fatalf("scan: %v", err)
                                }
                                got = append(got, r)
                        }
                        if err := rows.Err(); err != nil {
                                t.Fatalf("rows: %v", err)
                        }
                        if len(got) != 4 {
                                t.Fatalf("expected 4 events after merge, got %d", len(got))
                        }

                        if got[2].typ != "item.merge" {
                                t.Fatalf("expected merge marker at seq 3, got %q", got[2].typ)
                        }
                        if !strings.Contains(got[2].parents, head1) || !strings.Contains(got[2].parents, head2) {
                                t.Fatalf("merge parents missing heads: %s", got[2].parents)
                        }
                        if got[3].typ != "item.set_title" {
                                t.Fatalf("expected intended event at seq 4, got %q", got[3].typ)
                        }
                        if !strings.Contains(got[3].parents, got[2].id) {
                                t.Fatalf("intended event should parent merge marker %s, got parents %s", got[2].id, got[3].parents)
                        }

                        // Heads should be a single head (the new event).
                        var nHeads int
                        if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM entity_heads WHERE entity_kind = ? AND entity_id = ?`, kind, entityID).Scan(&nHeads); err != nil {
                                t.Fatalf("count heads: %v", err)
                        }
                        if nHeads != 1 {
                                t.Fatalf("expected 1 head after merge, got %d", nHeads)
                        }
                })
        })
}
