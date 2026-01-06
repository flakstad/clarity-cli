package store

import (
        "errors"
        "os"
        "path/filepath"
        "strings"
)

type GitBackedV1InitResult struct {
        Dir string `json:"dir"`

        WorkspaceID string `json:"workspaceId"`
        ReplicaID   string `json:"replicaId"`

        WorkspaceMetaCreated bool `json:"workspaceMetaCreated"`
        DeviceCreated        bool `json:"deviceCreated"`
        ShardCreated         bool `json:"shardCreated"`
        GitignoreUpdated     bool `json:"gitignoreUpdated"`

        WorkspaceMetaPath string `json:"workspaceMetaPath"`
        DevicePath        string `json:"devicePath"`
        ShardPath         string `json:"shardPath"`
        GitignorePath     string `json:"gitignorePath"`
}

// EnsureGitBackedV1Layout bootstraps the Git-backed JSONL v1 workspace layout.
//
// It is safe to call multiple times.
func EnsureGitBackedV1Layout(dir string) (GitBackedV1InitResult, error) {
        dir = filepath.Clean(strings.TrimSpace(dir))
        if dir == "" {
                return GitBackedV1InitResult{}, errors.New("empty dir")
        }

        s := Store{Dir: dir}
        if err := s.Ensure(); err != nil {
                return GitBackedV1InitResult{}, err
        }

        wsMeta, wsCreated, err := s.loadOrInitWorkspaceMeta()
        if err != nil {
                return GitBackedV1InitResult{}, err
        }
        device, deviceCreated, err := s.loadOrInitDeviceFile()
        if err != nil {
                return GitBackedV1InitResult{}, err
        }

        workspaceMetaPath := s.workspaceMetaPath()
        devicePath := s.devicePath()

        if err := os.MkdirAll(s.eventsDir(), 0o755); err != nil {
                return GitBackedV1InitResult{}, err
        }

        gitignorePath := filepath.Join(dir, ".gitignore")
        gitignoreUpdated := false
        // Git is optional for the v1 workspace layout. Only touch .gitignore when we have
        // evidence the directory is (or is intended to be) a Git repo.
        _, gitDirErr := os.Stat(filepath.Join(dir, ".git"))
        _, gitignoreErr := os.Stat(gitignorePath)
        if gitDirErr == nil || gitignoreErr == nil {
                updated, err := EnsureGitignoreHasClarityIgnores(gitignorePath)
                if err != nil {
                        return GitBackedV1InitResult{}, err
                }
                gitignoreUpdated = updated
        }

        shardPath := s.shardPath(strings.TrimSpace(device.ReplicaID))
        shardCreated := false
        if _, err := os.Stat(shardPath); err != nil {
                if !errors.Is(err, os.ErrNotExist) {
                        return GitBackedV1InitResult{}, err
                }
                f, err := os.OpenFile(shardPath, os.O_CREATE|os.O_WRONLY, 0o644)
                if err != nil {
                        return GitBackedV1InitResult{}, err
                }
                _ = f.Close()
                shardCreated = true
        }

        return GitBackedV1InitResult{
                Dir: dir,

                WorkspaceID: strings.TrimSpace(wsMeta.WorkspaceID),
                ReplicaID:   strings.TrimSpace(device.ReplicaID),

                WorkspaceMetaCreated: wsCreated,
                DeviceCreated:        deviceCreated,
                ShardCreated:         shardCreated,
                GitignoreUpdated:     gitignoreUpdated,

                WorkspaceMetaPath: workspaceMetaPath,
                DevicePath:        devicePath,
                ShardPath:         shardPath,
                GitignorePath:     gitignorePath,
        }, nil
}

func EnsureGitignoreHasClarityIgnores(path string) (updated bool, err error) {
        path = filepath.Clean(strings.TrimSpace(path))
        if path == "" {
                return false, errors.New("empty gitignore path")
        }

        want := []string{
                "",
                "# Clarity (derived/local-only)",
                ".clarity/",
        }

        b, err := os.ReadFile(path)
        if err != nil && !errors.Is(err, os.ErrNotExist) {
                return false, err
        }
        existing := string(b)
        lines := strings.Split(existing, "\n")
        has := map[string]bool{}
        for _, ln := range lines {
                has[strings.TrimSpace(ln)] = true
        }

        var toAppend []string
        for _, ln := range want {
                if ln == "" {
                        continue
                }
                if !has[ln] {
                        toAppend = append(toAppend, ln)
                }
        }
        if len(toAppend) == 0 {
                return false, nil
        }

        out := existing
        if strings.TrimSpace(out) != "" && !strings.HasSuffix(out, "\n") {
                out += "\n"
        }
        // Keep the insertion compact and readable.
        out += strings.Join(append([]string{""}, toAppend...), "\n") + "\n"

        if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
                return false, err
        }
        return true, nil
}
