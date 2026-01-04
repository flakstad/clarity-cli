package store

import (
        "errors"
        "fmt"
        "strings"
        "unicode/utf8"
)

type CaptureTemplateTarget struct {
        Workspace string `json:"workspace"`
        OutlineID string `json:"outlineId"`
}

type CaptureTemplateDefaults struct {
        Title       string   `json:"title,omitempty"`
        Description string   `json:"description,omitempty"`
        Tags        []string `json:"tags,omitempty"`
}

type CaptureTemplate struct {
        // Name is a human label for selection UIs.
        Name string `json:"name"`
        // Keys is a multi-key sequence (each entry must be exactly 1 rune) used for org-capture style selection.
        Keys []string `json:"keys"`
        // Target identifies where the captured item should be created by default.
        Target CaptureTemplateTarget `json:"target"`
        // Defaults are optional initial values used to seed the capture draft.
        Defaults CaptureTemplateDefaults `json:"defaults,omitempty"`
}

func NormalizeCaptureTemplateKeys(keys []string) ([]string, error) {
        if len(keys) == 0 {
                return nil, errors.New("keys is empty")
        }
        out := make([]string, 0, len(keys))
        for _, k := range keys {
                k = strings.TrimSpace(k)
                if k == "" {
                        return nil, errors.New("keys contains empty entry")
                }
                if utf8.RuneCountInString(k) != 1 {
                        return nil, fmt.Errorf("key %q must be exactly 1 rune", k)
                }
                out = append(out, k)
        }
        return out, nil
}

func NormalizeCaptureTemplateTags(tags []string) []string {
        if len(tags) == 0 {
                return nil
        }
        seen := map[string]bool{}
        out := make([]string, 0, len(tags))
        for _, t := range tags {
                t = strings.TrimSpace(t)
                t = strings.TrimPrefix(t, "#")
                if t == "" || seen[t] {
                        continue
                }
                seen[t] = true
                out = append(out, t)
        }
        if len(out) == 0 {
                return nil
        }
        return out
}

func ValidateCaptureTemplates(cfg *GlobalConfig) error {
        if cfg == nil {
                return nil
        }
        seen := map[string]bool{}
        for i, t := range cfg.CaptureTemplates {
                name := strings.TrimSpace(t.Name)
                if name == "" {
                        return fmt.Errorf("captureTemplates[%d].name is empty", i)
                }

                keys, err := NormalizeCaptureTemplateKeys(t.Keys)
                if err != nil {
                        return fmt.Errorf("captureTemplates[%d].keys: %w", i, err)
                }
                keyPath := strings.Join(keys, "")
                if seen[keyPath] {
                        return fmt.Errorf("duplicate capture template keys: %q", keyPath)
                }
                seen[keyPath] = true

                ws := strings.TrimSpace(t.Target.Workspace)
                if ws == "" {
                        return fmt.Errorf("captureTemplates[%d].target.workspace is empty", i)
                }
                outID := strings.TrimSpace(t.Target.OutlineID)
                if outID == "" {
                        return fmt.Errorf("captureTemplates[%d].target.outlineId is empty", i)
                }

                // Normalize defaults (validation is best-effort; preserve backward compatibility).
                _ = strings.TrimSpace(t.Defaults.Title)
                _ = strings.TrimSpace(t.Defaults.Description)
                _ = NormalizeCaptureTemplateTags(t.Defaults.Tags)
        }
        return nil
}
