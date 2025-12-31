package store

import (
        "context"
        "database/sql"
        "encoding/json"
        "errors"
        "fmt"
        "os"
        "strings"
        "time"

        "clarity-cli/internal/model"
)

// LoadSQLite loads the workspace state from the workspace SQLite db (preferred: .clarity/index.sqlite).
// If the SQLite state is empty but a legacy db.json exists,
// it imports db.json into SQLite once (preserving your existing data) and then loads from SQLite.
func (s Store) LoadSQLite(ctx context.Context) (*DB, error) {
        db, err := s.openSQLite(ctx)
        if err != nil {
                return nil, err
        }
        defer db.Close()

        if err := migrateSQLiteState(ctx, db); err != nil {
                return nil, err
        }

        hasState, err := sqliteStateHasAnyRows(ctx, db)
        if err != nil {
                return nil, err
        }
        if !hasState {
                // One-time import from db.json if present.
                if b, err := os.ReadFile(s.dbPath()); err == nil && len(b) > 0 {
                        legacy, legacyOrderByID, err := loadWireDB(b)
                        if err != nil {
                                return nil, err
                        }
                        dirty := false
                        if legacy.NextIDs == nil {
                                legacy.NextIDs = map[string]int{}
                                dirty = true
                        }
                        if len(legacy.Items) == 0 && len(legacy.LegacyTasks) > 0 {
                                legacy.Items = legacy.LegacyTasks
                                legacy.LegacyTasks = nil
                                dirty = true
                        }
                        if legacy.Version == 0 {
                                legacy.Version = 1
                                dirty = true
                        }
                        if migrateLegacyDates(&legacy) {
                                dirty = true
                        }
                        if migrateOutlines(&legacy) {
                                dirty = true
                        }
                        if migrateRanks(&legacy, legacyOrderByID) {
                                dirty = true
                        }
                        if migrateLegacyIDs(&legacy) {
                                dirty = true
                        }
                        _ = dirty // migrations are applied in memory; we import the migrated version.

                        if err := s.SaveSQLite(ctx, &legacy); err != nil {
                                return nil, err
                        }
                }
        }

        return loadStateFromSQLite(ctx, db)
}

func (s Store) SaveSQLite(ctx context.Context, st *DB) error {
        if st == nil {
                return errors.New("nil db")
        }
        db, err := s.openSQLite(ctx)
        if err != nil {
                return err
        }
        defer db.Close()

        if err := migrateSQLiteState(ctx, db); err != nil {
                return err
        }

        tx, err := db.BeginTx(ctx, &sql.TxOptions{})
        if err != nil {
                return err
        }
        defer func() { _ = tx.Rollback() }()

        // Meta (current actor/project, version).
        if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO state_meta(k, v) VALUES(?, ?)`, "version", fmt.Sprintf("%d", st.Version)); err != nil {
                return err
        }
        if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO state_meta(k, v) VALUES(?, ?)`, "current_actor_id", strings.TrimSpace(st.CurrentActorID)); err != nil {
                return err
        }
        if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO state_meta(k, v) VALUES(?, ?)`, "current_project_id", strings.TrimSpace(st.CurrentProjectID)); err != nil {
                return err
        }

        // Replace-all strategy (simple + safe for early dev; we can optimize to incremental writes later).
        tables := []string{
                "actors",
                "projects",
                "outlines",
                "items",
                "deps",
                "comments",
                "worklog",
        }
        for _, t := range tables {
                if _, err := tx.ExecContext(ctx, `DELETE FROM `+t); err != nil {
                        return err
                }
        }

        nowMs := time.Now().UTC().UnixMilli()

        for _, a := range st.Actors {
                raw, _ := json.Marshal(a)
                if _, err := tx.ExecContext(ctx, `INSERT INTO actors(id, json, updated_at_unixms) VALUES(?, ?, ?)`, a.ID, string(raw), nowMs); err != nil {
                        return err
                }
        }
        for _, p := range st.Projects {
                raw, _ := json.Marshal(p)
                if _, err := tx.ExecContext(ctx, `INSERT INTO projects(id, name, archived, json, updated_at_unixms) VALUES(?, ?, ?, ?, ?)`,
                        p.ID, p.Name, boolToInt(p.Archived), string(raw), nowMs); err != nil {
                        return err
                }
        }
        for _, o := range st.Outlines {
                raw, _ := json.Marshal(o)
                name := ""
                if o.Name != nil {
                        name = strings.TrimSpace(*o.Name)
                }
                statusDefs, _ := json.Marshal(o.StatusDefs)
                if _, err := tx.ExecContext(ctx, `INSERT INTO outlines(id, project_id, name, archived, status_defs_json, json, updated_at_unixms) VALUES(?, ?, ?, ?, ?, ?, ?)`,
                        o.ID, o.ProjectID, name, boolToInt(o.Archived), string(statusDefs), string(raw), nowMs); err != nil {
                        return err
                }
        }
        for _, it := range st.Items {
                raw, _ := json.Marshal(it)
                parent := ""
                if it.ParentID != nil {
                        parent = strings.TrimSpace(*it.ParentID)
                }
                dueDate := ""
                if it.Due != nil {
                        dueDate = strings.TrimSpace(it.Due.Date)
                }
                schedDate := ""
                if it.Schedule != nil {
                        schedDate = strings.TrimSpace(it.Schedule.Date)
                }
                tagsJSON, _ := json.Marshal(it.Tags)
                assigned := ""
                if it.AssignedActorID != nil {
                        assigned = strings.TrimSpace(*it.AssignedActorID)
                }
                if _, err := tx.ExecContext(ctx, `INSERT INTO items(
                        id, project_id, outline_id, parent_id, rank,
                        title, status_id,
                        priority, on_hold, archived,
                        owner_actor_id, assigned_actor_id,
                        due_date, schedule_date,
                        tags_json,
                        json, updated_at_unixms
                ) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
                        it.ID, it.ProjectID, it.OutlineID, parent, strings.TrimSpace(it.Rank),
                        it.Title, strings.TrimSpace(it.StatusID),
                        boolToInt(it.Priority), boolToInt(it.OnHold), boolToInt(it.Archived),
                        strings.TrimSpace(it.OwnerActorID), assigned,
                        dueDate, schedDate,
                        string(tagsJSON),
                        string(raw), nowMs,
                ); err != nil {
                        return err
                }
        }
        for _, d := range st.Deps {
                raw, _ := json.Marshal(d)
                if _, err := tx.ExecContext(ctx, `INSERT INTO deps(id, from_item_id, to_item_id, type, json, updated_at_unixms) VALUES(?, ?, ?, ?, ?, ?)`,
                        d.ID, d.FromItemID, d.ToItemID, string(d.Type), string(raw), nowMs); err != nil {
                        return err
                }
        }
        for _, c := range st.Comments {
                raw, _ := json.Marshal(c)
                if _, err := tx.ExecContext(ctx, `INSERT INTO comments(id, entity_kind, entity_id, author_id, created_at_unixms, json, updated_at_unixms) VALUES(?, ?, ?, ?, ?, ?, ?)`,
                        c.ID, "item", c.ItemID, c.AuthorID, c.CreatedAt.UTC().UnixMilli(), string(raw), nowMs); err != nil {
                        return err
                }
        }
        for _, w := range st.Worklog {
                raw, _ := json.Marshal(w)
                if _, err := tx.ExecContext(ctx, `INSERT INTO worklog(id, entity_kind, entity_id, author_id, created_at_unixms, json, updated_at_unixms) VALUES(?, ?, ?, ?, ?, ?, ?)`,
                        w.ID, "item", w.ItemID, w.AuthorID, w.CreatedAt.UTC().UnixMilli(), string(raw), nowMs); err != nil {
                        return err
                }
        }

        return tx.Commit()
}

func migrateSQLiteState(ctx context.Context, db *sql.DB) error {
        stmts := []string{
                `CREATE TABLE IF NOT EXISTS state_meta (
                        k TEXT PRIMARY KEY,
                        v TEXT NOT NULL
                );`,
                `CREATE TABLE IF NOT EXISTS actors (
                        id TEXT PRIMARY KEY,
                        json TEXT NOT NULL,
                        updated_at_unixms INTEGER NOT NULL
                );`,
                `CREATE TABLE IF NOT EXISTS projects (
                        id TEXT PRIMARY KEY,
                        name TEXT NOT NULL,
                        archived INTEGER NOT NULL,
                        json TEXT NOT NULL,
                        updated_at_unixms INTEGER NOT NULL
                );`,
                `CREATE TABLE IF NOT EXISTS outlines (
                        id TEXT PRIMARY KEY,
                        project_id TEXT NOT NULL,
                        name TEXT NOT NULL,
                        archived INTEGER NOT NULL,
                        status_defs_json TEXT NOT NULL,
                        json TEXT NOT NULL,
                        updated_at_unixms INTEGER NOT NULL
                );`,
                `CREATE INDEX IF NOT EXISTS idx_outlines_project ON outlines(project_id);`,
                `CREATE TABLE IF NOT EXISTS items (
                        id TEXT PRIMARY KEY,
                        project_id TEXT NOT NULL,
                        outline_id TEXT NOT NULL,
                        parent_id TEXT NOT NULL,
                        rank TEXT NOT NULL,
                        title TEXT NOT NULL,
                        status_id TEXT NOT NULL,
                        priority INTEGER NOT NULL,
                        on_hold INTEGER NOT NULL,
                        archived INTEGER NOT NULL,
                        owner_actor_id TEXT NOT NULL,
                        assigned_actor_id TEXT NOT NULL,
                        due_date TEXT NOT NULL,
                        schedule_date TEXT NOT NULL,
                        tags_json TEXT NOT NULL,
                        json TEXT NOT NULL,
                        updated_at_unixms INTEGER NOT NULL
                );`,
                `CREATE INDEX IF NOT EXISTS idx_items_outline ON items(outline_id);`,
                `CREATE INDEX IF NOT EXISTS idx_items_status ON items(status_id);`,
                `CREATE INDEX IF NOT EXISTS idx_items_due ON items(due_date);`,
                `CREATE INDEX IF NOT EXISTS idx_items_sched ON items(schedule_date);`,
                `CREATE TABLE IF NOT EXISTS deps (
                        id TEXT PRIMARY KEY,
                        from_item_id TEXT NOT NULL,
                        to_item_id TEXT NOT NULL,
                        type TEXT NOT NULL,
                        json TEXT NOT NULL,
                        updated_at_unixms INTEGER NOT NULL
                );`,
                `CREATE INDEX IF NOT EXISTS idx_deps_from ON deps(from_item_id);`,
                `CREATE INDEX IF NOT EXISTS idx_deps_to ON deps(to_item_id);`,
                `CREATE TABLE IF NOT EXISTS comments (
                        id TEXT PRIMARY KEY,
                        entity_kind TEXT NOT NULL,
                        entity_id TEXT NOT NULL,
                        author_id TEXT NOT NULL,
                        created_at_unixms INTEGER NOT NULL,
                        json TEXT NOT NULL,
                        updated_at_unixms INTEGER NOT NULL
                );`,
                `CREATE INDEX IF NOT EXISTS idx_comments_entity ON comments(entity_kind, entity_id, created_at_unixms);`,
                `CREATE TABLE IF NOT EXISTS worklog (
                        id TEXT PRIMARY KEY,
                        entity_kind TEXT NOT NULL,
                        entity_id TEXT NOT NULL,
                        author_id TEXT NOT NULL,
                        created_at_unixms INTEGER NOT NULL,
                        json TEXT NOT NULL,
                        updated_at_unixms INTEGER NOT NULL
                );`,
                `CREATE INDEX IF NOT EXISTS idx_worklog_entity ON worklog(entity_kind, entity_id, created_at_unixms);`,
        }
        for _, st := range stmts {
                if _, err := db.ExecContext(ctx, st); err != nil {
                        return err
                }
        }
        return nil
}

func sqliteStateHasAnyRows(ctx context.Context, db *sql.DB) (bool, error) {
        qs := []string{
                `SELECT COUNT(1) FROM items`,
                `SELECT COUNT(1) FROM projects`,
                `SELECT COUNT(1) FROM actors`,
        }
        for _, q := range qs {
                var n int
                if err := db.QueryRowContext(ctx, q).Scan(&n); err != nil {
                        // If tables don't exist yet, treat as empty.
                        return false, nil
                }
                if n > 0 {
                        return true, nil
                }
        }
        return false, nil
}

func loadStateFromSQLite(ctx context.Context, db *sql.DB) (*DB, error) {
        out := &DB{Version: 1, NextIDs: map[string]int{}}

        // Meta.
        readMeta := func(k string) string {
                var v string
                _ = db.QueryRowContext(ctx, `SELECT v FROM state_meta WHERE k = ?`, k).Scan(&v)
                return strings.TrimSpace(v)
        }
        if v := readMeta("version"); v != "" {
                // best-effort parse
                if n, err := strconvAtoi(v); err == nil {
                        out.Version = n
                }
        }
        out.CurrentActorID = readMeta("current_actor_id")
        out.CurrentProjectID = readMeta("current_project_id")

        // Load rows as JSON blobs.
        if xs, err := readJSONRows[model.Actor](ctx, db, `SELECT json FROM actors`); err == nil {
                out.Actors = xs
        } else {
                return nil, err
        }
        if xs, err := readJSONRows[model.Project](ctx, db, `SELECT json FROM projects`); err == nil {
                out.Projects = xs
        } else {
                return nil, err
        }
        if xs, err := readJSONRows[model.Outline](ctx, db, `SELECT json FROM outlines`); err == nil {
                out.Outlines = xs
        } else {
                return nil, err
        }
        if xs, err := readJSONRows[model.Item](ctx, db, `SELECT json FROM items`); err == nil {
                out.Items = xs
        } else {
                return nil, err
        }
        if xs, err := readJSONRows[model.Dependency](ctx, db, `SELECT json FROM deps`); err == nil {
                out.Deps = xs
        } else {
                return nil, err
        }
        if xs, err := readJSONRows[model.Comment](ctx, db, `SELECT json FROM comments`); err == nil {
                out.Comments = xs
        } else {
                return nil, err
        }
        if xs, err := readJSONRows[model.WorklogEntry](ctx, db, `SELECT json FROM worklog`); err == nil {
                out.Worklog = xs
        } else {
                return nil, err
        }

        // Ensure nil slices are empty for stable callers.
        if out.Actors == nil {
                out.Actors = []model.Actor{}
        }
        if out.Projects == nil {
                out.Projects = []model.Project{}
        }
        if out.Outlines == nil {
                out.Outlines = []model.Outline{}
        }
        if out.Items == nil {
                out.Items = []model.Item{}
        }
        if out.Deps == nil {
                out.Deps = []model.Dependency{}
        }
        if out.Comments == nil {
                out.Comments = []model.Comment{}
        }
        if out.Worklog == nil {
                out.Worklog = []model.WorklogEntry{}
        }

        return out, nil
}

func readJSONRows[T any](ctx context.Context, db *sql.DB, query string) ([]T, error) {
        rows, err := db.QueryContext(ctx, query)
        if err != nil {
                return nil, err
        }
        defer rows.Close()

        var out []T
        for rows.Next() {
                var js string
                if err := rows.Scan(&js); err != nil {
                        return nil, err
                }
                var v T
                if err := json.Unmarshal([]byte(js), &v); err != nil {
                        return nil, err
                }
                out = append(out, v)
        }
        if err := rows.Err(); err != nil {
                return nil, err
        }
        return out, nil
}

func boolToInt(b bool) int {
        if b {
                return 1
        }
        return 0
}

func strconvAtoi(s string) (int, error) {
        // tiny helper to avoid pulling in strconv in multiple files; keep localized here.
        n := 0
        for _, r := range strings.TrimSpace(s) {
                if r < '0' || r > '9' {
                        return 0, errors.New("invalid int")
                }
                n = n*10 + int(r-'0')
        }
        return n, nil
}
