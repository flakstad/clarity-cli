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

func TestListWorkspaceEntries_RegistryWinsOverLegacy(t *testing.T) {
        cfgDir := t.TempDir()
        t.Setenv("CLARITY_CONFIG_DIR", cfgDir)

        legacyRoot := filepath.Join(cfgDir, "workspaces")
        if err := os.MkdirAll(filepath.Join(legacyRoot, "team"), 0o755); err != nil {
                t.Fatalf("mkdir legacy: %v", err)
        }

        wsPath := filepath.Join(t.TempDir(), "team-ws")
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

        ents, err := ListWorkspaceEntries()
        if err != nil {
                t.Fatalf("ListWorkspaceEntries: %v", err)
        }
        if len(ents) != 1 {
                t.Fatalf("expected 1 entry, got %#v", ents)
        }
        if ents[0].Name != "team" {
                t.Fatalf("expected name team, got %#v", ents[0])
        }
        if ents[0].Legacy {
                t.Fatalf("expected registry to win, got legacy entry: %#v", ents[0])
        }
        if ents[0].Ref.Path != wsPath {
                t.Fatalf("expected path %q, got %#v", wsPath, ents[0].Ref.Path)
        }
}

func TestListWorkspaceEntries_MarksArchived(t *testing.T) {
        cfgDir := t.TempDir()
        t.Setenv("CLARITY_CONFIG_DIR", cfgDir)

        legacyRoot := filepath.Join(cfgDir, "workspaces")
        if err := os.MkdirAll(filepath.Join(legacyRoot, "legacy1"), 0o755); err != nil {
                t.Fatalf("mkdir legacy: %v", err)
        }

        cfg := &GlobalConfig{
                Workspaces: map[string]WorkspaceRef{
                        "team": {Path: "/tmp/team", Kind: "git"},
                },
                ArchivedWorkspaces: map[string]bool{
                        "team":    true,
                        "legacy1": true,
                },
        }
        if err := SaveConfig(cfg); err != nil {
                t.Fatalf("SaveConfig: %v", err)
        }

        ents, err := ListWorkspaceEntries()
        if err != nil {
                t.Fatalf("ListWorkspaceEntries: %v", err)
        }

        archived := map[string]bool{}
        for _, e := range ents {
                archived[e.Name] = e.Archived
        }
        if !archived["team"] {
                t.Fatalf("expected team to be archived, got %#v", archived)
        }
        if !archived["legacy1"] {
                t.Fatalf("expected legacy1 to be archived, got %#v", archived)
        }
}
