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

type CaptureTemplatePrompt struct {
	// Name is the variable name used for expansions (e.g. {{project}}).
	Name string `json:"name"`
	// Label is shown to the user.
	Label string `json:"label"`
	// Type is one of: string | multiline | choice | confirm
	Type string `json:"type"`

	// Default is an optional initial value. For choice prompts, it should match an entry in Options.
	Default string `json:"default,omitempty"`

	// Options are required for choice prompts.
	Options []string `json:"options,omitempty"`

	// Required controls whether an empty answer is allowed (default: false).
	Required bool `json:"required,omitempty"`
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
	// Prompts are optional questions asked before capture starts; answers become template expansion variables.
	Prompts []CaptureTemplatePrompt `json:"prompts,omitempty"`
}

// CaptureTemplateGroup defines a friendly label for a key prefix used in capture template selection.
// Example: {"name":"Work","keys":["w"]} makes the "w" prefix show as "Work" instead of "(prefix)".
type CaptureTemplateGroup struct {
	Name string   `json:"name"`
	Keys []string `json:"keys"`
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
	if err := ValidateCaptureTemplateGroups(cfg); err != nil {
		return err
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

		if err := ValidateCaptureTemplatePrompts(t.Prompts); err != nil {
			return fmt.Errorf("captureTemplates[%d].prompts: %w", i, err)
		}
	}
	return nil
}

func ValidateCaptureTemplateGroups(cfg *GlobalConfig) error {
	if cfg == nil || len(cfg.CaptureTemplateGroups) == 0 {
		return nil
	}

	// Pre-normalize templates so we can validate group prefixes point somewhere.
	tmplKeys := make([][]string, 0, len(cfg.CaptureTemplates))
	for i := range cfg.CaptureTemplates {
		keys, err := NormalizeCaptureTemplateKeys(cfg.CaptureTemplates[i].Keys)
		if err != nil {
			continue
		}
		tmplKeys = append(tmplKeys, keys)
	}

	seen := map[string]bool{}
	for i, g := range cfg.CaptureTemplateGroups {
		name := strings.TrimSpace(g.Name)
		if name == "" {
			return fmt.Errorf("captureTemplateGroups[%d].name is empty", i)
		}
		keys, err := NormalizeCaptureTemplateKeys(g.Keys)
		if err != nil {
			return fmt.Errorf("captureTemplateGroups[%d].keys: %w", i, err)
		}
		keyPath := strings.Join(keys, "")
		if seen[keyPath] {
			return fmt.Errorf("duplicate capture template group keys: %q", keyPath)
		}
		seen[keyPath] = true

		// Best-effort: ensure the group key path is actually a prefix of at least one template.
		// If templates haven't been configured yet, allow groups to exist (so users can set up
		// groups before templates).
		if len(tmplKeys) > 0 {
			ok := false
			for _, tk := range tmplKeys {
				if len(tk) < len(keys) {
					continue
				}
				match := true
				for j := range keys {
					if tk[j] != keys[j] {
						match = false
						break
					}
				}
				if match {
					ok = true
					break
				}
			}
			if !ok {
				return fmt.Errorf("captureTemplateGroups[%d]: keys %q do not match any capture template prefix", i, keyPath)
			}
		}
	}
	return nil
}

func ValidateCaptureTemplatePrompts(prompts []CaptureTemplatePrompt) error {
	if len(prompts) == 0 {
		return nil
	}
	reserved := map[string]bool{
		"now":       true,
		"date":      true,
		"time":      true,
		"workspace": true,
		"outline":   true,
		"clipboard": true,
		"selection": true,
		"url":       true,
	}
	seen := map[string]bool{}
	for i, p := range prompts {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			return fmt.Errorf("[%d].name is empty", i)
		}
		if strings.ContainsAny(name, " \t\r\n") {
			return fmt.Errorf("[%d].name contains whitespace: %q", i, name)
		}
		if reserved[name] {
			return fmt.Errorf("[%d].name uses reserved token: %q", i, name)
		}
		if seen[name] {
			return fmt.Errorf("[%d].name is duplicated: %q", i, name)
		}
		seen[name] = true

		label := strings.TrimSpace(p.Label)
		if label == "" {
			label = name
		}

		switch strings.TrimSpace(p.Type) {
		case "string", "multiline", "choice", "confirm":
		default:
			return fmt.Errorf("[%d] %q: invalid type %q (expected string|multiline|choice|confirm)", i, label, p.Type)
		}

		if strings.TrimSpace(p.Type) == "choice" {
			if len(p.Options) == 0 {
				return fmt.Errorf("[%d] %q: choice prompt requires options", i, label)
			}
			for j, opt := range p.Options {
				if strings.TrimSpace(opt) == "" {
					return fmt.Errorf("[%d] %q: options[%d] is empty", i, label, j)
				}
			}
		}
	}
	return nil
}
