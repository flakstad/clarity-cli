package tui

import (
	"os"
	"strings"
	"sync"

	"clarity-cli/internal/store"

	"github.com/charmbracelet/lipgloss"
)

type appearanceProfileID string

const (
	appearanceDefault appearanceProfileID = "default"
	appearanceNeon    appearanceProfileID = "neon"
	appearancePills   appearanceProfileID = "pills"
	appearanceMono    appearanceProfileID = "mono"
	appearanceCustom  appearanceProfileID = "custom"
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
	knownAppearances                      = []appearanceProfileID{appearanceDefault, appearanceNeon, appearancePills, appearanceMono, appearanceCustom}

	currentListStyle listStyleID = listStyleCards

	customProfile *store.TUICustomProfile
)

func applyAppearancePreference() {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("CLARITY_TUI_PROFILE")))
	if cfg, err := store.LoadConfig(); err == nil && cfg != nil && cfg.TUI != nil {
		customProfile = cfg.TUI.CustomProfile
	}
	if v == "" {
		if cfg, err := store.LoadConfig(); err == nil && cfg != nil && cfg.TUI != nil {
			if vv := strings.ToLower(strings.TrimSpace(cfg.TUI.Profile)); vv != "" {
				setAppearanceProfile(appearanceProfileID(vv))
				return
			}
		}
		setAppearanceProfile(appearanceDefault)
		return
	}
	setAppearanceProfile(appearanceProfileID(v))
}

func applyListStylePreference() {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("CLARITY_TUI_LISTS")))
	switch v {
	case "", "cards":
		if v == "" {
			if cfg, err := store.LoadConfig(); err == nil && cfg != nil && cfg.TUI != nil {
				if vv := strings.ToLower(strings.TrimSpace(cfg.TUI.Lists)); vv != "" {
					v = vv
				}
			}
		}
		switch v {
		case "", "cards":
			setListStyle(listStyleCards)
		case "rows":
			setListStyle(listStyleRows)
		case "minimal":
			setListStyle(listStyleMinimal)
		}
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
		progressFillBg = defaultProgressFillBg
		progressEmptyBg = defaultProgressEmptyBg
		progressFillFg = defaultProgressFillFg
		progressEmptyFg = defaultProgressEmptyFg
		colorSelectedBg = defaultColorSelectedBg
		colorSelectedFg = defaultColorSelectedFg
		colorSelectedBorder = defaultColorSelectedBorder
		colorCardBorder = defaultColorCardBorder
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
		progressFillBg = ac("159", "57")
		progressEmptyBg = ac("255", "235")
		progressFillFg = ac("232", "255")
		progressEmptyFg = ac("240", "252")
		colorSelectedBg = ac("225", "55")
		colorSelectedFg = ac("232", "255")
		colorSelectedBorder = ac("57", "225")
		colorCardBorder = defaultColorCardBorder
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
		progressFillBg = ac("153", "24")
		progressEmptyBg = ac("255", "235")
		progressFillFg = ac("232", "255")
		progressEmptyFg = ac("240", "252")
		colorSelectedBg = ac("153", "24")
		colorSelectedFg = ac("232", "255")
		colorSelectedBorder = ac("153", "24")
		colorCardBorder = defaultColorCardBorder
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
		progressFillBg = defaultProgressFillBg
		progressEmptyBg = defaultProgressEmptyBg
		progressFillFg = defaultProgressFillFg
		progressEmptyFg = defaultProgressEmptyFg
		colorSelectedBg = ac("253", "236")
		colorSelectedFg = colorSurfaceFg
		colorSelectedBorder = defaultColorSelectedBorder
		colorCardBorder = defaultColorCardBorder
	case appearanceCustom:
		if customProfile == nil {
			return
		}
		currentAppearance = appearanceCustom
		applyCustom := func(def lipgloss.AdaptiveColor, c *store.AdaptiveColor) lipgloss.AdaptiveColor {
			if c == nil {
				return def
			}
			light := strings.TrimSpace(c.Light)
			dark := strings.TrimSpace(c.Dark)
			if light == "" {
				light = def.Light
			}
			if dark == "" {
				dark = def.Dark
			}
			return ac(light, dark)
		}
		colorSelectedBg = applyCustom(defaultColorSelectedBg, customProfile.SelectedBg)
		colorSelectedFg = applyCustom(defaultColorSelectedFg, customProfile.SelectedFg)
		colorSelectedBorder = defaultColorSelectedBorder
		colorCardBorder = defaultColorCardBorder

		statusNonEndStyle = lipgloss.NewStyle().Foreground(applyCustom(ac("#d16d7a", "#d16d7a"), customProfile.StatusNonEndFg)).Bold(true)
		statusEndStyle = lipgloss.NewStyle().Foreground(applyCustom(ac("#6c757d", "#6c757d"), customProfile.StatusEndFg)).Bold(true)

		metaPriorityStyle = lipgloss.NewStyle().Foreground(applyCustom(ac("#5f9fb0", "#5f9fb0"), customProfile.MetaPriorityFg)).Bold(true)
		metaOnHoldStyle = lipgloss.NewStyle().Foreground(applyCustom(ac("#f39c12", "#f39c12"), customProfile.MetaOnHoldFg)).Bold(true)
		metaDueStyle = lipgloss.NewStyle().Foreground(applyCustom(ac("240", "245"), customProfile.MetaDueFg))
		metaScheduleStyle = lipgloss.NewStyle().Foreground(applyCustom(ac("240", "245"), customProfile.MetaScheduleFg))
		metaAssignStyle = lipgloss.NewStyle().Foreground(applyCustom(ac("240", "245"), customProfile.MetaAssignFg))
		metaCommentStyle = lipgloss.NewStyle().Foreground(applyCustom(ac("240", "245"), customProfile.MetaCommentFg))
		metaTagStyle = lipgloss.NewStyle().Foreground(applyCustom(ac("240", "245"), customProfile.MetaTagFg))

		progressFillBg = applyCustom(defaultProgressFillBg, customProfile.ProgressFillBg)
		progressEmptyBg = applyCustom(defaultProgressEmptyBg, customProfile.ProgressEmptyBg)
		progressFillFg = applyCustom(defaultProgressFillFg, customProfile.ProgressFillFg)
		progressEmptyFg = applyCustom(defaultProgressEmptyFg, customProfile.ProgressEmptyFg)
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
	case appearanceCustom:
		return "Custom"
	default:
		return "Default"
	}
}
