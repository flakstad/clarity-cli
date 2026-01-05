package tui

import (
        "fmt"
        "sort"
        "strings"

        "clarity-cli/internal/model"
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

func actorNameOrID(db *store.DB, actorID string) string {
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

func parseAgentSessionKey(name string) (string, bool) {
        // Expected shape: "[agent-session:<key>] Agent"
        name = strings.TrimSpace(name)
        const pfx = "[agent-session:"
        if !strings.HasPrefix(name, pfx) {
                return "", false
        }
        rest := strings.TrimPrefix(name, pfx)
        end := strings.Index(rest, "]")
        if end < 0 {
                return "", false
        }
        key := strings.TrimSpace(rest[:end])
        if key == "" {
                return "", false
        }
        return key, true
}

// actorCompactLabel is optimized for "at a glance" rendering in lists/badges.
// It intentionally collapses agent session names into a stable shorthand.
func actorCompactLabel(db *store.DB, actorID string) string {
        actorID = strings.TrimSpace(actorID)
        if actorID == "" {
                return ""
        }
        if db != nil {
                if a, ok := db.FindActor(actorID); ok && a != nil {
                        if a.Kind == model.ActorKindAgent {
                                return "agent"
                        }
                        if nm := strings.TrimSpace(a.Name); nm != "" {
                                return nm
                        }
                }
        }
        return actorID
}

func actorDisplayLabel(db *store.DB, actorID string) string {
        return actorCompactLabel(db, actorID)
}

// actorDetailLabel is optimized for item detail panes where we want "who is responsible"
// for an agent identity without showing session noise by default.
func actorDetailLabel(db *store.DB, actorID string) string {
        actorID = strings.TrimSpace(actorID)
        if actorID == "" {
                return ""
        }
        if db == nil {
                return actorID
        }
        a, ok := db.FindActor(actorID)
        if !ok || a == nil {
                return actorID
        }
        if a.Kind != model.ActorKindAgent {
                return actorCompactLabel(db, actorID)
        }

        // Show the owning human as the responsible party.
        humanID, ok := db.HumanUserIDForActor(actorID)
        if !ok || strings.TrimSpace(humanID) == "" {
                return "agent"
        }
        humanLabel := actorNameOrID(db, humanID)
        if strings.TrimSpace(humanLabel) == "" {
                humanLabel = humanID
        }
        return fmt.Sprintf("agent (%s)", humanLabel)
}

// actorPickerLabel is optimized for selection lists. It keeps the "agent" shorthand,
// but adds enough context to disambiguate multiple agent identities.
func actorPickerLabel(db *store.DB, actorID string) string {
        actorID = strings.TrimSpace(actorID)
        if actorID == "" {
                return ""
        }
        if db == nil {
                return actorID
        }
        a, ok := db.FindActor(actorID)
        if !ok || a == nil {
                return actorID
        }
        if a.Kind != model.ActorKindAgent {
                return actorCompactLabel(db, actorID)
        }

        var parts []string
        if humanID, ok := db.HumanUserIDForActor(actorID); ok && strings.TrimSpace(humanID) != "" {
                parts = append(parts, actorNameOrID(db, humanID))
        }
        if session, ok := parseAgentSessionKey(a.Name); ok {
                parts = append(parts, session)
        }
        if len(parts) == 0 {
                return "agent"
        }
        return fmt.Sprintf("agent (%s)", strings.Join(parts, ", "))
}
