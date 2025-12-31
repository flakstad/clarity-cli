package store

import (
        "encoding/json"
        "errors"
        "os"
        "path/filepath"
        "strings"
        "time"
)

type WorkspaceMetaFile struct {
        WorkspaceID string    `json:"workspaceId"`
        CreatedAt   time.Time `json:"createdAt"`
}

type DeviceFile struct {
        DeviceID   string    `json:"deviceId"`
        ReplicaID  string    `json:"replicaId"`
        CreatedAt  time.Time `json:"createdAt"`
        ModifiedAt time.Time `json:"modifiedAt"`
}

func (s Store) workspaceMetaPath() string {
        return filepath.Join(s.workspaceRoot(), "meta", "workspace.json")
}

func (s Store) clarityDir() string {
        dir := filepath.Clean(s.Dir)
        if filepath.Base(dir) == ".clarity" {
                return dir
        }
        return filepath.Join(s.workspaceRoot(), ".clarity")
}

func (s Store) devicePath() string {
        return filepath.Join(s.clarityDir(), "device.json")
}

func (s Store) loadOrInitWorkspaceMeta() (WorkspaceMetaFile, bool, error) {
        path := s.workspaceMetaPath()
        b, err := os.ReadFile(path)
        if err == nil && len(b) > 0 {
                var m WorkspaceMetaFile
                if err := json.Unmarshal(b, &m); err != nil {
                        return WorkspaceMetaFile{}, false, err
                }
                m.WorkspaceID = strings.TrimSpace(m.WorkspaceID)
                if m.WorkspaceID == "" {
                        return WorkspaceMetaFile{}, false, errors.New("meta/workspace.json: empty workspaceId")
                }
                return m, false, nil
        }
        if err != nil && !errors.Is(err, os.ErrNotExist) {
                return WorkspaceMetaFile{}, false, err
        }

        now := time.Now().UTC()
        id, err := newUUIDv4()
        if err != nil {
                return WorkspaceMetaFile{}, false, err
        }
        m := WorkspaceMetaFile{WorkspaceID: id, CreatedAt: now}
        if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
                return WorkspaceMetaFile{}, false, err
        }
        raw, _ := json.MarshalIndent(m, "", "  ")
        raw = append(raw, '\n')
        if err := os.WriteFile(path, raw, 0o644); err != nil {
                return WorkspaceMetaFile{}, false, err
        }
        return m, true, nil
}

func (s Store) loadOrInitDeviceFile() (DeviceFile, bool, error) {
        path := s.devicePath()
        b, err := os.ReadFile(path)
        if err == nil && len(b) > 0 {
                var d DeviceFile
                if err := json.Unmarshal(b, &d); err != nil {
                        return DeviceFile{}, false, err
                }
                d.DeviceID = strings.TrimSpace(d.DeviceID)
                d.ReplicaID = strings.TrimSpace(d.ReplicaID)
                if d.DeviceID == "" || d.ReplicaID == "" {
                        return DeviceFile{}, false, errors.New(".clarity/device.json: missing deviceId/replicaId")
                }
                return d, false, nil
        }
        if err != nil && !errors.Is(err, os.ErrNotExist) {
                return DeviceFile{}, false, err
        }

        now := time.Now().UTC()
        deviceID, err := newUUIDv4()
        if err != nil {
                return DeviceFile{}, false, err
        }
        replicaID, err := newUUIDv4()
        if err != nil {
                return DeviceFile{}, false, err
        }
        d := DeviceFile{
                DeviceID:   deviceID,
                ReplicaID:  replicaID,
                CreatedAt:  now,
                ModifiedAt: now,
        }
        if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
                return DeviceFile{}, false, err
        }
        raw, _ := json.MarshalIndent(d, "", "  ")
        raw = append(raw, '\n')
        if err := os.WriteFile(path, raw, 0o644); err != nil {
                return DeviceFile{}, false, err
        }
        return d, true, nil
}
