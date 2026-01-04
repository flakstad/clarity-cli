package tui

import (
        "testing"
        "time"
)

func TestExpandCaptureTemplateString(t *testing.T) {
        ctx := captureExpansionContext{
                Now:       time.Date(2026, 1, 4, 12, 34, 56, 0, time.UTC),
                Workspace: "Flakstad Software",
                OutlineID: "out-123",
                Clipboard: "clip",
                Selection: "sel",
                URL:       "https://example.com",
        }

        got := expandCaptureTemplateString("{{date}} {{time}} {{now}} {{workspace}} {{outline}} {{clipboard}} {{selection}} {{url}}", ctx)
        if want := "2026-01-04 12:34 2026-01-04T12:34:56Z Flakstad Software out-123 clip sel https://example.com"; got != want {
                t.Fatalf("unexpected expansion:\nwant: %q\ngot:  %q", want, got)
        }
}
