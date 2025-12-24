package store

import (
        "encoding/json"
        "errors"
        "os"
        "path/filepath"
        "strings"
)

const tuiStateFileName = "tui_state.json"

// TUIState stores small, user-facing UI state for restoring the last screen on relaunch.
//
// This file lives inside the workspace/store directory so state is naturally scoped per workspace.
// It is intentionally "best effort": callers should tolerate missing/invalid data.
type TUIState struct {
        Version int `json:"version"`

        // View is one of: projects|outlines|outline|item|agenda
        View string `json:"view,omitempty"`

        SelectedProjectID string `json:"selectedProjectId,omitempty"`
        SelectedOutlineID string `json:"selectedOutlineId,omitempty"`

        // OpenItemID is used when View == "item".
        OpenItemID string `json:"openItemId,omitempty"`

        // ReturnView is used when View == "item" (for backspace/esc).
        // One of: projects|outlines|outline|agenda
        ReturnView string `json:"returnView,omitempty"`

        // AgendaReturnView is used when View == "agenda" (for backspace/esc).
        // One of: projects|outlines|outline
        AgendaReturnView string `json:"agendaReturnView,omitempty"`

        // Pane is one of: outline|detail
        Pane string `json:"pane,omitempty"`

        ShowPreview bool `json:"showPreview,omitempty"`

        // Per-outline display mode.
        // Values: list|list+preview|document|columns
        OutlineViewMode map[string]string `json:"outlineViewMode,omitempty"`

        // RecentItemIDs stores most-recently-visited item ids (full item view only), newest first.
        // This powers the Go to panel "Recent items" shortcuts.
        RecentItemIDs []string `json:"recentItemIds,omitempty"`
}

func (s Store) tuiStatePath() string {
        return filepath.Join(s.Dir, tuiStateFileName)
}

func (s Store) LoadTUIState() (*TUIState, error) {
        if strings.TrimSpace(s.Dir) == "" {
                return &TUIState{Version: 1}, nil
        }
        if err := s.Ensure(); err != nil {
                return nil, err
        }
        b, err := os.ReadFile(s.tuiStatePath())
        if err != nil {
                if errors.Is(err, os.ErrNotExist) {
                        return &TUIState{Version: 1}, nil
                }
                return nil, err
        }
        var st TUIState
        if err := json.Unmarshal(b, &st); err != nil {
                // Best-effort; if corrupted, treat as missing.
                return &TUIState{Version: 1}, nil
        }
        if st.Version == 0 {
                st.Version = 1
        }
        return &st, nil
}

func (s Store) SaveTUIState(st *TUIState) error {
        if st == nil {
                return nil
        }
        if strings.TrimSpace(s.Dir) == "" {
                return nil
        }
        if err := s.Ensure(); err != nil {
                return err
        }
        if st.Version == 0 {
                st.Version = 1
        }
        b, err := json.MarshalIndent(st, "", "  ")
        if err != nil {
                return err
        }
        path := s.tuiStatePath()
        tmp := path + ".tmp"
        if err := os.WriteFile(tmp, b, 0o644); err != nil {
                return err
        }
        return os.Rename(tmp, path)
}
