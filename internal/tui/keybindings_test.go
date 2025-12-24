package tui

import (
        "testing"

        tea "github.com/charmbracelet/bubbletea"
)

func TestMoveIndentFallbackKeys(t *testing.T) {
        t.Run("move down", func(t *testing.T) {
                if !isMoveDown(tea.KeyMsg{Type: tea.KeyShiftDown}) {
                        t.Fatalf("expected Shift+Down to be recognized for move down")
                }
                if !isMoveDown(tea.KeyMsg{Type: tea.KeyCtrlJ}) {
                        t.Fatalf("expected Ctrl+J to be recognized for move down")
                }
        })
        t.Run("move up", func(t *testing.T) {
                if !isMoveUp(tea.KeyMsg{Type: tea.KeyShiftUp}) {
                        t.Fatalf("expected Shift+Up to be recognized for move up")
                }
                if !isMoveUp(tea.KeyMsg{Type: tea.KeyCtrlK}) {
                        t.Fatalf("expected Ctrl+K to be recognized for move up")
                }
        })
        t.Run("indent", func(t *testing.T) {
                if !isIndent(tea.KeyMsg{Type: tea.KeyRight, Alt: true}) {
                        t.Fatalf("expected Alt+Right to be recognized for indent")
                }
                if !isIndent(tea.KeyMsg{Type: tea.KeyCtrlL}) {
                        t.Fatalf("expected Ctrl+L to be recognized for indent")
                }
        })
        t.Run("outdent", func(t *testing.T) {
                if !isOutdent(tea.KeyMsg{Type: tea.KeyLeft, Alt: true}) {
                        t.Fatalf("expected Alt+Left to be recognized for outdent")
                }
                if !isOutdent(tea.KeyMsg{Type: tea.KeyCtrlH}) {
                        t.Fatalf("expected Ctrl+H to be recognized for outdent")
                }
        })
}

type fakeStringer string

func (f fakeStringer) String() string { return string(f) }

func TestUnknownCSIParsing(t *testing.T) {
        // "?CSI[49 59 57 65]?" => "1;9A" => Alt+Up
        km, ok := keyMsgFromUnknownCSIString(fakeStringer("?CSI[49 59 57 65]?").String())
        if !ok {
                t.Fatalf("expected unknown CSI to parse")
        }
        if km.Type != tea.KeyUp || !km.Alt {
                t.Fatalf("expected Alt+Up; got Type=%v Alt=%v", km.Type, km.Alt)
        }

        // "?CSI[49 59 53 67]?" => "1;5C" => Ctrl+Right
        km, ok = keyMsgFromUnknownCSIString(fakeStringer("?CSI[49 59 53 67]?").String())
        if !ok {
                t.Fatalf("expected unknown CSI to parse ctrl+right")
        }
        if km.Type != tea.KeyCtrlRight {
                t.Fatalf("expected Ctrl+Right; got Type=%v Alt=%v", km.Type, km.Alt)
        }
}
