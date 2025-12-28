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

func TestNewList_CursorNavigation_AlsoSupportsCtrlNP(t *testing.T) {
        l := newList("t", "h", nil)

        has := func(keys []string, want string) bool {
                for _, k := range keys {
                        if k == want {
                                return true
                        }
                }
                return false
        }

        if !has(l.KeyMap.CursorDown.Keys(), "ctrl+n") {
                t.Fatalf("expected CursorDown to include ctrl+n; got %v", l.KeyMap.CursorDown.Keys())
        }
        if !has(l.KeyMap.CursorUp.Keys(), "ctrl+p") {
                t.Fatalf("expected CursorUp to include ctrl+p; got %v", l.KeyMap.CursorUp.Keys())
        }

        // Ensure we didn't accidentally drop Bubble's defaults.
        if !has(l.KeyMap.CursorDown.Keys(), "down") || !has(l.KeyMap.CursorDown.Keys(), "j") {
                t.Fatalf("expected CursorDown to include down and j; got %v", l.KeyMap.CursorDown.Keys())
        }
        if !has(l.KeyMap.CursorUp.Keys(), "up") || !has(l.KeyMap.CursorUp.Keys(), "k") {
                t.Fatalf("expected CursorUp to include up and k; got %v", l.KeyMap.CursorUp.Keys())
        }
}
