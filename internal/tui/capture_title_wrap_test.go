package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
)

func TestCaptureTitleModal_DoesNotWrapOnHyphen(t *testing.T) {
	// This is a visual regression test: lipgloss wrapping inside renderModalBox can introduce
	// line breaks at hyphens while typing a single-line title.
	var m captureModel
	m.width = 44
	m.height = 20
	m.modal = captureModalEditTitle
	m.titleInput = textinput.New()

	long := strings.Repeat("a", 40) + "-" + strings.Repeat("b", 40)
	(&m).openTitleModal("draft-root", long)

	out := m.renderModal()
	if strings.Contains(out, "-\n") {
		t.Fatalf("expected no wrap at hyphen; got output containing \"-\\n\"")
	}
}
