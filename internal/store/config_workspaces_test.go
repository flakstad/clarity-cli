package store

import (
        "os"
        "path/filepath"
        "testing"
)

func TestWorkspaceDir_UsesRegistryWhenPresent(t *testing.T) {
        cfgDir := t.TempDir()
        t.Setenv("CLARITY_CONFIG_DIR", cfgDir)

        wsPath := filepath.Join(t.TempDir(), "my-workspace")
        if err := os.MkdirAll(wsPath, 0o755); err != nil {
                t.Fatalf("mkdir ws: %v", err)
        }

        cfg := &GlobalConfig{
                Workspaces: map[string]WorkspaceRef{
                        "team": {Path: wsPath, Kind: "git"},
                },
        }
        if err := SaveConfig(cfg); err != nil {
                t.Fatalf("SaveConfig: %v", err)
        }

        dir, err := WorkspaceDir("team")
        if err != nil {
                t.Fatalf("WorkspaceDir: %v", err)
        }
        if dir != wsPath {
                t.Fatalf("expected %q, got %q", wsPath, dir)
        }
}

func TestListWorkspaces_IncludesRegistryAndLegacyDirs(t *testing.T) {
        cfgDir := t.TempDir()
        t.Setenv("CLARITY_CONFIG_DIR", cfgDir)

        // Legacy dir workspace
        legacyRoot := filepath.Join(cfgDir, "workspaces")
        if err := os.MkdirAll(filepath.Join(legacyRoot, "default"), 0o755); err != nil {
                t.Fatalf("mkdir legacy: %v", err)
        }

        cfg := &GlobalConfig{
                Workspaces: map[string]WorkspaceRef{
                        "team": {Path: "/tmp/team", Kind: "git"},
                },
        }
        if err := SaveConfig(cfg); err != nil {
                t.Fatalf("SaveConfig: %v", err)
        }

        ws, err := ListWorkspaces()
        if err != nil {
                t.Fatalf("ListWorkspaces: %v", err)
        }

        if len(ws) != 2 || ws[0] != "default" || ws[1] != "team" {
                t.Fatalf("unexpected workspaces: %#v", ws)
        }
}
