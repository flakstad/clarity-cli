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

type listStyleID string

const (
	listStyleCards   listStyleID = "cards"
	listStyleRows    listStyleID = "rows"
	listStyleMinimal listStyleID = "minimal"
)

var (
	appearanceMu      sync.RWMutex
	currentAppearance appearanceProfileID = appearanceDefault
	knownAppearances                      = []appearanceProfileID{appearanceDefault, appearanceNeon, appearancePills, appearanceMono}

	currentListStyle listStyleID = listStyleCards
)

func applyAppearancePreference() {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("CLARITY_TUI_PROFILE")))
	if v == "" {
		setAppearanceProfile(appearanceDefault)
		return
	}
	setAppearanceProfile(appearanceProfileID(v))
}

func applyListStylePreference() {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("CLARITY_TUI_LISTS")))
	switch v {
	case "", "cards":
		setListStyle(listStyleCards)
	case "rows":
		setListStyle(listStyleRows)
	case "minimal":
		setListStyle(listStyleMinimal)
	default:
		// Unknown value: ignore.
	}
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
		metaDueStyle = defaultMetaDueStyle
		metaScheduleStyle = defaultMetaScheduleStyle
		metaAssignStyle = defaultMetaAssignStyle
		metaCommentStyle = defaultMetaCommentStyle
		metaTagStyle = defaultMetaTagStyle
	case appearanceNeon:
		currentAppearance = appearanceNeon
		statusNonEndStyle = lipgloss.NewStyle().Foreground(ac("#a100ff", "#ff4fd8")).Bold(true)
		statusEndStyle = lipgloss.NewStyle().Foreground(ac("#007a3d", "#3ddc84")).Bold(true)
		metaPriorityStyle = lipgloss.NewStyle().Foreground(ac("#005f87", "#00d7ff")).Bold(true)
		metaOnHoldStyle = lipgloss.NewStyle().Foreground(ac("#af5f00", "#ffaf00")).Bold(true)
		metaDueStyle = lipgloss.NewStyle().Foreground(ac("#d70000", "#ff5f5f")).Bold(true)
		metaScheduleStyle = lipgloss.NewStyle().Foreground(ac("#005f87", "#00afff")).Bold(true)
		metaAssignStyle = lipgloss.NewStyle().Foreground(ac("#5f00af", "#af87ff")).Bold(true)
		metaCommentStyle = lipgloss.NewStyle().Foreground(ac("#af005f", "#ff5fd7")).Bold(true)
		metaTagStyle = lipgloss.NewStyle().Foreground(ac("#005f00", "#5fff87")).Bold(true)
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
		metaDueStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(ac("232", "255")).
			Background(ac("203", "124")).
			Bold(true)
		metaScheduleStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(ac("232", "255")).
			Background(ac("111", "25")).
			Bold(true)
		metaAssignStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(ac("232", "255")).
			Background(ac("141", "97")).
			Bold(true)
		metaCommentStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(ac("232", "255")).
			Background(ac("176", "90")).
			Bold(true)
		metaTagStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(ac("232", "255")).
			Background(ac("114", "28")).
			Bold(true)
	case appearanceMono:
		currentAppearance = appearanceMono
		statusNonEndStyle = lipgloss.NewStyle().Foreground(colorSurfaceFg).Bold(true)
		statusEndStyle = faintIfDark(lipgloss.NewStyle().Foreground(colorMuted)).Strikethrough(true)
		metaPriorityStyle = lipgloss.NewStyle().Foreground(colorSurfaceFg).Underline(true)
		metaOnHoldStyle = lipgloss.NewStyle().Foreground(colorSurfaceFg).Underline(true)
		metaDueStyle = lipgloss.NewStyle().Foreground(colorSurfaceFg)
		metaScheduleStyle = lipgloss.NewStyle().Foreground(colorSurfaceFg)
		metaAssignStyle = lipgloss.NewStyle().Foreground(colorSurfaceFg)
		metaCommentStyle = lipgloss.NewStyle().Foreground(colorSurfaceFg)
		metaTagStyle = lipgloss.NewStyle().Foreground(colorSurfaceFg)
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

func setListStyle(id listStyleID) {
	appearanceMu.Lock()
	defer appearanceMu.Unlock()
	switch id {
	case listStyleCards, listStyleRows, listStyleMinimal:
		currentListStyle = id
	default:
		// ignore
	}
}

func listStyle() listStyleID {
	appearanceMu.RLock()
	id := currentListStyle
	appearanceMu.RUnlock()
	return id
}

func listStyleLabel(id listStyleID) string {
	switch id {
	case listStyleRows:
		return "Rows"
	case listStyleMinimal:
		return "Minimal"
	default:
		return "Cards"
	}
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
