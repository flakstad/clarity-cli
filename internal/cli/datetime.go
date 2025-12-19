package cli

import (
        "fmt"
        "regexp"
        "strings"
        "time"

        "clarity-cli/internal/model"
)

var (
        reDateOnly = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
        reDateTime = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})[ T](\d{2}:\d{2})(?::\d{2})?$`)
)

// parseDateTime parses:
// - YYYY-MM-DD (date-only)
// - YYYY-MM-DD HH:MM (local date+time)
// - RFC3339 / RFC3339Nano (timezone-aware)
//
// It returns a DateTime where Time is nil for date-only inputs.
func parseDateTime(s string) (*model.DateTime, error) {
        s = strings.TrimSpace(s)
        if s == "" {
                return nil, fmt.Errorf("empty datetime")
        }

        if reDateOnly.MatchString(s) {
                return &model.DateTime{Date: s, Time: nil}, nil
        }

        if m := reDateTime.FindStringSubmatch(s); m != nil {
                date := m[1]
                hm := m[2]
                return &model.DateTime{Date: date, Time: &hm}, nil
        }

        // RFC3339: interpret as absolute time, store as date+time in UTC.
        if ts, err := time.Parse(time.RFC3339Nano, s); err == nil {
                ts = ts.UTC()
                date := ts.Format("2006-01-02")
                hm := ts.Format("15:04")
                return &model.DateTime{Date: date, Time: &hm}, nil
        }
        if ts, err := time.Parse(time.RFC3339, s); err == nil {
                ts = ts.UTC()
                date := ts.Format("2006-01-02")
                hm := ts.Format("15:04")
                return &model.DateTime{Date: date, Time: &hm}, nil
        }

        return nil, fmt.Errorf("invalid datetime %q (expected YYYY-MM-DD, YYYY-MM-DD HH:MM, or RFC3339)", s)
}
