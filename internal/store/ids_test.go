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
        if got, want := len(suffix), 6; got != want {
                t.Fatalf("expected item id suffix len %d, got %d (%q)", want, got, suffix)
        }
}

func TestNewRandomID_OtherIDsStayStableLength(t *testing.T) {
        id, err := newRandomID("proj")
        if err != nil {
                t.Fatalf("newRandomID: %v", err)
        }
        if !strings.HasPrefix(id, "proj-") {
                t.Fatalf("expected proj prefix, got %q", id)
        }
        suffix := strings.TrimPrefix(id, "proj-")
        if got, want := len(suffix), 8; got != want {
                t.Fatalf("expected proj id suffix len %d, got %d (%q)", want, got, suffix)
        }
}
