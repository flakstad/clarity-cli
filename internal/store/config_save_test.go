package store

import (
        "encoding/json"
        "fmt"
        "os"
        "path/filepath"
        "strings"
        "sync"
        "testing"
)

func TestSaveConfig_ConcurrentWriters_DoesNotCorruptConfig(t *testing.T) {
        cfgDir := t.TempDir()
        t.Setenv("CLARITY_CONFIG_DIR", cfgDir)

        seed := &GlobalConfig{
                CurrentWorkspace: "seed",
                Workspaces: map[string]WorkspaceRef{
                        "seed": {Path: "/tmp/seed", Kind: "git"},
                },
        }
        if err := SaveConfig(seed); err != nil {
                t.Fatalf("SaveConfig(seed): %v", err)
        }

        const n = 64
        errCh := make(chan error, n)

        var wg sync.WaitGroup
        for i := 0; i < n; i++ {
                wg.Add(1)
                go func(i int) {
                        defer wg.Done()

                        cfg, err := LoadConfig()
                        if err != nil {
                                errCh <- err
                                return
                        }
                        if cfg.Workspaces == nil {
                                cfg.Workspaces = map[string]WorkspaceRef{}
                        }
                        cfg.Workspaces[fmt.Sprintf("ws-%d", i)] = WorkspaceRef{
                                Path: fmt.Sprintf("/tmp/ws-%d", i),
                                Kind: "git",
                        }
                        cfg.DeviceID = fmt.Sprintf("dev-%d", i)

                        if err := SaveConfig(cfg); err != nil {
                                errCh <- err
                                return
                        }
                }(i)
        }

        wg.Wait()
        close(errCh)
        for err := range errCh {
                t.Errorf("concurrent SaveConfig: %v", err)
        }
        if t.Failed() {
                return
        }

        // Ensure the on-disk config is valid JSON.
        path, err := ConfigPath()
        if err != nil {
                t.Fatalf("ConfigPath: %v", err)
        }
        raw, err := os.ReadFile(path)
        if err != nil {
                t.Fatalf("read config.json: %v", err)
        }
        var cfg GlobalConfig
        if err := json.Unmarshal(raw, &cfg); err != nil {
                t.Fatalf("config.json corrupted/unparseable: %v\nraw:\n%s", err, string(raw))
        }

        // Ensure we didn't leave behind temp files.
        ents, err := os.ReadDir(cfgDir)
        if err != nil {
                t.Fatalf("ReadDir: %v", err)
        }
        for _, e := range ents {
                name := e.Name()
                if strings.HasPrefix(name, "config.json.") && strings.HasSuffix(name, ".tmp") {
                        t.Fatalf("leftover temp file: %s", name)
                }
        }

        // Best-effort backup should be parseable if present.
        if bak, err := os.ReadFile(path + ".bak"); err == nil && len(bak) > 0 {
                var bakCfg GlobalConfig
                if err := json.Unmarshal(bak, &bakCfg); err != nil {
                        t.Fatalf("config.json.bak corrupted/unparseable: %v\nraw:\n%s", err, string(bak))
                }
        }
}

func TestEnsureGitBackedV1Layout_DoesNotCreateGitignoreWithoutGit(t *testing.T) {
        dir := t.TempDir()
        res, err := EnsureGitBackedV1Layout(dir)
        if err != nil {
                t.Fatalf("EnsureGitBackedV1Layout: %v", err)
        }
        if res.GitignoreUpdated {
                t.Fatalf("expected GitignoreUpdated=false, got true")
        }
        if _, err := os.Stat(filepath.Join(dir, ".gitignore")); err == nil {
                t.Fatalf("expected .gitignore to not be created")
        }
}

func TestEnsureGitBackedV1Layout_CreatesGitignoreWhenGitPresent(t *testing.T) {
        dir := t.TempDir()
        if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
                t.Fatalf("mkdir .git: %v", err)
        }
        res, err := EnsureGitBackedV1Layout(dir)
        if err != nil {
                t.Fatalf("EnsureGitBackedV1Layout: %v", err)
        }
        if !res.GitignoreUpdated {
                t.Fatalf("expected GitignoreUpdated=true, got false")
        }
        if _, err := os.Stat(filepath.Join(dir, ".gitignore")); err != nil {
                t.Fatalf("expected .gitignore to exist: %v", err)
        }
}
