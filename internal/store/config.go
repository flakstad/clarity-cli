package store

import (
        "encoding/json"
        "errors"
        "os"
        "path/filepath"
        "sort"
        "strings"
)

type GlobalConfig struct {
        CurrentWorkspace string `json:"currentWorkspace,omitempty"`
}

func ConfigDir() (string, error) {
        home, err := os.UserHomeDir()
        if err != nil {
                return "", err
        }
        return filepath.Join(home, ".clarity"), nil
}

func ConfigPath() (string, error) {
        dir, err := ConfigDir()
        if err != nil {
                return "", err
        }
        return filepath.Join(dir, "config.json"), nil
}

func LoadConfig() (*GlobalConfig, error) {
        path, err := ConfigPath()
        if err != nil {
                return nil, err
        }
        b, err := os.ReadFile(path)
        if err != nil {
                if errors.Is(err, os.ErrNotExist) {
                        return &GlobalConfig{}, nil
                }
                return nil, err
        }
        var cfg GlobalConfig
        if err := json.Unmarshal(b, &cfg); err != nil {
                return nil, err
        }
        return &cfg, nil
}

func SaveConfig(cfg *GlobalConfig) error {
        path, err := ConfigPath()
        if err != nil {
                return err
        }
        dir := filepath.Dir(path)
        if err := os.MkdirAll(dir, 0o755); err != nil {
                return err
        }
        b, err := json.MarshalIndent(cfg, "", "  ")
        if err != nil {
                return err
        }
        tmp := path + ".tmp"
        if err := os.WriteFile(tmp, b, 0o644); err != nil {
                return err
        }
        return os.Rename(tmp, path)
}

func NormalizeWorkspaceName(name string) (string, error) {
        name = strings.TrimSpace(name)
        if name == "" {
                return "", errors.New("workspace name is empty")
        }
        // Keep it simple for now; treat it as a directory name.
        return name, nil
}

func ListWorkspaces() ([]string, error) {
        dir, err := ConfigDir()
        if err != nil {
                return nil, err
        }
        wsRoot := filepath.Join(dir, "workspaces")
        ents, err := os.ReadDir(wsRoot)
        if err != nil {
                if errors.Is(err, os.ErrNotExist) {
                        return []string{}, nil
                }
                return nil, err
        }
        var out []string
        for _, e := range ents {
                if e.IsDir() {
                        out = append(out, e.Name())
                }
        }
        sort.Strings(out)
        return out, nil
}
