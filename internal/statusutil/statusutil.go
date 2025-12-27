package statusutil

import (
        "fmt"
        "strings"

        "clarity-cli/internal/model"
)

func NormalizeStatusID(s string) (string, error) {
        switch strings.ToUpper(strings.TrimSpace(s)) {
        case "TODO", "todo":
                return "todo", nil
        case "DOING", "doing":
                return "doing", nil
        case "DONE", "done":
                return "done", nil
        case "NONE", "none":
                return "", nil
        default:
                // For outline-defined statuses, we allow any non-empty id.
                s = strings.TrimSpace(s)
                if s == "" {
                        return "", fmt.Errorf("invalid status: empty")
                }
                return s, nil
        }
}

func ValidateStatusID(outline model.Outline, statusID string) bool {
        sid := strings.TrimSpace(statusID)
        if sid == "" {
                return true
        }
        for _, def := range outline.StatusDefs {
                if def.ID == sid {
                        return true
                }
        }
        return false
}

func IsEndState(outline model.Outline, statusID string) bool {
        sid := strings.TrimSpace(statusID)
        if sid == "" {
                return false
        }
        for _, def := range outline.StatusDefs {
                if def.ID == sid {
                        return def.IsEndState
                }
        }
        // Fallback for legacy outlines or stores without status defs.
        return strings.ToLower(sid) == "done"
}
