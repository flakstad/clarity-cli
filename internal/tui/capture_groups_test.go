package tui

import (
	"testing"

	"clarity-cli/internal/store"
)

func TestCaptureTemplatePrefixLabels(t *testing.T) {
	t.Setenv("CLARITY_CONFIG_DIR", t.TempDir())

	base := []store.CaptureTemplate{
		{
			Name: "Work inbox",
			Keys: []string{"w", "i"},
			Target: store.CaptureTemplateTarget{
				Workspace: "Test Workspace",
				OutlineID: "out-1",
			},
		},
		{
			Name: "Work meeting",
			Keys: []string{"w", "m"},
			Target: store.CaptureTemplateTarget{
				Workspace: "Test Workspace",
				OutlineID: "out-1",
			},
		},
	}

	t.Run("derives_from_first_template_word", func(t *testing.T) {
		m, err := newCaptureModel(&store.GlobalConfig{CaptureTemplates: base}, "")
		if err != nil {
			t.Fatalf("newCaptureModel: %v", err)
		}

		var got string
		for _, it := range m.templateList.Items() {
			row, ok := it.(captureOptionItem)
			if !ok {
				continue
			}
			if row.key == "w" {
				got = row.label
				break
			}
		}
		if got != "Work (2)" {
			t.Fatalf("unexpected label for prefix 'w': %q", got)
		}
	})

	t.Run("uses_explicit_group_name_when_configured", func(t *testing.T) {
		cfg := &store.GlobalConfig{
			CaptureTemplates: base,
			CaptureTemplateGroups: []store.CaptureTemplateGroup{
				{Name: "Work stuff", Keys: []string{"w"}},
			},
		}
		m, err := newCaptureModel(cfg, "")
		if err != nil {
			t.Fatalf("newCaptureModel: %v", err)
		}

		var got string
		for _, it := range m.templateList.Items() {
			row, ok := it.(captureOptionItem)
			if !ok {
				continue
			}
			if row.key == "w" {
				got = row.label
				break
			}
		}
		if got != "Work stuff (2)" {
			t.Fatalf("unexpected label for prefix 'w': %q", got)
		}
	})
}
