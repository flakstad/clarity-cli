package store

import (
        "os"
        "path/filepath"
        "strings"
        "testing"
)

func TestEnsureGitBackedV1Layout_CreatesFiles(t *testing.T) {
        dir := t.TempDir()

        res, err := EnsureGitBackedV1Layout(dir)
        if err != nil {
                t.Fatalf("EnsureGitBackedV1Layout: %v", err)
        }
        if res.WorkspaceID == "" {
                t.Fatalf("expected workspace id")
        }
        if res.ReplicaID == "" {
                t.Fatalf("expected replica id")
        }
        if !res.WorkspaceMetaCreated {
                t.Fatalf("expected workspace meta to be created")
        }
        if !res.DeviceCreated {
                t.Fatalf("expected device file to be created")
        }
        if !res.ShardCreated {
                t.Fatalf("expected shard file to be created")
        }
        if !res.GitignoreUpdated {
                t.Fatalf("expected gitignore to be updated")
        }

        // workspace meta committed file
        if _, err := os.Stat(filepath.Join(dir, "meta", "workspace.json")); err != nil {
                t.Fatalf("stat meta/workspace.json: %v", err)
        }
        // local device file
        if _, err := os.Stat(filepath.Join(dir, ".clarity", "device.json")); err != nil {
                t.Fatalf("stat .clarity/device.json: %v", err)
        }
        // shard
        if _, err := os.Stat(res.ShardPath); err != nil {
                t.Fatalf("stat shard: %v", err)
        }
        if b, err := os.ReadFile(filepath.Join(dir, ".gitignore")); err != nil {
                t.Fatalf("read .gitignore: %v", err)
        } else if !strings.Contains(string(b), ".clarity/") {
                t.Fatalf("expected .gitignore to include .clarity/")
        }

        // Idempotent.
        res2, err := EnsureGitBackedV1Layout(dir)
        if err != nil {
                t.Fatalf("EnsureGitBackedV1Layout (2): %v", err)
        }
        if res2.WorkspaceID != res.WorkspaceID {
                t.Fatalf("workspace id changed: %q -> %q", res.WorkspaceID, res2.WorkspaceID)
        }
        if res2.ReplicaID != res.ReplicaID {
                t.Fatalf("replica id changed: %q -> %q", res.ReplicaID, res2.ReplicaID)
        }
        if res2.GitignoreUpdated {
                t.Fatalf("expected gitignore to not be updated on second run")
        }
}
