package store

import (
        "strings"
        "testing"
)

func TestNewRandomID_ItemIDsAreShort(t *testing.T) {
        id, err := newRandomID("item")
        if err != nil {
                t.Fatalf("newRandomID: %v", err)
        }
        if !strings.HasPrefix(id, "item-") {
                t.Fatalf("expected item prefix, got %q", id)
        }
        suffix := strings.TrimPrefix(id, "item-")
        if got, want := len(suffix), 3; got != want {
                t.Fatalf("expected item id suffix len %d, got %d (%q)", want, got, suffix)
        }
}

func TestNewRandomID_UserFacingIDsAreShort(t *testing.T) {
        id, err := newRandomID("proj")
        if err != nil {
                t.Fatalf("newRandomID: %v", err)
        }
        if !strings.HasPrefix(id, "proj-") {
                t.Fatalf("expected proj prefix, got %q", id)
        }
        suffix := strings.TrimPrefix(id, "proj-")
        if got, want := len(suffix), 3; got != want {
                t.Fatalf("expected proj id suffix len %d, got %d (%q)", want, got, suffix)
        }

        id, err = newRandomID("act")
        if err != nil {
                t.Fatalf("newRandomID: %v", err)
        }
        if !strings.HasPrefix(id, "act-") {
                t.Fatalf("expected act prefix, got %q", id)
        }
        suffix = strings.TrimPrefix(id, "act-")
        if got, want := len(suffix), 3; got != want {
                t.Fatalf("expected act id suffix len %d, got %d (%q)", want, got, suffix)
        }

        id, err = newRandomID("out")
        if err != nil {
                t.Fatalf("newRandomID: %v", err)
        }
        if !strings.HasPrefix(id, "out-") {
                t.Fatalf("expected out prefix, got %q", id)
        }
        suffix = strings.TrimPrefix(id, "out-")
        if got, want := len(suffix), 3; got != want {
                t.Fatalf("expected out id suffix len %d, got %d (%q)", want, got, suffix)
        }
}

func TestNewRandomID_NonUserFacingIDsStayLonger(t *testing.T) {
        id, err := newRandomID("dep")
        if err != nil {
                t.Fatalf("newRandomID: %v", err)
        }
        if !strings.HasPrefix(id, "dep-") {
                t.Fatalf("expected dep prefix, got %q", id)
        }
        suffix := strings.TrimPrefix(id, "dep-")
        if got, want := len(suffix), 8; got != want {
                t.Fatalf("expected dep id suffix len %d, got %d (%q)", want, got, suffix)
        }
}
