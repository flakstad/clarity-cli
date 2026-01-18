package tui

import (
	"os"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
)

type appearanceProfileID string

const (
	appearanceDefault appearanceProfileID = "default"
	appearanceNeon    appearanceProfileID = "neon"
	appearancePills   appearanceProfileID = "pills"
	appearanceMono    appearanceProfileID = "mono"
)

var (
	appearanceMu      sync.RWMutex
	currentAppearance appearanceProfileID = appearanceDefault
	knownAppearances                      = []appearanceProfileID{appearanceDefault, appearanceNeon, appearancePills, appearanceMono}
)

func applyAppearancePreference() {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("CLARITY_TUI_PROFILE")))
	if v == "" {
		setAppearanceProfile(appearanceDefault)
		return
	}
	setAppearanceProfile(appearanceProfileID(v))
}

func setAppearanceProfile(id appearanceProfileID) {
	appearanceMu.Lock()
	defer appearanceMu.Unlock()

	switch id {
	case appearanceDefault:
		currentAppearance = appearanceDefault
		statusNonEndStyle = defaultStatusNonEndStyle
		statusEndStyle = defaultStatusEndStyle
		metaPriorityStyle = defaultMetaPriorityStyle
		metaOnHoldStyle = defaultMetaOnHoldStyle
	case appearanceNeon:
		currentAppearance = appearanceNeon
		statusNonEndStyle = lipgloss.NewStyle().Foreground(ac("#a100ff", "#ff4fd8")).Bold(true)
		statusEndStyle = lipgloss.NewStyle().Foreground(ac("#007a3d", "#3ddc84")).Bold(true)
		metaPriorityStyle = lipgloss.NewStyle().Foreground(ac("#005f87", "#00d7ff")).Bold(true)
		metaOnHoldStyle = lipgloss.NewStyle().Foreground(ac("#af5f00", "#ffaf00")).Bold(true)
	case appearancePills:
		currentAppearance = appearancePills
		statusNonEndStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(ac("232", "255")).
			Background(ac("217", "53")).
			Bold(true)
		statusEndStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(ac("240", "252")).
			Background(ac("252", "236")).
			Bold(true)
		metaPriorityStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(ac("232", "255")).
			Background(ac("110", "30")).
			Bold(true)
		metaOnHoldStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(ac("232", "255")).
			Background(ac("214", "130")).
			Bold(true)
	case appearanceMono:
		currentAppearance = appearanceMono
		statusNonEndStyle = lipgloss.NewStyle().Foreground(colorSurfaceFg).Bold(true)
		statusEndStyle = faintIfDark(lipgloss.NewStyle().Foreground(colorMuted)).Strikethrough(true)
		metaPriorityStyle = lipgloss.NewStyle().Foreground(colorSurfaceFg).Underline(true)
		metaOnHoldStyle = lipgloss.NewStyle().Foreground(colorSurfaceFg).Underline(true)
	default:
		// Unknown value: ignore.
	}
}

func appearanceProfile() appearanceProfileID {
	appearanceMu.RLock()
	id := currentAppearance
	appearanceMu.RUnlock()
	return id
}

func appearanceLabel(id appearanceProfileID) string {
	switch id {
	case appearanceNeon:
		return "Neon"
	case appearancePills:
		return "Pills"
	case appearanceMono:
		return "Mono"
	default:
		return "Default"
	}
}
