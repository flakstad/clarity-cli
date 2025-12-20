package tui

import "testing"

func TestNewList_DoesNotQuitOnEsc(t *testing.T) {
        l := newList("t", "h", nil)

        for _, k := range l.KeyMap.Quit.Keys() {
                if k == "esc" {
                        t.Fatalf("expected list quit binding not to include esc; got %v", l.KeyMap.Quit.Keys())
                }
        }

        foundQ := false
        for _, k := range l.KeyMap.Quit.Keys() {
                if k == "q" {
                        foundQ = true
                        break
                }
        }
        if !foundQ {
                t.Fatalf("expected list quit binding to include q; got %v", l.KeyMap.Quit.Keys())
        }
}
