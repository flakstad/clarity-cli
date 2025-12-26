package tui

import (
        "fmt"
        "strings"
        "time"

        "clarity-cli/internal/model"
)

// formatDateTimeOutline matches outline.js rendering:
// - date-only: "Jan 5"
// - date+time: "Jan 5 14:30" (24h)
func formatDateTimeOutline(dt *model.DateTime) string {
        if dt == nil {
                return ""
        }
        date := strings.TrimSpace(dt.Date)
        if date == "" {
                return ""
        }
        parsed, err := time.Parse("2006-01-02", date)
        if err != nil {
                // Best-effort: fall back to raw date string.
                if dt.Time != nil && strings.TrimSpace(*dt.Time) != "" {
                        return fmt.Sprintf("%s %s", date, strings.TrimSpace(*dt.Time))
                }
                return date
        }
        day := parsed.Format("Jan 2")
        if dt.Time == nil || strings.TrimSpace(*dt.Time) == "" {
                return day
        }
        return fmt.Sprintf("%s %s", day, strings.TrimSpace(*dt.Time))
}

func formatDueLabel(dt *model.DateTime) string {
        txt := formatDateTimeOutline(dt)
        if strings.TrimSpace(txt) == "" {
                return ""
        }
        return "due " + txt
}

func formatScheduleLabel(dt *model.DateTime) string {
        txt := formatDateTimeOutline(dt)
        if strings.TrimSpace(txt) == "" {
                return ""
        }
        return "on " + txt
}
