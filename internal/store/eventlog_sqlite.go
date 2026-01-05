package store

import (
        "context"
        "crypto/rand"
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

        _ "modernc.org/sqlite"
)

func getenv(k string) string { return os.Getenv(k) }

func (s Store) existingSQLitePath() (string, bool) {
        // V1: derived index location (preferred).
        preferred := filepath.Join(s.localDir(), "index.sqlite")
        if _, err := os.Stat(preferred); err == nil {
                return preferred, true
        }

        // Legacy: single-file workspace db.
        legacy := filepath.Join(s.localDir(), "clarity.sqlite")
        if _, err := os.Stat(legacy); err == nil {
                return legacy, true
        }

        // Legacy: store-root sqlite (pre ".clarity/index.sqlite" era).
        legacyRoot := filepath.Join(filepath.Clean(s.Dir), "clarity.sqlite")
        if _, err := os.Stat(legacyRoot); err == nil {
                return legacyRoot, true
        }

        return "", false
}

func (s Store) sqlitePath() string {
        // V1: derived index location (preferred).
        preferred := filepath.Join(s.localDir(), "index.sqlite")
        if _, err := os.Stat(preferred); err == nil {
                return preferred
        }

        // Legacy: single-file workspace db.
        legacy := filepath.Join(s.localDir(), "clarity.sqlite")
        if _, err := os.Stat(legacy); err == nil {
                return legacy
        }

        // Legacy: store-root sqlite (pre ".clarity/index.sqlite" era).
        legacyRoot := filepath.Join(filepath.Clean(s.Dir), "clarity.sqlite")
        if _, err := os.Stat(legacyRoot); err == nil {
                return legacyRoot
        }

        // Default to preferred, creating it if missing.
        return preferred
}

func (s Store) openSQLite(ctx context.Context) (*sql.DB, error) {
        if err := s.Ensure(); err != nil {
                return nil, err
        }
        // modernc.org/sqlite driver name is "sqlite".
        db, err := sql.Open("sqlite", s.sqlitePath())
        if err != nil {
                return nil, err
        }
        // Pragmas for multi-process local usage.
        // WAL enables one writer + many readers; busy_timeout helps avoid "database is locked" flakiness.
        pragmas := []string{
                "PRAGMA journal_mode=WAL;",
                "PRAGMA synchronous=NORMAL;",
                "PRAGMA foreign_keys=ON;",
                "PRAGMA busy_timeout=5000;",
        }
        for _, p := range pragmas {
                if _, err := db.ExecContext(ctx, p); err != nil {
                        _ = db.Close()
                        return nil, err
                }
        }
        if err := s.migrateSQLite(ctx, db); err != nil {
                _ = db.Close()
                return nil, err
        }
        return db, nil
}

func (s Store) migrateSQLite(ctx context.Context, db *sql.DB) error {
        stmts := []string{
                `CREATE TABLE IF NOT EXISTS meta (
                        k TEXT PRIMARY KEY,
                        v TEXT NOT NULL
                );`,
                `CREATE TABLE IF NOT EXISTS events (
                        event_id TEXT PRIMARY KEY,
                        workspace_id TEXT NOT NULL,
                        replica_id TEXT NOT NULL,
                        entity_kind TEXT NOT NULL,
                        entity_id TEXT NOT NULL,
                        entity_seq INTEGER NOT NULL,
                        type TEXT NOT NULL,
                        parents_json TEXT NOT NULL,
                        issued_at_unixms INTEGER NOT NULL,
                        actor_id TEXT NOT NULL,
                        payload_json TEXT NOT NULL,
                        local_status TEXT NOT NULL,
                        server_status TEXT NOT NULL,
                        rejection_reason TEXT,
                        published_at_unixms INTEGER,
                        created_at_unixms INTEGER NOT NULL
                );`,
                `CREATE INDEX IF NOT EXISTS idx_events_entity ON events(entity_kind, entity_id, entity_seq);`,
                `CREATE INDEX IF NOT EXISTS idx_events_issued ON events(issued_at_unixms);`,
                `CREATE TABLE IF NOT EXISTS entity_heads (
                        entity_kind TEXT NOT NULL,
                        entity_id TEXT NOT NULL,
                        head_event_id TEXT NOT NULL,
                        PRIMARY KEY(entity_kind, entity_id, head_event_id)
                );`,
                `CREATE TABLE IF NOT EXISTS entity_seq (
                        entity_kind TEXT NOT NULL,
                        entity_id TEXT NOT NULL,
                        next_seq INTEGER NOT NULL,
                        PRIMARY KEY(entity_kind, entity_id)
                );`,
        }
        for _, st := range stmts {
                if _, err := db.ExecContext(ctx, st); err != nil {
                        return err
                }
        }

        // Ensure workspaceId exists.
        wsID, err := ensureMetaUUID(ctx, db, "workspace_id")
        if err != nil {
                return err
        }

        // Only use global config (device/workspace->replica mapping) for workspace-root stores.
        // This avoids writing ~/.clarity/config.json for ephemeral --dir usage (tests/fixtures).
        useGlobalReplicaMap := false
        if cfgDir, err := ConfigDir(); err == nil {
                wsRoot := filepath.Join(cfgDir, "workspaces") + string(os.PathSeparator)
                dir := filepath.Clean(s.Dir) + string(os.PathSeparator)
                if strings.HasPrefix(dir, wsRoot) {
                        useGlobalReplicaMap = true
                }
        }

        // Ensure replicaId exists and has desired clone behavior:
        // - Clone workspace dir to another machine => same workspace_id, but that machine generates a new replicaId
        //   and overwrites the copied sqlite meta replica_id.
        // - Move/rename workspace on the same machine => replicaId stays stable.
        //
        // Mechanism: store a per-device mapping workspaceId -> replicaId in ~/.clarity/config.json.
        if useGlobalReplicaMap {
                if cfg, cfgErr := LoadConfig(); cfgErr == nil {
                        _, dirty1, _ := ensureDeviceID(cfg)
                        wantReplicaID, dirty2, err := ensureReplicaIDForWorkspace(cfg, wsID)
                        if err != nil {
                                return err
                        }
                        if dirty1 || dirty2 {
                                _ = SaveConfig(cfg) // best-effort
                        }
                        curReplicaID, err := ensureMetaUUID(ctx, db, "replica_id")
                        if err != nil {
                                return err
                        }
                        if strings.TrimSpace(curReplicaID) != strings.TrimSpace(wantReplicaID) {
                                if _, err := db.ExecContext(ctx, `INSERT OR REPLACE INTO meta(k, v) VALUES(?, ?)`, "replica_id", wantReplicaID); err != nil {
                                        return err
                                }
                        }
                        return nil
                }
        }

        // Fallback: keep replicaId inside sqlite only.
        _, err = ensureMetaUUID(ctx, db, "replica_id")
        return err
}

func ensureMetaUUID(ctx context.Context, db *sql.DB, key string) (string, error) {
        key = strings.TrimSpace(key)
        if key == "" {
                return "", errors.New("empty meta key")
        }
        var v string
        err := db.QueryRowContext(ctx, `SELECT v FROM meta WHERE k = ?`, key).Scan(&v)
        if err == nil && strings.TrimSpace(v) != "" {
                return v, nil
        }
        if err != nil && !errors.Is(err, sql.ErrNoRows) {
                return "", err
        }
        id, err := newUUIDv4()
        if err != nil {
                return "", err
        }
        if _, err := db.ExecContext(ctx, `INSERT OR REPLACE INTO meta(k, v) VALUES(?, ?)`, key, id); err != nil {
                return "", err
        }
        return id, nil
}

func ensureDeviceID(cfg *GlobalConfig) (string, bool, error) {
        if cfg == nil {
                return "", false, errors.New("nil config")
        }
        if strings.TrimSpace(cfg.DeviceID) != "" {
                return strings.TrimSpace(cfg.DeviceID), false, nil
        }
        id, err := newUUIDv4()
        if err != nil {
                return "", false, err
        }
        cfg.DeviceID = id
        return id, true, nil
}

func ensureReplicaIDForWorkspace(cfg *GlobalConfig, workspaceID string) (string, bool, error) {
        if cfg == nil {
                return "", false, errors.New("nil config")
        }
        workspaceID = strings.TrimSpace(workspaceID)
        if workspaceID == "" {
                return "", false, errors.New("empty workspaceID")
        }
        if cfg.Replicas == nil {
                cfg.Replicas = map[string]string{}
        }
        if id := strings.TrimSpace(cfg.Replicas[workspaceID]); id != "" {
                return id, false, nil
        }
        id, err := newUUIDv4()
        if err != nil {
                return "", false, err
        }
        cfg.Replicas[workspaceID] = id
        return id, true, nil
}

func newUUIDv4() (string, error) {
        var b [16]byte
        if _, err := rand.Read(b[:]); err != nil {
                return "", err
        }
        // RFC 4122 variant + v4
        b[6] = (b[6] & 0x0f) | 0x40
        b[8] = (b[8] & 0x3f) | 0x80
        return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
                uint32(b[0])<<24|uint32(b[1])<<16|uint32(b[2])<<8|uint32(b[3]),
                uint16(b[4])<<8|uint16(b[5]),
                uint16(b[6])<<8|uint16(b[7]),
                uint16(b[8])<<8|uint16(b[9]),
                uint64(b[10])<<40|uint64(b[11])<<32|uint64(b[12])<<24|uint64(b[13])<<16|uint64(b[14])<<8|uint64(b[15]),
        ), nil
}

func (s Store) appendEventSQLite(ctx context.Context, actorID, typ, entityID string, payload any) error {
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
        nowMs := now.UnixMilli()

        // Marshal payload to JSON for durability (command events).
        pb, err := json.Marshal(payload)
        if err != nil {
                return err
        }

        eventID, err := newUUIDv4()
        if err != nil {
                return err
        }

        tx, err := db.BeginTx(ctx, &sql.TxOptions{})
        if err != nil {
                return err
        }
        defer func() { _ = tx.Rollback() }()

        // Read current heads (allowing for sync-era forks; locally we require <=1).
        rows, err := tx.QueryContext(ctx, `SELECT head_event_id FROM entity_heads WHERE entity_kind = ? AND entity_id = ?`, kind.String(), entityID)
        if err != nil {
                return err
        }
        var heads []string
        for rows.Next() {
                var h string
                if err := rows.Scan(&h); err != nil {
                        _ = rows.Close()
                        return err
                }
                h = strings.TrimSpace(h)
                if h != "" {
                        heads = append(heads, h)
                }
        }
        _ = rows.Close()
        if err := rows.Err(); err != nil {
                return err
        }
        sort.Strings(heads)

        // Auto-merge forks (multiple heads) by appending an explicit merge marker, then
        // appending the intended event with the merge marker as its single parent.
        //
        // This keeps the per-entity stream append-only and resolves conflicts without
        // requiring a manual merge for v1.
        mergeID := ""
        var mergeParentsJSON []byte
        if len(heads) > 1 {
                mergeID, err = newUUIDv4()
                if err != nil {
                        return err
                }
                mergeParentsJSON, _ = json.Marshal(heads)
        }

        var parents []string
        switch {
        case mergeID != "":
                parents = []string{mergeID}
        case len(heads) == 1:
                parents = []string{heads[0]}
        default:
                parents = nil
        }
        parentsJSON, _ := json.Marshal(parents)

        // Allocate per-entity sequence.
        var next int64
        err = tx.QueryRowContext(ctx, `SELECT next_seq FROM entity_seq WHERE entity_kind = ? AND entity_id = ?`, kind.String(), entityID).Scan(&next)
        switch {
        case err == nil:
                // ok
        case errors.Is(err, sql.ErrNoRows):
                next = 1
                if _, err := tx.ExecContext(ctx, `INSERT INTO entity_seq(entity_kind, entity_id, next_seq) VALUES(?, ?, ?)`, kind.String(), entityID, int64(2)); err != nil {
                        return err
                }
        default:
                return err
        }
        step := int64(1)
        if mergeID != "" {
                step = 2
        }

        seq := next
        if next > 1 || step > 1 {
                if _, err := tx.ExecContext(ctx, `UPDATE entity_seq SET next_seq = ? WHERE entity_kind = ? AND entity_id = ?`, next+step, kind.String(), entityID); err != nil {
                        return err
                }
        }

        // When we auto-merge, allocate the merge marker its own seq and the actual event
        // becomes seq+1.
        mergeSeq := int64(0)
        eventSeq := seq
        if mergeID != "" {
                mergeSeq = seq
                eventSeq = seq + 1
        }

        // Insert merge marker, if needed.
        if mergeID != "" {
                mp := map[string]any{
                        "auto":  true,
                        "heads": heads,
                }
                mb, err := json.Marshal(mp)
                if err != nil {
                        return err
                }
                mergeType := fmt.Sprintf("%s.merge", strings.TrimSpace(kind.String()))
                if _, err := tx.ExecContext(ctx, `
                        INSERT INTO events(
                                event_id, workspace_id, replica_id,
                                entity_kind, entity_id, entity_seq,
                                type, parents_json,
                                issued_at_unixms, actor_id, payload_json,
                                local_status, server_status,
                                rejection_reason, published_at_unixms, created_at_unixms
                        ) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, ?)
                `, mergeID, wsID, repID, kind.String(), entityID, mergeSeq, mergeType, string(mergeParentsJSON), nowMs, actorID, string(mb), "local", "pending", nowMs); err != nil {
                        return err
                }
        }

        // Insert intended event (server_status starts pending; local_status starts local).
        if _, err := tx.ExecContext(ctx, `
                INSERT INTO events(
                        event_id, workspace_id, replica_id,
                        entity_kind, entity_id, entity_seq,
                        type, parents_json,
                        issued_at_unixms, actor_id, payload_json,
                        local_status, server_status,
                        rejection_reason, published_at_unixms, created_at_unixms
                ) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, ?)
        `, eventID, wsID, repID, kind.String(), entityID, eventSeq, typ, string(parentsJSON), nowMs, actorID, string(pb), "local", "pending", nowMs); err != nil {
                return err
        }

        // Advance head: enforce linear local history by replacing any existing heads with the new one.
        if _, err := tx.ExecContext(ctx, `DELETE FROM entity_heads WHERE entity_kind = ? AND entity_id = ?`, kind.String(), entityID); err != nil {
                return err
        }
        if _, err := tx.ExecContext(ctx, `INSERT INTO entity_heads(entity_kind, entity_id, head_event_id) VALUES(?, ?, ?)`, kind.String(), entityID, eventID); err != nil {
                return err
        }

        return tx.Commit()
}

func (s Store) readEventsSQLite(ctx context.Context, limit int) ([]model.Event, error) {
        db, err := s.openSQLite(ctx)
        if err != nil {
                return nil, err
        }
        defer db.Close()

        q := `SELECT event_id, issued_at_unixms, actor_id, type, entity_id, payload_json
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

        var out []model.Event
        for rows.Next() {
                var id, actor, typ, entityID, payloadJSON string
                var tsMs int64
                if err := rows.Scan(&id, &tsMs, &actor, &typ, &entityID, &payloadJSON); err != nil {
                        return nil, err
                }
                var payload any
                _ = json.Unmarshal([]byte(payloadJSON), &payload)
                out = append(out, model.Event{
                        ID:       id,
                        TS:       time.UnixMilli(tsMs).UTC(),
                        ActorID:  actor,
                        Type:     typ,
                        EntityID: entityID,
                        Payload:  payload,
                })
        }
        if err := rows.Err(); err != nil {
                return nil, err
        }
        if out == nil {
                out = []model.Event{}
        }
        return out, nil
}

func (s Store) readEventsForEntitySQLite(ctx context.Context, entityID string, limit int) ([]model.Event, error) {
        entityID = strings.TrimSpace(entityID)
        if entityID == "" {
                return []model.Event{}, nil
        }
        db, err := s.openSQLite(ctx)
        if err != nil {
                return nil, err
        }
        defer db.Close()

        q := `SELECT event_id, issued_at_unixms, actor_id, type, entity_id, payload_json
              FROM events
              WHERE entity_id = ?
              ORDER BY entity_seq ASC`
        var rows *sql.Rows
        if limit > 0 {
                rows, err = db.QueryContext(ctx, q+` LIMIT ?`, entityID, limit)
        } else {
                rows, err = db.QueryContext(ctx, q, entityID)
        }
        if err != nil {
                return nil, err
        }
        defer rows.Close()

        var out []model.Event
        for rows.Next() {
                var id, actor, typ, eid, payloadJSON string
                var tsMs int64
                if err := rows.Scan(&id, &tsMs, &actor, &typ, &eid, &payloadJSON); err != nil {
                        return nil, err
                }
                var payload any
                _ = json.Unmarshal([]byte(payloadJSON), &payload)
                out = append(out, model.Event{
                        ID:       id,
                        TS:       time.UnixMilli(tsMs).UTC(),
                        ActorID:  actor,
                        Type:     typ,
                        EntityID: eid,
                        Payload:  payload,
                })
        }
        if err := rows.Err(); err != nil {
                return nil, err
        }
        if out == nil {
                out = []model.Event{}
        }
        return out, nil
}
