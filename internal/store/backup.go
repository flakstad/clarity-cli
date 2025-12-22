package store

import (
        "bufio"
        "context"
        "database/sql"
        "encoding/json"
        "errors"
        "fmt"
        "os"
        "strings"
        "time"
)

// ReadEventsV1 returns durable v1 events from the SQLite event log.
//
// limit == 0 means "all".
func (s Store) ReadEventsV1(ctx context.Context, limit int) ([]EventV1, error) {
        db, err := s.openSQLite(ctx)
        if err != nil {
                return nil, err
        }
        defer db.Close()

        q := `SELECT
                event_id, workspace_id, replica_id,
                entity_kind, entity_id, entity_seq,
                type, parents_json,
                issued_at_unixms, actor_id, payload_json,
                local_status, server_status,
                rejection_reason, published_at_unixms
        FROM events
        ORDER BY created_at_unixms ASC`
        var rows *sql.Rows
        if limit > 0 {
                rows, err = db.QueryContext(ctx, q+` LIMIT ?`, limit)
        } else {
                rows, err = db.QueryContext(ctx, q)
        }
        if err != nil {
                return nil, err
        }
        defer rows.Close()

        var out []EventV1
        for rows.Next() {
                var (
                        id, wsID, repID, kind, entityID, typ, parentsJSON, actorID, payloadJSON, localStatus, serverStatus string
                        seq                                                                                                int64
                        issuedAtMs                                                                                         int64
                        rej                                                                                                sql.NullString
                        publishedAtMs                                                                                      sql.NullInt64
                )
                if err := rows.Scan(
                        &id, &wsID, &repID,
                        &kind, &entityID, &seq,
                        &typ, &parentsJSON,
                        &issuedAtMs, &actorID, &payloadJSON,
                        &localStatus, &serverStatus,
                        &rej, &publishedAtMs,
                ); err != nil {
                        return nil, err
                }

                var parents []string
                _ = json.Unmarshal([]byte(parentsJSON), &parents)

                var rejPtr *string
                if rej.Valid {
                        v := strings.TrimSpace(rej.String)
                        if v != "" {
                                rejPtr = &v
                        }
                }

                var pubPtr *int64
                if publishedAtMs.Valid {
                        v := publishedAtMs.Int64
                        pubPtr = &v
                }

                payload := json.RawMessage(strings.TrimSpace(payloadJSON))
                if len(payload) == 0 {
                        payload = json.RawMessage("null")
                }

                out = append(out, EventV1{
                        EventID:           id,
                        WorkspaceID:       wsID,
                        ReplicaID:         repID,
                        EntityKind:        EntityKind(kind),
                        EntityID:          entityID,
                        EntitySeq:         seq,
                        Type:              typ,
                        Parents:           parents,
                        IssuedAt:          time.UnixMilli(issuedAtMs).UTC(),
                        ActorID:           actorID,
                        Payload:           payload,
                        LocalStatus:       localStatus,
                        ServerStatus:      serverStatus,
                        RejectionReason:   rejPtr,
                        PublishedAtUnixMs: pubPtr,
                })
        }
        if err := rows.Err(); err != nil {
                return nil, err
        }
        if out == nil {
                out = []EventV1{}
        }
        return out, nil
}

// ReplaceEventsV1 replaces the SQLite event log with the provided v1 events.
//
// This is intended for backup/restore workflows, not day-to-day mutations.
//
// If workspaceID is non-empty, it is written into meta.workspace_id prior to the event import,
// and migrateSQLite is rerun to ensure replica_id behavior remains correct for this device.
func (s Store) ReplaceEventsV1(ctx context.Context, workspaceID string, evs []EventV1) error {
        db, err := s.openSQLite(ctx)
        if err != nil {
                return err
        }
        defer db.Close()

        tx, err := db.BeginTx(ctx, &sql.TxOptions{})
        if err != nil {
                return err
        }
        defer func() { _ = tx.Rollback() }()

        // Clear derived tables first.
        stmts := []string{
                `DELETE FROM events;`,
                `DELETE FROM entity_heads;`,
                `DELETE FROM entity_seq;`,
        }
        for _, st := range stmts {
                if _, err := tx.ExecContext(ctx, st); err != nil {
                        return err
                }
        }

        if strings.TrimSpace(workspaceID) != "" {
                if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO meta(k, v) VALUES(?, ?)`, "workspace_id", strings.TrimSpace(workspaceID)); err != nil {
                        return err
                }
        }

        // Rebuild heads + seq after inserting events.
        type entKey struct {
                kind string
                id   string
        }
        type entAgg struct {
                maxSeq    int64
                allIDs    map[string]struct{}
                parentIDs map[string]struct{}
        }
        agg := map[entKey]*entAgg{}

        nowMs := time.Now().UTC().UnixMilli()

        for _, ev := range evs {
                id := strings.TrimSpace(ev.EventID)
                if id == "" {
                        return errors.New("backup: event has empty eventId")
                }
                kind := strings.TrimSpace(ev.EntityKind.String())
                entityID := strings.TrimSpace(ev.EntityID)
                if kind == "" || entityID == "" {
                        return errors.New("backup: event has empty entityKind/entityId")
                }
                typ := strings.TrimSpace(ev.Type)
                if typ == "" {
                        return errors.New("backup: event has empty type")
                }
                wsID := strings.TrimSpace(ev.WorkspaceID)
                repID := strings.TrimSpace(ev.ReplicaID)
                actorID := strings.TrimSpace(ev.ActorID)
                if wsID == "" || repID == "" || actorID == "" {
                        return errors.New("backup: event has empty workspaceId/replicaId/actorId")
                }

                parentsJSON, _ := json.Marshal(ev.Parents)
                payload := ev.Payload
                if len(payload) == 0 {
                        payload = json.RawMessage("null")
                }
                issuedAtMs := ev.IssuedAt.UTC().UnixMilli()
                if issuedAtMs == 0 {
                        issuedAtMs = nowMs
                }

                var rej any = nil
                if ev.RejectionReason != nil && strings.TrimSpace(*ev.RejectionReason) != "" {
                        rej = strings.TrimSpace(*ev.RejectionReason)
                }
                var pub any = nil
                if ev.PublishedAtUnixMs != nil {
                        pub = *ev.PublishedAtUnixMs
                }

                if _, err := tx.ExecContext(ctx, `
                        INSERT INTO events(
                                event_id, workspace_id, replica_id,
                                entity_kind, entity_id, entity_seq,
                                type, parents_json,
                                issued_at_unixms, actor_id, payload_json,
                                local_status, server_status,
                                rejection_reason, published_at_unixms, created_at_unixms
                        ) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                `,
                        id, wsID, repID,
                        kind, entityID, ev.EntitySeq,
                        typ, string(parentsJSON),
                        issuedAtMs, actorID, string(payload),
                        strings.TrimSpace(ev.LocalStatus), strings.TrimSpace(ev.ServerStatus),
                        rej, pub, issuedAtMs,
                ); err != nil {
                        return err
                }

                k := entKey{kind: kind, id: entityID}
                a := agg[k]
                if a == nil {
                        a = &entAgg{maxSeq: 0, allIDs: map[string]struct{}{}, parentIDs: map[string]struct{}{}}
                        agg[k] = a
                }
                a.allIDs[id] = struct{}{}
                for _, p := range ev.Parents {
                        p = strings.TrimSpace(p)
                        if p != "" {
                                a.parentIDs[p] = struct{}{}
                        }
                }
                if ev.EntitySeq > a.maxSeq {
                        a.maxSeq = ev.EntitySeq
                }
        }

        // Populate entity_seq + entity_heads.
        for k, a := range agg {
                next := a.maxSeq + 1
                if next < 1 {
                        next = 1
                }
                if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO entity_seq(entity_kind, entity_id, next_seq) VALUES(?, ?, ?)`, k.kind, k.id, next); err != nil {
                        return err
                }

                // Heads are events that are not a parent of any other event in this entity stream.
                for id := range a.allIDs {
                        if _, ok := a.parentIDs[id]; ok {
                                continue
                        }
                        if _, err := tx.ExecContext(ctx, `INSERT INTO entity_heads(entity_kind, entity_id, head_event_id) VALUES(?, ?, ?)`, k.kind, k.id, id); err != nil {
                                return err
                        }
                }
        }

        if err := tx.Commit(); err != nil {
                return err
        }

        // Ensure replica_id semantics match the current device after restoring a workspace_id.
        return s.migrateSQLite(ctx, db)
}

// WriteEventsJSONL writes a JSONL stream of EventV1 (one event per line).
func WriteEventsJSONL(path string, evs []EventV1) error {
        f, err := os.Create(path)
        if err != nil {
                return err
        }
        defer f.Close()

        bw := bufio.NewWriter(f)
        enc := json.NewEncoder(bw)
        for _, ev := range evs {
                if err := enc.Encode(ev); err != nil {
                        return err
                }
        }
        return bw.Flush()
}

// ReadEventsJSONL reads EventV1 events from a JSONL file.
func ReadEventsJSONL(path string) ([]EventV1, error) {
        f, err := os.Open(path)
        if err != nil {
                return nil, err
        }
        defer f.Close()

        var out []EventV1
        sc := bufio.NewScanner(f)
        for sc.Scan() {
                line := strings.TrimSpace(sc.Text())
                if line == "" {
                        continue
                }
                var ev EventV1
                if err := json.Unmarshal([]byte(line), &ev); err != nil {
                        return nil, fmt.Errorf("parse events jsonl: %w", err)
                }
                out = append(out, ev)
        }
        if err := sc.Err(); err != nil {
                return nil, err
        }
        if out == nil {
                out = []EventV1{}
        }
        return out, nil
}
