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

func TestNewList_GoToStartEnd_AlsoSupportsAngleBrackets(t *testing.T) {
        l := newList("t", "h", nil)

        has := func(keys []string, want string) bool {
                for _, k := range keys {
                        if k == want {
                                return true
                        }
                }
                return false
        }

        if !has(l.KeyMap.GoToStart.Keys(), "<") {
                t.Fatalf("expected GoToStart to include <; got %v", l.KeyMap.GoToStart.Keys())
        }
        if !has(l.KeyMap.GoToEnd.Keys(), ">") {
                t.Fatalf("expected GoToEnd to include >; got %v", l.KeyMap.GoToEnd.Keys())
        }

        // Ensure we didn't accidentally drop Bubble's defaults.
        if !has(l.KeyMap.GoToStart.Keys(), "home") || !has(l.KeyMap.GoToStart.Keys(), "g") {
                t.Fatalf("expected GoToStart to include home and g; got %v", l.KeyMap.GoToStart.Keys())
        }
        if !has(l.KeyMap.GoToEnd.Keys(), "end") || !has(l.KeyMap.GoToEnd.Keys(), "G") {
                t.Fatalf("expected GoToEnd to include end and G; got %v", l.KeyMap.GoToEnd.Keys())
        }
}
