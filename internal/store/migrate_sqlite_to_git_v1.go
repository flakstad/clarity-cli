package store

import (
        "context"
        "database/sql"
        "encoding/json"
        "errors"
        "fmt"
        "io"
        "os"
        "path/filepath"
        "strings"
        "time"
)

type MigrateSQLiteToGitBackedV1Result struct {
        FromDir string `json:"fromDir"`
        ToDir   string `json:"toDir"`

        WorkspaceID string `json:"workspaceId"`
        ReplicaID   string `json:"replicaId"`

        EventsWritten int `json:"eventsWritten"`

        WorkspaceMetaPath string `json:"workspaceMetaPath"`
        DevicePath        string `json:"devicePath"`
        ShardPath         string `json:"shardPath"`
        GitignorePath     string `json:"gitignorePath"`
}

// MigrateSQLiteToGitBackedV1 creates a Git-backed JSONL v1 workspace directory from a legacy SQLite workspace.
//
// It intentionally migrates the *event log* (SQLite events table) rather than attempting to snapshot derived state.
func MigrateSQLiteToGitBackedV1(ctx context.Context, fromDir, toDir string) (MigrateSQLiteToGitBackedV1Result, error) {
        fromDir = filepath.Clean(strings.TrimSpace(fromDir))
        toDir = filepath.Clean(strings.TrimSpace(toDir))
        if fromDir == "" || toDir == "" {
                return MigrateSQLiteToGitBackedV1Result{}, errors.New("missing from/to dir")
        }

        if st, err := os.Stat(fromDir); err != nil {
                return MigrateSQLiteToGitBackedV1Result{}, err
        } else if !st.IsDir() {
                return MigrateSQLiteToGitBackedV1Result{}, fmt.Errorf("from is not a directory: %s", fromDir)
        }

        if err := ensureDirEmptyOrNew(toDir); err != nil {
                return MigrateSQLiteToGitBackedV1Result{}, err
        }

        src := Store{Dir: fromDir}
        db, err := src.openSQLite(ctx)
        if err != nil {
                return MigrateSQLiteToGitBackedV1Result{}, err
        }
        defer db.Close()

        wsID, repID, err := readSQLiteWorkspaceReplica(ctx, db)
        if err != nil {
                return MigrateSQLiteToGitBackedV1Result{}, err
        }

        evs, err := src.ReadEventsV1(ctx, 0)
        if err != nil {
                return MigrateSQLiteToGitBackedV1Result{}, err
        }

        dst := Store{Dir: toDir}
        if err := os.MkdirAll(filepath.Join(dst.workspaceRoot(), "meta"), 0o755); err != nil {
                return MigrateSQLiteToGitBackedV1Result{}, err
        }
        if err := os.MkdirAll(dst.eventsDir(), 0o755); err != nil {
                return MigrateSQLiteToGitBackedV1Result{}, err
        }

        metaPath := dst.workspaceMetaPath()
        meta := WorkspaceMetaFile{WorkspaceID: wsID, CreatedAt: time.Now().UTC()}
        if err := writeJSONFile(metaPath, meta, 0o644); err != nil {
                return MigrateSQLiteToGitBackedV1Result{}, err
        }

        devicePath := dst.devicePath()
        device := DeviceFile{
                DeviceID:   mustUUIDv4(),
                ReplicaID:  repID,
                CreatedAt:  time.Now().UTC(),
                ModifiedAt: time.Now().UTC(),
        }
        if err := os.MkdirAll(filepath.Dir(devicePath), 0o755); err != nil {
                return MigrateSQLiteToGitBackedV1Result{}, err
        }
        if err := writeJSONFile(devicePath, device, 0o600); err != nil {
                return MigrateSQLiteToGitBackedV1Result{}, err
        }

        shardPath := dst.shardPath(repID)
        n, err := writeEventsJSONL(shardPath, evs)
        if err != nil {
                return MigrateSQLiteToGitBackedV1Result{}, err
        }

        // Best-effort copy of workspace resources (canonical) if present.
        _ = copyDirIfExists(filepath.Join(src.workspaceRoot(), "resources"), filepath.Join(dst.workspaceRoot(), "resources"))

        gitignorePath := filepath.Join(dst.workspaceRoot(), ".gitignore")
        if _, err := ensureGitignoreHasClarityIgnores(gitignorePath); err != nil {
                return MigrateSQLiteToGitBackedV1Result{}, err
        }

        return MigrateSQLiteToGitBackedV1Result{
                FromDir: fromDir,
                ToDir:   toDir,

                WorkspaceID: wsID,
                ReplicaID:   repID,

                EventsWritten: n,

                WorkspaceMetaPath: metaPath,
                DevicePath:        devicePath,
                ShardPath:         shardPath,
                GitignorePath:     gitignorePath,
        }, nil
}

func ensureDirEmptyOrNew(dir string) error {
        dir = filepath.Clean(strings.TrimSpace(dir))
        if dir == "" {
                return errors.New("empty dir")
        }
        if err := os.MkdirAll(dir, 0o755); err != nil {
                return err
        }
        ents, err := os.ReadDir(dir)
        if err != nil {
                return err
        }
        if len(ents) != 0 {
                return fmt.Errorf("to dir is not empty: %s", dir)
        }
        return nil
}

func readSQLiteWorkspaceReplica(ctx context.Context, db *sql.DB) (workspaceID, replicaID string, err error) {
        var ws string
        if err := db.QueryRowContext(ctx, `SELECT v FROM meta WHERE k = ?`, "workspace_id").Scan(&ws); err != nil {
                return "", "", err
        }
        var rep string
        if err := db.QueryRowContext(ctx, `SELECT v FROM meta WHERE k = ?`, "replica_id").Scan(&rep); err != nil {
                return "", "", err
        }
        ws = strings.TrimSpace(ws)
        rep = strings.TrimSpace(rep)
        if ws == "" || rep == "" {
                return "", "", errors.New("sqlite meta missing workspace_id/replica_id")
        }
        return ws, rep, nil
}

func writeEventsJSONL(path string, evs []EventV1) (int, error) {
        if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
                return 0, err
        }
        f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
        if err != nil {
                return 0, err
        }
        defer f.Close()

        n := 0
        enc := json.NewEncoder(f)
        for _, ev := range evs {
                if err := enc.Encode(ev); err != nil {
                        return n, err
                }
                n++
        }
        return n, nil
}

func writeJSONFile(path string, v any, perm os.FileMode) error {
        b, err := json.MarshalIndent(v, "", "  ")
        if err != nil {
                return err
        }
        b = append(b, '\n')
        tmp := path + ".tmp"
        if err := os.WriteFile(tmp, b, perm); err != nil {
                return err
        }
        return os.Rename(tmp, path)
}

func mustUUIDv4() string {
        id, err := newUUIDv4()
        if err != nil {
                // Extremely unlikely; crash is acceptable in this one-shot migration tool.
                panic(err)
        }
        return id
}

func copyDirIfExists(src, dst string) error {
        st, err := os.Stat(src)
        if err != nil {
                if errors.Is(err, os.ErrNotExist) {
                        return nil
                }
                return err
        }
        if !st.IsDir() {
                return nil
        }
        return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
                if err != nil {
                        return err
                }
                rel, err := filepath.Rel(src, path)
                if err != nil {
                        return err
                }
                target := filepath.Join(dst, rel)
                if d.IsDir() {
                        return os.MkdirAll(target, 0o755)
                }
                in, err := os.Open(path)
                if err != nil {
                        return err
                }
                defer in.Close()
                if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
                        return err
                }
                out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
                if err != nil {
                        return err
                }
                defer out.Close()
                if _, err := io.Copy(out, in); err != nil {
                        return err
                }
                return nil
        })
}
