package store

import "testing"

func TestNormalizeCaptureTemplateKeys(t *testing.T) {
        t.Run("empty", func(t *testing.T) {
                if _, err := NormalizeCaptureTemplateKeys(nil); err == nil {
                        t.Fatalf("expected error")
                }
        })

        t.Run("trims_and_validates_one_rune", func(t *testing.T) {
                got, err := NormalizeCaptureTemplateKeys([]string{" a ", "β"})
                if err != nil {
                        t.Fatalf("unexpected error: %v", err)
                }
                if len(got) != 2 || got[0] != "a" || got[1] != "β" {
                        t.Fatalf("unexpected keys: %#v", got)
                }
        })

        t.Run("rejects_multi_rune", func(t *testing.T) {
                if _, err := NormalizeCaptureTemplateKeys([]string{"ab"}); err == nil {
                        t.Fatalf("expected error")
                }
        })
}

func TestValidateCaptureTemplates(t *testing.T) {
        cfg := &GlobalConfig{
                CaptureTemplates: []CaptureTemplate{
                        {
                                Name: "Work",
                                Keys: []string{"w"},
                                Target: CaptureTemplateTarget{
                                        Workspace: "Flakstad Software",
                                        OutlineID: "out-123",
                                },
                        },
                        {
                                Name: "Duplicate",
                                Keys: []string{"w"},
                                Target: CaptureTemplateTarget{
                                        Workspace: "Personal",
                                        OutlineID: "out-999",
                                },
                        },
                },
        }
        if err := ValidateCaptureTemplates(cfg); err == nil {
                t.Fatalf("expected duplicate-keys error")
        }
}

func TestNormalizeCaptureTemplateTags(t *testing.T) {
        got := NormalizeCaptureTemplateTags([]string{"  foo ", "#bar", "", "foo"})
        if len(got) != 2 || got[0] != "foo" || got[1] != "bar" {
                t.Fatalf("unexpected tags: %#v", got)
        }
}

func TestValidateCaptureTemplates_Prompts(t *testing.T) {
        base := CaptureTemplate{
                Name: "Work",
                Keys: []string{"w"},
                Target: CaptureTemplateTarget{
                        Workspace: "Flakstad Software",
                        OutlineID: "out-123",
                },
        }

        t.Run("rejects_reserved_name", func(t *testing.T) {
                cfg := &GlobalConfig{CaptureTemplates: []CaptureTemplate{
                        func() CaptureTemplate {
                                t := base
                                t.Prompts = []CaptureTemplatePrompt{{Name: "url", Label: "URL", Type: "string"}}
                                return t
                        }(),
                }}
                if err := ValidateCaptureTemplates(cfg); err == nil {
                        t.Fatalf("expected error")
                }
        })

        t.Run("rejects_choice_without_options", func(t *testing.T) {
                cfg := &GlobalConfig{CaptureTemplates: []CaptureTemplate{
                        func() CaptureTemplate {
                                t := base
                                t.Prompts = []CaptureTemplatePrompt{{Name: "project", Label: "Project", Type: "choice"}}
                                return t
                        }(),
                }}
                if err := ValidateCaptureTemplates(cfg); err == nil {
                        t.Fatalf("expected error")
                }
        })

        t.Run("rejects_duplicate_prompt_name", func(t *testing.T) {
                cfg := &GlobalConfig{CaptureTemplates: []CaptureTemplate{
                        func() CaptureTemplate {
                                t := base
                                t.Prompts = []CaptureTemplatePrompt{
                                        {Name: "project", Label: "Project", Type: "string"},
                                        {Name: "project", Label: "Project again", Type: "string"},
                                }
                                return t
                        }(),
                }}
                if err := ValidateCaptureTemplates(cfg); err == nil {
                        t.Fatalf("expected error")
                }
        })
}
