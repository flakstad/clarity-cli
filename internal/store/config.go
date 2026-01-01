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

        // Workspaces is an optional registry of named workspace roots.
        // When set, these entries take precedence over ~/.clarity/workspaces/<name>.
        Workspaces map[string]WorkspaceRef `json:"workspaces,omitempty"`

        // CaptureTemplates define user-configured quick-capture targets and key sequences.
        // Stored globally (not per-workspace) to allow capturing across workspaces.
        CaptureTemplates []CaptureTemplate `json:"captureTemplates,omitempty"`

        // DeviceID is a stable per-machine identifier. It is used to derive per-workspace replica IDs
        // so that cloning a workspace directory to another machine yields a new replicaId automatically.
        DeviceID string `json:"deviceId,omitempty"`

        // Replicas maps workspaceId -> replicaId for this device.
        Replicas map[string]string `json:"replicas,omitempty"`
}

type WorkspaceRef struct {
        // Path is the workspace root directory.
        Path string `json:"path"`

        // Kind is an optional hint for the UI ("git", "local", ...).
        Kind string `json:"kind,omitempty"`

        // LastOpened is an optional timestamp for MRU selection in UIs.
        LastOpened string `json:"lastOpened,omitempty"`
}

func ConfigDir() (string, error) {
        // Test/advanced override (keeps unit tests from touching ~/.clarity).
        if v := strings.TrimSpace(os.Getenv("CLARITY_CONFIG_DIR")); v != "" {
                return v, nil
        }
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
        outSet := map[string]struct{}{}

        wsRoot := filepath.Join(dir, "workspaces")
        if ents, err := os.ReadDir(wsRoot); err == nil {
                for _, e := range ents {
                        if e.IsDir() {
                                outSet[e.Name()] = struct{}{}
                        }
                }
        } else if err != nil && !errors.Is(err, os.ErrNotExist) {
                return nil, err
        }

        cfg, err := LoadConfig()
        if err != nil {
                return nil, err
        }
        for name := range cfg.Workspaces {
                name = strings.TrimSpace(name)
                if name == "" {
                        continue
                }
                outSet[name] = struct{}{}
        }

        out := make([]string, 0, len(outSet))
        for name := range outSet {
                out = append(out, name)
        }
        sort.Strings(out)
        if out == nil {
                out = []string{}
        }
        return out, nil
}
