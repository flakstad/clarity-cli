package store

import (
        "regexp"
        "strings"
        "strconv"

        "clarity-cli/internal/model"
)

var reNonID = regexp.MustCompile(`[^a-z0-9-]+`)

func slugifyStatusID(label string) string {
        s := strings.ToLower(strings.TrimSpace(label))
        s = strings.ReplaceAll(s, " ", "-")
        s = reNonID.ReplaceAllString(s, "-")
        s = strings.Trim(s, "-")
        if s == "" {
                return "status"
        }
        return s
}

// NewStatusIDFromLabel returns a stable id derived from label, and disambiguates with suffixes if needed.
func NewStatusIDFromLabel(o *model.Outline, label string) string {
        base := slugifyStatusID(label)
        id := base
        used := map[string]bool{}
        for _, def := range o.StatusDefs {
                used[def.ID] = true
        }
        if !used[id] {
                return id
        }
        for i := 2; i < 1000; i++ {
                candidate := base + "-" + strconv.Itoa(i)
                if !used[candidate] {
                        return candidate
                }
        }
        return base + "-x"
}
