package store

import (
        "os"
        "path/filepath"
        "testing"
)

func TestLoadUsers_NotFound(t *testing.T) {
        dir := t.TempDir()
        f, ok, err := LoadUsers(dir)
        if err != nil {
                t.Fatalf("LoadUsers: %v", err)
        }
        if ok {
                t.Fatalf("expected ok=false for missing file")
        }
        if len(f.Users) != 0 {
                t.Fatalf("expected empty users")
        }
}

func TestLoadUsers_NormalizesAndSorts(t *testing.T) {
        dir := t.TempDir()
        if err := os.MkdirAll(filepath.Join(dir, "meta"), 0o755); err != nil {
                t.Fatalf("mkdir meta: %v", err)
        }
        raw := []byte(`{
  "users": [
    {"email": "B@EXAMPLE.COM", "actorId": "act-b"},
    {"email": " a@example.com ", "actorId": "act-a"}
  ]
}`)
        if err := os.WriteFile(filepath.Join(dir, "meta", "users.json"), raw, 0o644); err != nil {
                t.Fatalf("write users.json: %v", err)
        }

        f, ok, err := LoadUsers(dir)
        if err != nil {
                t.Fatalf("LoadUsers: %v", err)
        }
        if !ok {
                t.Fatalf("expected ok=true")
        }
        if len(f.Users) != 2 {
                t.Fatalf("expected 2 users, got %d", len(f.Users))
        }
        if f.Users[0].Email != "a@example.com" || f.Users[0].ActorID != "act-a" {
                t.Fatalf("unexpected first user: %+v", f.Users[0])
        }
        if f.Users[1].Email != "b@example.com" || f.Users[1].ActorID != "act-b" {
                t.Fatalf("unexpected second user: %+v", f.Users[1])
        }

        if got, ok := f.ActorIDForEmail("B@EXAMPLE.COM"); !ok || got != "act-b" {
                t.Fatalf("ActorIDForEmail: got (%v,%v)", got, ok)
        }
}
