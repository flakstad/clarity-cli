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

func RequiresNote(outline model.Outline, statusID string) bool {
	statusID = strings.TrimSpace(statusID)
	if statusID == "" {
		return false
	}
	for _, def := range outline.StatusDefs {
		if strings.TrimSpace(def.ID) == statusID {
			return def.RequiresNote
		}
	}
	return false
}

// CheckboxUncheckedStatusID returns the status id to treat as "unchecked" for checkbox-style items.
// It prefers the first non-end-state status in the outline order, falling back to the first status id.
func CheckboxUncheckedStatusID(outline model.Outline) string {
	for _, def := range outline.StatusDefs {
		id := strings.TrimSpace(def.ID)
		if id == "" {
			continue
		}
		if !def.IsEndState {
			return id
		}
	}
	for _, def := range outline.StatusDefs {
		id := strings.TrimSpace(def.ID)
		if id != "" {
			return id
		}
	}
	return ""
}

// CheckboxCheckedStatusID returns the status id to treat as "checked" for checkbox-style items.
// It prefers the first end-state status in the outline order; if none exist, it falls back to the
// last non-empty status id.
func CheckboxCheckedStatusID(outline model.Outline) string {
	var last string
	for _, def := range outline.StatusDefs {
		id := strings.TrimSpace(def.ID)
		if id == "" {
			continue
		}
		last = id
		if def.IsEndState {
			return id
		}
	}
	return last
}

func IsCheckboxChecked(outline model.Outline, statusID string) bool {
	// Checkbox semantics are derived from end-state:
	// - any end-state status counts as "checked"
	// - any non-end status counts as "unchecked"
	//
	// When toggling, callers should still pick a concrete end/non-end status id
	// (CheckboxCheckedStatusID / CheckboxUncheckedStatusID).
	return IsEndState(outline, statusID)
}
