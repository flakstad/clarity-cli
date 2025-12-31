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

type EventV1Line struct {
        Path  string
        Line  int
        Event EventV1
}

func ReadEventsV1Lines(dir string) ([]EventV1Line, error) {
        st := Store{Dir: dir}
        return st.readEventsV1LinesJSONL()
}

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

        // V1 Git-backed mode: workspaceId is committed (meta/workspace.json); replicaId is local-only
        // (.clarity/device.json, gitignored). This keeps the canonical log append-only and merge-friendly.
        wsMeta, _, err := s.loadOrInitWorkspaceMeta()
        if err != nil {
                return err
        }
        device, _, err := s.loadOrInitDeviceFile()
        if err != nil {
                return err
        }
        wsID := strings.TrimSpace(wsMeta.WorkspaceID)
        repID := strings.TrimSpace(device.ReplicaID)

        now := time.Now().UTC()

        pb, err := json.Marshal(payload)
        if err != nil {
                return err
        }
        eventID, err := newUUIDv4()
        if err != nil {
                return err
        }

        ev := EventV1{
                EventID:     eventID,
                WorkspaceID: wsID,
                ReplicaID:   repID,

                EntityKind: kind,
                EntityID:   entityID,
                EntitySeq:  0,

                Type:    typ,
                Parents: nil,

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
        evs, err := s.readEventsV1LinesJSONL()
        if err != nil {
                return nil, err
        }
        sort.Slice(evs, func(i, j int) bool {
                a := evs[i].Event
                b := evs[j].Event
                if a.IssuedAt.Equal(b.IssuedAt) {
                        return a.EventID < b.EventID
                }
                return a.IssuedAt.Before(b.IssuedAt)
        })

        out := make([]model.Event, 0, len(evs))
        for _, l := range evs {
                e := l.Event
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
        evs, err := s.readEventsV1LinesJSONL()
        if err != nil {
                return nil, err
        }
        // Keep v1 semantics: per-entity order is seq, then timestamp/id tie-break.
        sort.Slice(evs, func(i, j int) bool {
                a := evs[i].Event
                b := evs[j].Event
                if strings.TrimSpace(a.EntityID) != entityID && strings.TrimSpace(b.EntityID) == entityID {
                        return false
                }
                if strings.TrimSpace(a.EntityID) == entityID && strings.TrimSpace(b.EntityID) != entityID {
                        return true
                }
                if a.EntitySeq != b.EntitySeq {
                        return a.EntitySeq < b.EntitySeq
                }
                if a.IssuedAt.Equal(b.IssuedAt) {
                        return a.EventID < b.EventID
                }
                return a.IssuedAt.Before(b.IssuedAt)
        })

        var out []model.Event
        for _, l := range evs {
                e := l.Event
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

func (s Store) readEventsV1LinesJSONL() ([]EventV1Line, error) {
        // Support both:
        // - sharded logs: events/events.<replica-id>.jsonl (preferred)
        // - legacy single log: events/events.jsonl
        dir := s.eventsDir()
        entries, err := os.ReadDir(dir)
        if err != nil {
                if errors.Is(err, os.ErrNotExist) {
                        return []EventV1Line{}, nil
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

        var out []EventV1Line
        for _, p := range paths {
                evs, err := readEventV1Lines(p)
                if err != nil {
                        return nil, err
                }
                out = append(out, evs...)
        }
        if out == nil {
                out = []EventV1Line{}
        }
        return out, nil
}

func readEventV1Lines(path string) ([]EventV1Line, error) {
        f, err := os.Open(path)
        if err != nil {
                if errors.Is(err, os.ErrNotExist) {
                        return []EventV1Line{}, nil
                }
                return nil, err
        }
        defer f.Close()

        sc := bufio.NewScanner(f)
        sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

        var out []EventV1Line
        lineNo := 0
        for sc.Scan() {
                lineNo++
                b := bytes.TrimSpace(sc.Bytes())
                if len(b) == 0 {
                        continue
                }
                var ev EventV1
                if err := json.Unmarshal(b, &ev); err != nil {
                        return nil, fmt.Errorf("%s:%d: %w", path, lineNo, err)
                }
                out = append(out, EventV1Line{Path: path, Line: lineNo, Event: ev})
        }
        if err := sc.Err(); err != nil {
                return nil, err
        }
        if out == nil {
                out = []EventV1Line{}
        }
        return out, nil
}
