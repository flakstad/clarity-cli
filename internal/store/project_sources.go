package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type ProjectSourceKind string

const (
	ProjectSourceKindWorkspace ProjectSourceKind = "workspace"
	ProjectSourceKindExternal  ProjectSourceKind = "external"
)

type ProjectSource struct {
	// Kind is "workspace" (default) or "external" (future).
	Kind ProjectSourceKind `json:"kind"`

	// Name is an optional display label.
	Name string `json:"name,omitempty"`

	// Workspace is the workspace name (for "workspace" sources).
	Workspace string `json:"workspace,omitempty"`

	// Path is a local filesystem path (for "external" sources).
	Path string `json:"path,omitempty"`
}

type ProjectSourcesFile struct {
	Version int             `json:"version"`
	Sources []ProjectSource `json:"sources,omitempty"`
}

func (s Store) projectSourcesPath() string {
	return filepath.Join(s.workspaceRoot(), "meta", "project-sources.json")
}

// LoadProjectSources reads meta/project-sources.json if present.
//
// This is a v1 placeholder for a future "external project sources" feature. For v1, Clarity uses
// workspace-level boundaries: a separate Git repo/workspace per access scope.
func (s Store) LoadProjectSources() (ProjectSourcesFile, bool, error) {
	path := s.projectSourcesPath()
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ProjectSourcesFile{}, false, nil
		}
		return ProjectSourcesFile{}, false, err
	}
	if len(b) == 0 {
		return ProjectSourcesFile{}, false, errors.New("meta/project-sources.json: empty file")
	}
	var f ProjectSourcesFile
	if err := json.Unmarshal(b, &f); err != nil {
		return ProjectSourcesFile{}, false, err
	}
	if f.Version == 0 {
		f.Version = 1
	}
	if f.Version != 1 {
		return ProjectSourcesFile{}, false, errors.New("meta/project-sources.json: unsupported version")
	}
	for i := range f.Sources {
		f.Sources[i].Name = strings.TrimSpace(f.Sources[i].Name)
		f.Sources[i].Workspace = strings.TrimSpace(f.Sources[i].Workspace)
		f.Sources[i].Path = filepath.Clean(strings.TrimSpace(f.Sources[i].Path))
		if f.Sources[i].Kind == "" {
			f.Sources[i].Kind = ProjectSourceKindWorkspace
		}
	}
	return f, true, nil
}
