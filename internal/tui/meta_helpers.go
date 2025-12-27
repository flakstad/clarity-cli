package tui

import (
        "sort"
        "strings"

        "clarity-cli/internal/store"
)

func normalizeTag(tag string) string {
        tag = strings.TrimSpace(tag)
        tag = strings.TrimPrefix(tag, "#")
        tag = strings.TrimSpace(tag)
        return tag
}

func uniqueSortedStrings(xs []string) []string {
        seen := map[string]bool{}
        out := make([]string, 0, len(xs))
        for _, x := range xs {
                x = strings.TrimSpace(x)
                if x == "" || seen[x] {
                        continue
                }
                seen[x] = true
                out = append(out, x)
        }
        sort.Slice(out, func(i, j int) bool {
                ai := strings.ToLower(out[i])
                aj := strings.ToLower(out[j])
                if ai == aj {
                        return out[i] < out[j]
                }
                return ai < aj
        })
        return out
}

func actorDisplayLabel(db *store.DB, actorID string) string {
        actorID = strings.TrimSpace(actorID)
        if actorID == "" {
                return ""
        }
        if db != nil {
                if a, ok := db.FindActor(actorID); ok && a != nil {
                        if nm := strings.TrimSpace(a.Name); nm != "" {
                                return nm
                        }
                }
        }
        return actorID
}
