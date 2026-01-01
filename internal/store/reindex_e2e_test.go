package store

import (
        "io/fs"
        "os"
        "path/filepath"
        "testing"
)

func TestReindex_EndToEnd_FromTestdataWorkspace(t *testing.T) {
        fixture := filepath.Join("testdata", "reindex_ws")

        tmp := t.TempDir()
        copyDir(t, fixture, tmp)

        // Sanity: fixture doctor should have no errors.
        rep := DoctorEventsV1(tmp)
        if rep.HasErrors() {
                t.Fatalf("doctor has errors: %+v", rep.Issues)
        }

        res, err := ReplayEventsV1(tmp)
        if err != nil {
                t.Fatalf("ReplayEventsV1: %v", err)
        }
        if res.AppliedCount != 9 {
                t.Fatalf("expected applied=9, got %d (skipped=%d)", res.AppliedCount, res.SkippedCount)
        }

        s := Store{Dir: tmp}
        if err := s.Save(res.DB); err != nil {
                t.Fatalf("Save: %v", err)
        }

        loaded, err := s.Load()
        if err != nil {
                t.Fatalf("Load: %v", err)
        }
        if len(loaded.Actors) != 1 || len(loaded.Projects) != 1 || len(loaded.Outlines) != 1 {
                t.Fatalf("unexpected core counts: actors=%d projects=%d outlines=%d", len(loaded.Actors), len(loaded.Projects), len(loaded.Outlines))
        }
        if len(loaded.Items) != 2 || len(loaded.Comments) != 1 || len(loaded.Worklog) != 1 || len(loaded.Deps) != 1 {
                t.Fatalf("unexpected content counts: items=%d comments=%d worklog=%d deps=%d", len(loaded.Items), len(loaded.Comments), len(loaded.Worklog), len(loaded.Deps))
        }
}

func copyDir(t *testing.T, src string, dst string) {
        t.Helper()
        err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
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
                b, err := os.ReadFile(path)
                if err != nil {
                        return err
                }
                return os.WriteFile(target, b, 0o644)
        })
        if err != nil {
                t.Fatalf("copy fixture: %v", err)
        }
}
