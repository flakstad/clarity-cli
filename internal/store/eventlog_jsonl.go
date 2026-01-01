package store

import (
        "bufio"
        "bytes"
        "context"
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

func WalkEventsV1Lines(dir string, fn func(EventV1Line) error) error {
        st := Store{Dir: dir}
        return st.walkEventsV1LinesJSONL(fn)
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

        ids, parentIDs, maxSeq, err := s.scanEntityIDsParentsMaxSeqV1(kind, entityID)
        if err != nil {
                return err
        }
        heads := computeHeads(ids, parentIDs)
        if len(heads) > 1 {
                return fmt.Errorf("conflict: entity has %d heads; requires explicit merge (try: clarity doctor --fail)", len(heads))
        }
        var parents []string
        if len(heads) == 1 {
                parents = []string{heads[0]}
        }
        seq := int64(1)
        switch {
        case len(ids) == 0:
                seq = 1
        case maxSeq > 0:
                seq = maxSeq + 1
        default:
                // Legacy compatibility: if existing JSONL events have entitySeq=0, allocate a
                // stable-ish seq based on count to keep per-entity ordering usable.
                seq = int64(len(ids) + 1)
        }

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
        var out []EventV1Line
        err := s.walkEventsV1LinesJSONL(func(l EventV1Line) error {
                out = append(out, l)
                return nil
        })
        if err != nil {
                return nil, err
        }
        if out == nil {
                out = []EventV1Line{}
        }
        return out, nil
}

func (s Store) walkEventsV1LinesJSONL(fn func(EventV1Line) error) error {
        // Support both:
        // - sharded logs: events/events.<replica-id>.jsonl (preferred)
        // - legacy single log: events/events.jsonl
        dir := s.eventsDir()
        entries, err := os.ReadDir(dir)
        if err != nil {
                if errors.Is(err, os.ErrNotExist) {
                        return nil
                }
                return err
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

        for _, p := range paths {
                f, err := os.Open(p)
                if err != nil {
                        if errors.Is(err, os.ErrNotExist) {
                                continue
                        }
                        return err
                }

                sc := bufio.NewScanner(f)
                sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

                lineNo := 0
                for sc.Scan() {
                        lineNo++
                        b := bytes.TrimSpace(sc.Bytes())
                        if len(b) == 0 {
                                continue
                        }
                        var ev EventV1
                        if err := json.Unmarshal(b, &ev); err != nil {
                                _ = f.Close()
                                return fmt.Errorf("%s:%d: %w", p, lineNo, err)
                        }
                        if err := fn(EventV1Line{Path: p, Line: lineNo, Event: ev}); err != nil {
                                _ = f.Close()
                                return err
                        }
                }
                _ = f.Close()
                if err := sc.Err(); err != nil {
                        return err
                }
        }
        return nil
}

func (s Store) scanEntityIDsParentsMaxSeqV1(kind EntityKind, entityID string) (map[string]struct{}, map[string]struct{}, int64, error) {
        kind = EntityKind(strings.TrimSpace(string(kind)))
        entityID = strings.TrimSpace(entityID)
        if !kind.valid() || entityID == "" {
                return map[string]struct{}{}, map[string]struct{}{}, 0, nil
        }

        ids := map[string]struct{}{}
        parentIDs := map[string]struct{}{}
        var maxSeq int64
        err := s.walkEventsV1LinesJSONL(func(l EventV1Line) error {
                ev := l.Event
                if ev.EntityKind != kind || strings.TrimSpace(ev.EntityID) != entityID {
                        return nil
                }
                id := strings.TrimSpace(ev.EventID)
                if id != "" {
                        ids[id] = struct{}{}
                }
                if ev.EntitySeq > maxSeq {
                        maxSeq = ev.EntitySeq
                }
                for _, p := range ev.Parents {
                        p = strings.TrimSpace(p)
                        if p != "" {
                                parentIDs[p] = struct{}{}
                        }
                }
                return nil
        })
        if err != nil {
                return nil, nil, 0, err
        }
        return ids, parentIDs, maxSeq, nil
}

func computeHeads(ids, parentIDs map[string]struct{}) []string {
        var heads []string
        for id := range ids {
                if _, ok := parentIDs[id]; ok {
                        continue
                }
                heads = append(heads, id)
        }
        sort.Strings(heads)
        if heads == nil {
                heads = []string{}
        }
        return heads
}
