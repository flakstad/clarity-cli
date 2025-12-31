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

        WorkspaceMetaPath string `json:"workspaceMetaPath"`
        DevicePath        string `json:"devicePath"`
        ShardPath         string `json:"shardPath"`
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

                WorkspaceMetaPath: workspaceMetaPath,
                DevicePath:        devicePath,
                ShardPath:         shardPath,
        }, nil
}
