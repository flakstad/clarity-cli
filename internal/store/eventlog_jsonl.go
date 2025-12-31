package store

import (
        "bufio"
        "bytes"
        "context"
        "database/sql"
        "encoding/json"
        "errors"
        "fmt"
        "os"
        "path/filepath"
        "sort"
        "strings"
        "time"

        "clarity-cli/internal/model"
)

func (s Store) workspaceRoot() string {
        dir := filepath.Clean(s.Dir)
        if filepath.Base(dir) == ".clarity" {
                return filepath.Dir(dir)
        }
        return dir
}

func (s Store) eventsDir() string {
        return filepath.Join(s.workspaceRoot(), "events")
}

func (s Store) shardPath(replicaID string) string {
        replicaID = strings.TrimSpace(replicaID)
        if replicaID == "" {
                // Fallback to legacy filename (single-writer without a replica id).
                return filepath.Join(s.eventsDir(), "events.jsonl")
        }
        return filepath.Join(s.eventsDir(), fmt.Sprintf("events.%s.jsonl", replicaID))
}

func (s Store) appendEventJSONL(ctx context.Context, actorID, typ, entityID string, payload any) error {
        // Use the same contract validation as the SQLite backend.
        kind := inferEntityKindFromType(typ)
        if !kind.valid() {
                return formatErrEventContract("invalid entity kind for type %q", typ)
        }
        entityID = strings.TrimSpace(entityID)
        if entityID == "" {
                return formatErrEventContract("missing entity id")
        }
        typ = strings.TrimSpace(typ)
        if typ == "" {
                return formatErrEventContract("missing type")
        }
        actorID = strings.TrimSpace(actorID)
        if actorID == "" {
                return formatErrEventContract("missing actor id")
        }

        // Temporary bridge: allocate (workspaceId, replicaId) and (entitySeq, parents) via the existing
        // SQLite meta tables. As we pivot fully to Git+JSONL canonical storage, this becomes derived state
        // rebuilt during reindex/doctor operations.
        db, err := s.openSQLite(ctx)
        if err != nil {
                return err
        }
        defer db.Close()

        wsID, err := ensureMetaUUID(ctx, db, "workspace_id")
        if err != nil {
                return err
        }
        repID, err := ensureMetaUUID(ctx, db, "replica_id")
        if err != nil {
                return err
        }

        now := time.Now().UTC()

        pb, err := json.Marshal(payload)
        if err != nil {
                return err
        }
        eventID, err := newUUIDv4()
        if err != nil {
                return err
        }

        // Allocate seq + parent via the same logic as appendEventSQLite, but without inserting into sqlite events.
        seq, parents, err := allocateEntitySeqAndParents(ctx, db, kind, entityID, eventID)
        if err != nil {
                return err
        }

        ev := EventV1{
                EventID:     eventID,
                WorkspaceID: wsID,
                ReplicaID:   repID,

                EntityKind: kind,
                EntityID:   entityID,
                EntitySeq:  seq,

                Type:    typ,
                Parents: parents,

                IssuedAt: now,
                ActorID:  actorID,
                Payload:  json.RawMessage(pb),

                LocalStatus:  "local",
                ServerStatus: "pending",
        }

        if err := os.MkdirAll(s.eventsDir(), 0o755); err != nil {
                return err
        }
        path := s.shardPath(repID)

        f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
        if err != nil {
                return err
        }
        defer f.Close()

        line, err := json.Marshal(ev)
        if err != nil {
                return err
        }
        if _, err := f.Write(append(line, '\n')); err != nil {
                return err
        }
        return nil
}

func allocateEntitySeqAndParents(ctx context.Context, db *sql.DB, kind EntityKind, entityID, nextHeadEventID string) (int64, []string, error) {
        tx, err := db.BeginTx(ctx, &sql.TxOptions{})
        if err != nil {
                return 0, nil, err
        }
        defer func() { _ = tx.Rollback() }()

        rows, err := tx.QueryContext(ctx, `SELECT head_event_id FROM entity_heads WHERE entity_kind = ? AND entity_id = ?`, kind.String(), entityID)
        if err != nil {
                return 0, nil, err
        }
        var heads []string
        for rows.Next() {
                var h string
                if err := rows.Scan(&h); err != nil {
                        _ = rows.Close()
                        return 0, nil, err
                }
                h = strings.TrimSpace(h)
                if h != "" {
                        heads = append(heads, h)
                }
        }
        _ = rows.Close()
        if err := rows.Err(); err != nil {
                return 0, nil, err
        }
        if len(heads) > 1 {
                return 0, nil, fmt.Errorf("conflict: entity has %d heads; requires explicit merge", len(heads))
        }

        var parents []string
        if len(heads) == 1 {
                parents = []string{heads[0]}
        }

        var next int64
        err = tx.QueryRowContext(ctx, `SELECT next_seq FROM entity_seq WHERE entity_kind = ? AND entity_id = ?`, kind.String(), entityID).Scan(&next)
        switch {
        case err == nil:
                // ok
        case errors.Is(err, sql.ErrNoRows):
                next = 1
                if _, err := tx.ExecContext(ctx, `INSERT INTO entity_seq(entity_kind, entity_id, next_seq) VALUES(?, ?, ?)`, kind.String(), entityID, int64(2)); err != nil {
                        return 0, nil, err
                }
        default:
                return 0, nil, err
        }
        seq := next
        if next > 1 {
                if _, err := tx.ExecContext(ctx, `UPDATE entity_seq SET next_seq = ? WHERE entity_kind = ? AND entity_id = ?`, next+1, kind.String(), entityID); err != nil {
                        return 0, nil, err
                }
        }

        if _, err := tx.ExecContext(ctx, `DELETE FROM entity_heads WHERE entity_kind = ? AND entity_id = ?`, kind.String(), entityID); err != nil {
                return 0, nil, err
        }
        if _, err := tx.ExecContext(ctx, `INSERT INTO entity_heads(entity_kind, entity_id, head_event_id) VALUES(?, ?, ?)`, kind.String(), entityID, strings.TrimSpace(nextHeadEventID)); err != nil {
                return 0, nil, err
        }

        if err := tx.Commit(); err != nil {
                return 0, nil, err
        }
        return seq, parents, nil
}

func (s Store) readEventsJSONL(limit int) ([]model.Event, error) {
        evs, err := s.readEventsV1JSONL()
        if err != nil {
                return nil, err
        }
        sort.Slice(evs, func(i, j int) bool {
                if evs[i].IssuedAt.Equal(evs[j].IssuedAt) {
                        return evs[i].EventID < evs[j].EventID
                }
                return evs[i].IssuedAt.Before(evs[j].IssuedAt)
        })

        out := make([]model.Event, 0, len(evs))
        for _, e := range evs {
                var payload any
                _ = json.Unmarshal(e.Payload, &payload)
                out = append(out, model.Event{
                        ID:       e.EventID,
                        TS:       e.IssuedAt.UTC(),
                        ActorID:  e.ActorID,
                        Type:     e.Type,
                        EntityID: e.EntityID,
                        Payload:  payload,
                })
                if limit > 0 && len(out) >= limit {
                        break
                }
        }
        if out == nil {
                out = []model.Event{}
        }
        return out, nil
}

func (s Store) readEventsForEntityJSONL(entityID string, limit int) ([]model.Event, error) {
        entityID = strings.TrimSpace(entityID)
        if entityID == "" {
                return []model.Event{}, nil
        }
        evs, err := s.readEventsV1JSONL()
        if err != nil {
                return nil, err
        }
        // Keep v1 semantics: per-entity order is seq, then timestamp/id tie-break.
        sort.Slice(evs, func(i, j int) bool {
                if strings.TrimSpace(evs[i].EntityID) != entityID && strings.TrimSpace(evs[j].EntityID) == entityID {
                        return false
                }
                if strings.TrimSpace(evs[i].EntityID) == entityID && strings.TrimSpace(evs[j].EntityID) != entityID {
                        return true
                }
                if evs[i].EntitySeq != evs[j].EntitySeq {
                        return evs[i].EntitySeq < evs[j].EntitySeq
                }
                if evs[i].IssuedAt.Equal(evs[j].IssuedAt) {
                        return evs[i].EventID < evs[j].EventID
                }
                return evs[i].IssuedAt.Before(evs[j].IssuedAt)
        })

        var out []model.Event
        for _, e := range evs {
                if strings.TrimSpace(e.EntityID) != entityID {
                        continue
                }
                var payload any
                _ = json.Unmarshal(e.Payload, &payload)
                out = append(out, model.Event{
                        ID:       e.EventID,
                        TS:       e.IssuedAt.UTC(),
                        ActorID:  e.ActorID,
                        Type:     e.Type,
                        EntityID: e.EntityID,
                        Payload:  payload,
                })
                if limit > 0 && len(out) >= limit {
                        break
                }
        }
        if out == nil {
                out = []model.Event{}
        }
        return out, nil
}

func (s Store) readEventsV1JSONL() ([]EventV1, error) {
        // Support both:
        // - sharded logs: events/events.<replica-id>.jsonl (preferred)
        // - legacy single log: events/events.jsonl
        dir := s.eventsDir()
        entries, err := os.ReadDir(dir)
        if err != nil {
                if errors.Is(err, os.ErrNotExist) {
                        return []EventV1{}, nil
                }
                return nil, err
        }
        var paths []string
        for _, ent := range entries {
                if ent.IsDir() {
                        continue
                }
                name := ent.Name()
                if !strings.HasPrefix(name, "events") || !strings.HasSuffix(name, ".jsonl") {
                        continue
                }
                paths = append(paths, filepath.Join(dir, name))
        }
        sort.Strings(paths)

        var out []EventV1
        for _, p := range paths {
                evs, err := readEventV1Lines(p)
                if err != nil {
                        return nil, err
                }
                out = append(out, evs...)
        }
        if out == nil {
                out = []EventV1{}
        }
        return out, nil
}

func readEventV1Lines(path string) ([]EventV1, error) {
        f, err := os.Open(path)
        if err != nil {
                if errors.Is(err, os.ErrNotExist) {
                        return []EventV1{}, nil
                }
                return nil, err
        }
        defer f.Close()

        sc := bufio.NewScanner(f)
        sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

        var out []EventV1
        for sc.Scan() {
                b := bytes.TrimSpace(sc.Bytes())
                if len(b) == 0 {
                        continue
                }
                var ev EventV1
                if err := json.Unmarshal(b, &ev); err != nil {
                        return nil, err
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
