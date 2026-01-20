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

	// ArchivedWorkspaces hides workspaces from default pickers/listings.
	// This can include both registered (git-backed) workspaces and legacy workspaces.
	ArchivedWorkspaces map[string]bool `json:"archivedWorkspaces,omitempty"`

	// CaptureTemplates define user-configured quick-capture targets and key sequences.
	// Stored globally (not per-workspace) to allow capturing across workspaces.
	CaptureTemplates []CaptureTemplate `json:"captureTemplates,omitempty"`

	// CaptureTemplateGroups optionally provide friendly labels for key prefixes in capture
	// template selection UIs (e.g. "w" => "Work"). When not set, the UI derives a label
	// from the first template name under that prefix.
	CaptureTemplateGroups []CaptureTemplateGroup `json:"captureTemplateGroups,omitempty"`

	// DeviceID is a stable per-machine identifier. It is used to derive per-workspace replica IDs
	// so that cloning a workspace directory to another machine yields a new replicaId automatically.
	DeviceID string `json:"deviceId,omitempty"`

	// Replicas maps workspaceId -> replicaId for this device.
	Replicas map[string]string `json:"replicas,omitempty"`

	// TUI holds optional user preferences for the interactive TUI.
	TUI *TUIConfig `json:"tui,omitempty"`
}

type TUIConfig struct {
	// Profile is the appearance profile id (e.g. "default", "neon").
	Profile string `json:"profile,omitempty"`
	// Glyphs selects the glyph set (e.g. "unicode", "ascii").
	Glyphs string `json:"glyphs,omitempty"`
	// Lists selects Projects/Outlines list style (e.g. "cards", "rows", "minimal").
	Lists string `json:"lists,omitempty"`

	// CustomProfile optionally defines a user-configured profile ("custom").
	CustomProfile *TUICustomProfile `json:"customProfile,omitempty"`
}

type TUICustomProfile struct {
	SelectedBg *AdaptiveColor `json:"selectedBg,omitempty"`
	SelectedFg *AdaptiveColor `json:"selectedFg,omitempty"`

	StatusNonEndFg *AdaptiveColor `json:"statusNonEndFg,omitempty"`
	StatusEndFg    *AdaptiveColor `json:"statusEndFg,omitempty"`

	MetaPriorityFg *AdaptiveColor `json:"metaPriorityFg,omitempty"`
	MetaOnHoldFg   *AdaptiveColor `json:"metaOnHoldFg,omitempty"`
	MetaDueFg      *AdaptiveColor `json:"metaDueFg,omitempty"`
	MetaScheduleFg *AdaptiveColor `json:"metaScheduleFg,omitempty"`
	MetaAssignFg   *AdaptiveColor `json:"metaAssignFg,omitempty"`
	MetaCommentFg  *AdaptiveColor `json:"metaCommentFg,omitempty"`
	MetaTagFg      *AdaptiveColor `json:"metaTagFg,omitempty"`

	ProgressFillBg  *AdaptiveColor `json:"progressFillBg,omitempty"`
	ProgressEmptyBg *AdaptiveColor `json:"progressEmptyBg,omitempty"`
	ProgressFillFg  *AdaptiveColor `json:"progressFillFg,omitempty"`
	ProgressEmptyFg *AdaptiveColor `json:"progressEmptyFg,omitempty"`
}

type AdaptiveColor struct {
	Light string `json:"light,omitempty"`
	Dark  string `json:"dark,omitempty"`
}

type WorkspaceRef struct {
	// Path is the workspace root directory.
	Path string `json:"path"`

	// Kind is an optional hint for the UI ("git", "local", ...).
	Kind string `json:"kind,omitempty"`

	// LastOpened is an optional timestamp for MRU selection in UIs.
	LastOpened string `json:"lastOpened,omitempty"`
}

type WorkspaceEntry struct {
	Name     string       `json:"name"`
	Ref      WorkspaceRef `json:"ref"`
	Legacy   bool         `json:"legacy"`
	Archived bool         `json:"archived,omitempty"`
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

func atomicWriteFile(dir, tmpPattern, path string, b []byte, perm os.FileMode) error {
	f, err := os.CreateTemp(dir, tmpPattern)
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	_ = os.Chmod(tmp, perm)
	return os.Rename(tmp, path)
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

	// Best-effort safety net: keep a copy of the previous config to make recovery from
	// accidental overwrites easier. Ignore errors to avoid blocking normal usage.
	if prev, err := os.ReadFile(path); err == nil && len(prev) > 0 {
		// Use a unique temp file name + atomic rename to avoid cross-process corruption.
		_ = atomicWriteFile(dir, "config.json.bak.*.tmp", path+".bak", prev, 0o644)
	}

	// Use a unique temp file name to avoid cross-process clobbering/corruption when multiple
	// Clarity processes write config concurrently (CLI + TUI + web).
	return atomicWriteFile(dir, "config.json.*.tmp", path, b, 0o600)
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

// ListWorkspaceEntries returns a stable list of known workspaces with optional path details.
//
// It unions legacy workspaces from `~/.clarity/workspaces/<name>` and the global workspace registry
// in `config.json`. If a name exists in both places, the registry entry wins (Legacy=false).
func ListWorkspaceEntries() ([]WorkspaceEntry, error) {
	dir, err := ConfigDir()
	if err != nil {
		return nil, err
	}

	wsRoot := filepath.Join(dir, "workspaces")
	legacy := map[string]WorkspaceEntry{}
	if ents, err := os.ReadDir(wsRoot); err == nil {
		for _, e := range ents {
			if !e.IsDir() {
				continue
			}
			name := strings.TrimSpace(e.Name())
			if name == "" {
				continue
			}
			legacy[name] = WorkspaceEntry{
				Name: name,
				Ref: WorkspaceRef{
					Path: filepath.Join(wsRoot, name),
					Kind: "legacy",
				},
				Legacy: true,
			}
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}

	outMap := map[string]WorkspaceEntry{}
	for name, ref := range cfg.Workspaces {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		ref.Path = filepath.Clean(strings.TrimSpace(ref.Path))
		outMap[name] = WorkspaceEntry{
			Name:     name,
			Ref:      ref,
			Legacy:   false,
			Archived: cfg.ArchivedWorkspaces != nil && cfg.ArchivedWorkspaces[name],
		}
	}
	for name, entry := range legacy {
		if _, ok := outMap[name]; ok {
			continue
		}
		entry.Archived = cfg.ArchivedWorkspaces != nil && cfg.ArchivedWorkspaces[name]
		outMap[name] = entry
	}

	names := make([]string, 0, len(outMap))
	for name := range outMap {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]WorkspaceEntry, 0, len(names))
	for _, name := range names {
		out = append(out, outMap[name])
	}
	if out == nil {
		out = []WorkspaceEntry{}
	}
	return out, nil
}
