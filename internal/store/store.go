package store

import (
        "bufio"
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

const (
        dbFileName     = "db.json"
        eventsFileName = "events.jsonl"
)

type DB struct {
        Version          int                  `json:"version"`
        CurrentActorID   string               `json:"currentActorId,omitempty"`
        CurrentProjectID string               `json:"currentProjectId,omitempty"`
        NextIDs          map[string]int       `json:"nextIds"`
        Actors           []model.Actor        `json:"actors"`
        Projects         []model.Project      `json:"projects"`
        Outlines         []model.Outline      `json:"outlines"`
        Items            []model.Item         `json:"items"`
        LegacyTasks      []model.Item         `json:"tasks,omitempty"`
        Deps             []model.Dependency   `json:"deps"`
        Comments         []model.Comment      `json:"comments"`
        Worklog          []model.WorklogEntry `json:"worklog"`

        // Derived indexes for fast per-item lookups in the TUI. These are not persisted.
        idxBuilt            bool                            `json:"-"`
        idxChildrenByParent map[string][]model.Item         `json:"-"`
        idxCommentsByItem   map[string][]model.Comment      `json:"-"`
        idxWorklogByItem    map[string][]model.WorklogEntry `json:"-"`
}

type Store struct {
        Dir string
}

func DiscoverDir(start string) (string, bool) {
        dir := start
        for {
                candidate := filepath.Join(dir, ".clarity")
                if st, err := os.Stat(candidate); err == nil && st.IsDir() {
                        return candidate, true
                }
                parent := filepath.Dir(dir)
                if parent == dir {
                        return "", false
                }
                dir = parent
        }
}

func DefaultDir() (string, error) {
        cwd, err := os.Getwd()
        if err != nil {
                return "", err
        }
        if found, ok := DiscoverDir(cwd); ok {
                return found, nil
        }
        return filepath.Join(cwd, ".clarity"), nil
}

func WorkspaceDir(name string) (string, error) {
        name, err := NormalizeWorkspaceName(name)
        if err != nil {
                return "", err
        }
        dir, err := ConfigDir()
        if err != nil {
                return "", err
        }
        return filepath.Join(dir, "workspaces", name), nil
}

func (s Store) Ensure() error {
        return os.MkdirAll(s.Dir, 0o755)
}

func (s Store) dbPath() string {
        return filepath.Join(s.Dir, dbFileName)
}

func (s Store) Load() (*DB, error) {
        if err := s.Ensure(); err != nil {
                return nil, err
        }

        // SQLite is the only source of truth. LoadSQLite will auto-import legacy db.json once if needed.
        return s.LoadSQLite(context.Background())
}

func migrateRanks(db *DB, legacyOrderByID map[string]int) bool {
        // V1: ordering is stored as per-item rank (lexicographic) per sibling group.
        // Older DBs used an integer order. Migrate groups that have missing ranks.
        changed := false

        type key struct {
                outlineID string
                parentID  string // "" for nil
        }

        groups := map[key][]int{}
        for i := range db.Items {
                it := &db.Items[i]
                pid := ""
                if it.ParentID != nil {
                        pid = *it.ParentID
                }
                k := key{outlineID: it.OutlineID, parentID: pid}
                groups[k] = append(groups[k], i)
        }

        for _, idxs := range groups {
                needs := false
                for _, idx := range idxs {
                        if strings.TrimSpace(db.Items[idx].Rank) == "" {
                                needs = true
                                break
                        }
                }
                if !needs {
                        continue
                }

                sort.Slice(idxs, func(i, j int) bool {
                        a := db.Items[idxs[i]]
                        b := db.Items[idxs[j]]
                        oa, oka := legacyOrderByID[a.ID]
                        ob, okb := legacyOrderByID[b.ID]
                        if oka && okb && oa != ob {
                                return oa < ob
                        }
                        return a.CreatedAt.Before(b.CreatedAt)
                })

                prev := ""
                for _, idx := range idxs {
                        it := &db.Items[idx]
                        if strings.TrimSpace(it.Rank) != "" {
                                prev = it.Rank
                                continue
                        }
                        var next string
                        if strings.TrimSpace(prev) == "" {
                                r, err := RankInitial()
                                if err != nil {
                                        r = "h"
                                }
                                next = r
                        } else {
                                r, err := RankAfter(prev)
                                if err != nil {
                                        r = prev + "0"
                                }
                                next = r
                        }
                        it.Rank = next
                        prev = next
                        changed = true
                }
        }

        return changed
}

func migrateLegacyIDs(db *DB) bool {
        changed := false
        // deps: fromTaskId/toTaskId -> fromItemId/toItemId
        for i := range db.Deps {
                d := &db.Deps[i]
                if d.FromItemID == "" && d.LegacyFromTaskID != "" {
                        d.FromItemID = d.LegacyFromTaskID
                        changed = true
                }
                if d.ToItemID == "" && d.LegacyToTaskID != "" {
                        d.ToItemID = d.LegacyToTaskID
                        changed = true
                }
        }

        // comments/worklog: taskId -> itemId
        for i := range db.Comments {
                c := &db.Comments[i]
                if c.ItemID == "" && c.LegacyTaskID != "" {
                        c.ItemID = c.LegacyTaskID
                        changed = true
                }
        }
        for i := range db.Worklog {
                w := &db.Worklog[i]
                if w.ItemID == "" && w.LegacyTaskID != "" {
                        w.ItemID = w.LegacyTaskID
                        changed = true
                }
        }
        return changed
}

func migrateOutlines(db *DB) bool {
        changed := false
        // V1: every item must belong to an outline. Older DBs won't have outlines.
        // Strategy:
        // - Create one unnamed default outline per project IF that project has items without outline IDs
        // - Attach those items to their project's outline
        // - Normalize status IDs TODO/DOING/DONE -> todo/doing/done
        if db.Outlines == nil {
                db.Outlines = []model.Outline{}
                changed = true
        }

        projectToOutline := map[string]string{}
        for _, o := range db.Outlines {
                projectToOutline[o.ProjectID] = o.ID
        }

        next := func(prefix string) string { return (&Store{}).NextID(db, prefix) }

        projectNeedsOutline := map[string]bool{}
        for i := range db.Items {
                if db.Items[i].OutlineID == "" && db.Items[i].ProjectID != "" {
                        projectNeedsOutline[db.Items[i].ProjectID] = true
                }
        }

        for i := range db.Projects {
                pid := db.Projects[i].ID
                if !projectNeedsOutline[pid] {
                        continue
                }
                if _, ok := projectToOutline[pid]; ok {
                        continue
                }
                oid := next("out")
                projectToOutline[pid] = oid
                db.Outlines = append(db.Outlines, model.Outline{
                        ID:         oid,
                        ProjectID:  pid,
                        Name:       nil,
                        StatusDefs: DefaultOutlineStatusDefs(),
                        CreatedBy:  db.Projects[i].CreatedBy,
                        CreatedAt:  db.Projects[i].CreatedAt,
                })
                changed = true
        }

        for i := range db.Items {
                t := &db.Items[i]
                if t.OutlineID == "" {
                        if oid, ok := projectToOutline[t.ProjectID]; ok {
                                t.OutlineID = oid
                                changed = true
                        }
                }
                switch t.StatusID {
                case "TODO":
                        t.StatusID = "todo"
                        changed = true
                case "DOING":
                        t.StatusID = "doing"
                        changed = true
                case "DONE":
                        t.StatusID = "done"
                        changed = true
                }
        }
        return changed
}

func migrateLegacyDates(db *DB) bool {
        changed := false
        for i := range db.Items {
                t := &db.Items[i]
                if t.Due == nil && t.LegacyDueAt != nil {
                        t.Due = legacyTimeToDateTime(*t.LegacyDueAt)
                        changed = true
                }
                if t.Schedule == nil && t.LegacyScheduledAt != nil {
                        t.Schedule = legacyTimeToDateTime(*t.LegacyScheduledAt)
                        changed = true
                }
        }
        return changed
}

func legacyTimeToDateTime(ts time.Time) *model.DateTime {
        // Best-effort migration: we previously used 09:00 local default for date-only;
        // detect that and migrate to date-only.
        ts = ts.UTC()
        date := ts.Format("2006-01-02")
        hm := ts.Format("15:04")
        if hm == "09:00" || hm == "00:00" {
                return &model.DateTime{Date: date, Time: nil}
        }
        return &model.DateTime{Date: date, Time: &hm}
}

func (s Store) Save(db *DB) error {
        if err := s.Ensure(); err != nil {
                return err
        }
        // SQLite-only: never write db.json.
        return s.SaveSQLite(context.Background(), db)
}

func (s Store) NextID(db *DB, prefix string) string {
        // Hash-based IDs (stable-ish length) instead of sequential integers.
        // Prefixes remain for readability: item-xxx, proj-xxx, out-xxx, etc.
        //
        // NOTE: NextIDs is kept for backwards compatibility with old db.json files,
        // but is no longer used as the source of IDs.
        baseLen := idSuffixLen(prefix)
        tryLens := []int{baseLen}
        // For the short, user-facing ids (3 chars), gracefully expand if we collide repeatedly.
        // This keeps the "usually 3 chars" UX but avoids falling back to integer ids.
        if baseLen == 3 {
                tryLens = append(tryLens, 4, 5, 6, 8)
        }

        for _, ln := range tryLens {
                maxAttempts := 10
                switch ln {
                case 3:
                        maxAttempts = 200
                case 4:
                        maxAttempts = 50
                }
                for i := 0; i < maxAttempts; i++ {
                        id, err := newRandomIDWithLen(prefix, ln)
                        if err != nil {
                                // fallback: keep old behavior if crypto/rand fails
                                if db.NextIDs == nil {
                                        db.NextIDs = map[string]int{}
                                }
                                db.NextIDs[prefix]++
                                return fmt.Sprintf("%s-%d", prefix, db.NextIDs[prefix])
                        }
                        if !idExists(db, id) {
                                return id
                        }
                }
        }
        // Extremely unlikely fallback
        if db.NextIDs == nil {
                db.NextIDs = map[string]int{}
        }
        db.NextIDs[prefix]++
        return fmt.Sprintf("%s-%d", prefix, db.NextIDs[prefix])
}

func (s Store) AppendEvent(actorID, typ, entityID string, payload any) error {
        // SQLite-only: never write events.jsonl.
        return s.appendEventSQLite(context.Background(), actorID, typ, entityID, payload)
}

func (db *DB) FindActor(id string) (*model.Actor, bool) {
        for i := range db.Actors {
                if db.Actors[i].ID == id {
                        return &db.Actors[i], true
                }
        }
        return nil, false
}

// HumanUserIDForActor returns the owning human user id for an actor.
// - human actor => itself
// - agent actor => actor.UserID (required)
func (db *DB) HumanUserIDForActor(actorID string) (string, bool) {
        a, ok := db.FindActor(actorID)
        if !ok {
                return "", false
        }
        switch a.Kind {
        case model.ActorKindHuman:
                return a.ID, true
        case model.ActorKindAgent:
                if a.UserID == nil || *a.UserID == "" {
                        return "", false
                }
                return *a.UserID, true
        default:
                return "", false
        }
}

func (db *DB) FindProject(id string) (*model.Project, bool) {
        for i := range db.Projects {
                if db.Projects[i].ID == id {
                        return &db.Projects[i], true
                }
        }
        return nil, false
}

func (db *DB) FindOutline(id string) (*model.Outline, bool) {
        for i := range db.Outlines {
                if db.Outlines[i].ID == id {
                        return &db.Outlines[i], true
                }
        }
        return nil, false
}

func (db *DB) FindItem(id string) (*model.Item, bool) {
        for i := range db.Items {
                if db.Items[i].ID == id {
                        return &db.Items[i], true
                }
        }
        return nil, false
}

func (db *DB) ensureIndexes() {
        if db == nil || db.idxBuilt {
                return
        }
        db.idxChildrenByParent = map[string][]model.Item{}
        db.idxCommentsByItem = map[string][]model.Comment{}
        db.idxWorklogByItem = map[string][]model.WorklogEntry{}

        for _, it := range db.Items {
                if it.Archived {
                        continue
                }
                if it.ParentID == nil {
                        continue
                }
                pid := strings.TrimSpace(*it.ParentID)
                if pid == "" {
                        continue
                }
                db.idxChildrenByParent[pid] = append(db.idxChildrenByParent[pid], it)
        }

        for _, c := range db.Comments {
                id := strings.TrimSpace(c.ItemID)
                if id == "" {
                        continue
                }
                db.idxCommentsByItem[id] = append(db.idxCommentsByItem[id], c)
        }
        for id := range db.idxCommentsByItem {
                comments := db.idxCommentsByItem[id]
                sort.Slice(comments, func(i, j int) bool { return comments[i].CreatedAt.After(comments[j].CreatedAt) })
                db.idxCommentsByItem[id] = comments
        }

        for _, w := range db.Worklog {
                id := strings.TrimSpace(w.ItemID)
                if id == "" {
                        continue
                }
                db.idxWorklogByItem[id] = append(db.idxWorklogByItem[id], w)
        }
        for id := range db.idxWorklogByItem {
                entries := db.idxWorklogByItem[id]
                sort.Slice(entries, func(i, j int) bool { return entries[i].CreatedAt.After(entries[j].CreatedAt) })
                db.idxWorklogByItem[id] = entries
        }

        db.idxBuilt = true
}

func (db *DB) ChildrenOf(parentItemID string) []model.Item {
        if db == nil {
                return nil
        }
        db.ensureIndexes()
        return db.idxChildrenByParent[strings.TrimSpace(parentItemID)]
}

func (db *DB) CommentsForItem(itemID string) []model.Comment {
        if db == nil {
                return nil
        }
        db.ensureIndexes()
        return db.idxCommentsByItem[strings.TrimSpace(itemID)]
}

func (db *DB) WorklogForItem(itemID string) []model.WorklogEntry {
        if db == nil {
                return nil
        }
        db.ensureIndexes()
        return db.idxWorklogByItem[strings.TrimSpace(itemID)]
}

func NormalizeActorKind(s string) (model.ActorKind, error) {
        switch strings.ToLower(strings.TrimSpace(s)) {
        case "human":
                return model.ActorKindHuman, nil
        case "agent":
                return model.ActorKindAgent, nil
        default:
                return "", fmt.Errorf("invalid actor kind: %q (expected human|agent)", s)
        }
}

func ParseStatusID(s string) (string, error) {
        switch strings.ToUpper(strings.TrimSpace(s)) {
        case "TODO", "todo":
                return "todo", nil
        case "DOING", "doing":
                return "doing", nil
        case "DONE", "done":
                return "done", nil
        case "NONE", "none":
                return "", nil
        default:
                // For outline-defined statuses, we allow any non-empty id.
                s = strings.TrimSpace(s)
                if s == "" {
                        return "", fmt.Errorf("invalid status: empty")
                }
                return s, nil
        }
}

func ReadEvents(dir string, limit int) ([]model.Event, error) {
        st := Store{Dir: dir}
        // SQLite-only by default. Keep legacy JSONL reader for back-compat/forensics if explicitly requested.
        if st.eventLogBackend() != EventLogBackendJSONL {
                return st.readEventsSQLite(context.Background(), limit)
        }
        path := filepath.Join(dir, eventsFileName)
        f, err := os.Open(path)
        if err != nil {
                if errors.Is(err, os.ErrNotExist) {
                        return []model.Event{}, nil
                }
                return nil, err
        }
        defer f.Close()

        var out []model.Event
        sc := bufio.NewScanner(f)
        for sc.Scan() {
                var ev model.Event
                if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
                        return nil, err
                }
                out = append(out, ev)
                if limit > 0 && len(out) >= limit {
                        break
                }
        }
        if err := sc.Err(); err != nil {
                return nil, err
        }
        return out, nil
}

// ReadEventsTail reads the last N events from the append-only events log.
//
// The returned slice is in chronological order (oldest-first within the returned window).
// If limit <= 0, this behaves like ReadEvents(dir, 0) and returns all events.
func ReadEventsTail(dir string, limit int) ([]model.Event, error) {
        st := Store{Dir: dir}
        if st.eventLogBackend() != EventLogBackendJSONL {
                // Simple implementation: read all and take tail. We can optimize later.
                // This preserves existing behavior while we transition the store.
                evs, err := st.readEventsSQLite(context.Background(), 0)
                if err != nil {
                        return nil, err
                }
                if limit <= 0 || len(evs) <= limit {
                        return evs, nil
                }
                return evs[len(evs)-limit:], nil
        }
        if limit <= 0 {
                return ReadEvents(dir, 0)
        }

        path := filepath.Join(dir, eventsFileName)
        f, err := os.Open(path)
        if err != nil {
                if errors.Is(err, os.ErrNotExist) {
                        return []model.Event{}, nil
                }
                return nil, err
        }
        defer f.Close()

        // Ring buffer for the last `limit` events.
        ring := make([]model.Event, limit)
        start := 0
        size := 0

        sc := bufio.NewScanner(f)
        for sc.Scan() {
                var ev model.Event
                if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
                        return nil, err
                }
                if size < limit {
                        ring[size] = ev
                        size++
                } else {
                        ring[start] = ev
                        start = (start + 1) % limit
                }
        }
        if err := sc.Err(); err != nil {
                return nil, err
        }

        if size == 0 {
                return []model.Event{}, nil
        }
        if size < limit {
                return ring[:size], nil
        }

        out := make([]model.Event, 0, limit)
        out = append(out, ring[start:]...)
        out = append(out, ring[:start]...)
        return out, nil
}

// ReadEventsForEntity returns events matching entityID from the append-only events log.
//
// The returned slice is in chronological order (oldest-first within the returned window).
// If limit <= 0, all matching events are returned.
func ReadEventsForEntity(dir, entityID string, limit int) ([]model.Event, error) {
        entityID = strings.TrimSpace(entityID)
        if entityID == "" {
                return []model.Event{}, nil
        }
        st := Store{Dir: dir}
        if st.eventLogBackend() != EventLogBackendJSONL {
                return st.readEventsForEntitySQLite(context.Background(), entityID, limit)
        }

        path := filepath.Join(dir, eventsFileName)
        f, err := os.Open(path)
        if err != nil {
                if errors.Is(err, os.ErrNotExist) {
                        return []model.Event{}, nil
                }
                return nil, err
        }
        defer f.Close()

        // If limit <= 0, return all matches.
        if limit <= 0 {
                var out []model.Event
                sc := bufio.NewScanner(f)
                for sc.Scan() {
                        var ev model.Event
                        if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
                                return nil, err
                        }
                        if strings.TrimSpace(ev.EntityID) != entityID {
                                continue
                        }
                        out = append(out, ev)
                }
                if err := sc.Err(); err != nil {
                        return nil, err
                }
                return out, nil
        }

        // Ring buffer for the last `limit` matching events.
        ring := make([]model.Event, limit)
        start := 0
        size := 0

        sc := bufio.NewScanner(f)
        for sc.Scan() {
                var ev model.Event
                if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
                        return nil, err
                }
                if strings.TrimSpace(ev.EntityID) != entityID {
                        continue
                }
                if size < limit {
                        ring[size] = ev
                        size++
                } else {
                        ring[start] = ev
                        start = (start + 1) % limit
                }
        }
        if err := sc.Err(); err != nil {
                return nil, err
        }

        if size == 0 {
                return []model.Event{}, nil
        }
        if size < limit {
                return ring[:size], nil
        }

        out := make([]model.Event, 0, limit)
        out = append(out, ring[start:]...)
        out = append(out, ring[:start]...)
        return out, nil
}
