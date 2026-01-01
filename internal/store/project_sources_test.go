package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProjectSources_MissingFile(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	_, ok, err := s.LoadProjectSources()
	if err != nil {
		t.Fatalf("LoadProjectSources: %v", err)
	}
	if ok {
		t.Fatalf("expected ok=false for missing file")
	}
}

func TestLoadProjectSources_ReadsAndNormalizes(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	if err := os.MkdirAll(filepath.Join(dir, "meta"), 0o755); err != nil {
		t.Fatalf("mkdir meta: %v", err)
	}
	raw := `{"version":1,"sources":[{"kind":"external","name":"  Secret  ","path":"../secret-repo/"}]}`
	if err := os.WriteFile(filepath.Join(dir, "meta", "project-sources.json"), []byte(raw), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	f, ok, err := s.LoadProjectSources()
	if err != nil {
		t.Fatalf("LoadProjectSources: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if f.Version != 1 || len(f.Sources) != 1 {
		t.Fatalf("unexpected file: %#v", f)
	}
	if f.Sources[0].Name != "Secret" {
		t.Fatalf("expected trimmed name, got %#v", f.Sources[0].Name)
	}
	if f.Sources[0].Path == "" || filepath.Base(f.Sources[0].Path) != "secret-repo" {
		t.Fatalf("expected cleaned path, got %#v", f.Sources[0].Path)
	}
}
