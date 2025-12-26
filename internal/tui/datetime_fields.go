package tui

import (
        "strings"
        "time"

        "clarity-cli/internal/model"
)

func parseDateTimeFieldsOrNow(dt *model.DateTime) (y int, m int, d int, h int, min int) {
        now := time.Now().UTC()
        y, m, d = now.Year(), int(now.Month()), now.Day()
        h, min = 0, 0

        if dt == nil {
                return
        }
        if t, err := time.Parse("2006-01-02", strings.TrimSpace(dt.Date)); err == nil {
                y, m, d = t.Year(), int(t.Month()), t.Day()
        }
        if dt.Time != nil {
                if tt, err := time.Parse("15:04", strings.TrimSpace(*dt.Time)); err == nil {
                        h, min = tt.Hour(), tt.Minute()
                }
        }
        return
}
