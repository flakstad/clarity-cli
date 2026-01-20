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
	appearanceDefault   appearanceProfileID = "default"
	appearanceAlabaster appearanceProfileID = "alabaster"
	appearanceDracula   appearanceProfileID = "dracula"
	appearanceGruvbox   appearanceProfileID = "gruvbox"
	appearanceSolarized appearanceProfileID = "solarized"

	// Deprecated aliases (kept for config/back-compat).
	appearanceMidnight appearanceProfileID = "midnight" // alias of dracula
	appearancePaper    appearanceProfileID = "paper"    // alias of solarized
	appearanceTerminal appearanceProfileID = "terminal"
	appearanceNeon     appearanceProfileID = "neon"
	appearancePills    appearanceProfileID = "pills"
	appearanceMono     appearanceProfileID = "mono"
	appearanceCustom   appearanceProfileID = "custom"
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
	knownAppearances                      = []appearanceProfileID{appearanceDefault, appearanceAlabaster, appearanceDracula, appearanceGruvbox, appearanceSolarized, appearanceNeon, appearancePills, appearanceMono, appearanceTerminal, appearanceCustom}

	currentListStyle listStyleID = listStyleCards

	customProfile *store.TUICustomProfile
)

func resetAppearancePaletteToDefaults() {
	colorMuted = defaultColorMuted
	colorChromeMutedFg = defaultColorChromeMutedFg
	colorChromeSubtleFg = defaultColorChromeSubtleFg
	colorSurfaceBg = defaultColorSurfaceBg
	colorSurfaceFg = defaultColorSurfaceFg
	colorControlBg = defaultColorControlBg
	colorInputBg = defaultColorInputBg
	colorAccent = defaultColorAccent
	colorAccentFg = defaultColorAccentFg
	colorShadow = defaultColorShadow
	colorCardMetaFg = defaultColorCardMetaFg
	colorFlashErrorBg = defaultColorFlashErrorBg
	colorModalSurfaceBg = defaultColorModalSurfaceBg
	colorModalSurfaceFg = defaultColorModalSurfaceFg
	colorModalHeaderBg = defaultColorModalHeaderBg
	colorModalHeaderFg = defaultColorModalHeaderFg
}

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

	// Back-compat aliases.
	switch id {
	case appearanceMidnight:
		id = appearanceDracula
	case appearancePaper:
		id = appearanceSolarized
	}

	switch id {
	case appearanceDefault:
		currentAppearance = appearanceDefault
		resetAppearancePaletteToDefaults()
		// Modals inherit from the current surface + control surfaces.
		colorModalSurfaceBg = colorSurfaceBg
		colorModalSurfaceFg = colorSurfaceFg
		colorModalHeaderBg = colorControlBg
		colorModalHeaderFg = colorSurfaceFg
		statusNonEndStyle = defaultStatusNonEndStyle
		statusEndStyle = defaultStatusEndStyle
		metaPriorityStyle = defaultMetaPriorityStyle
		metaOnHoldStyle = defaultMetaOnHoldStyle
		metaDueStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaScheduleStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaAssignStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaCommentStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaTagStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		progressFillBg = defaultProgressFillBg
		progressEmptyBg = defaultProgressEmptyBg
		progressFillFg = defaultProgressFillFg
		progressEmptyFg = defaultProgressEmptyFg
		colorSelectedBg = defaultColorSelectedBg
		colorSelectedFg = defaultColorSelectedFg
		colorSelectedBorder = defaultColorSelectedBorder
		colorCardBorder = defaultColorCardBorder
	case appearanceAlabaster:
		currentAppearance = appearanceAlabaster
		resetAppearancePaletteToDefaults()

		// Alabaster (light-first, minimal, low-chroma).
		// Dark variant is "Alabaster-inspired" for OS-dark terminals.
		colorSurfaceBg = ac("#f7f7f7", "#0f0f0f")
		colorSurfaceFg = ac("#434343", "#cecece")
		colorControlBg = ac("#eeeeee", "#161616")
		colorInputBg = ac("#ffffff", "#111111")

		colorSelectedBg = ac("#e9e9e9", "#262626")
		colorSelectedFg = ac("#1f1f1f", "#f8f8f8")
		colorSelectedBorder = ac("#325cc0", "#5f87d7")
		colorCardBorder = ac("#d0d0d0", "#333333")

		colorMuted = ac("#777777", "#7a7a7a")
		colorChromeMutedFg = ac("#6f6f6f", "#9a9a9a")
		colorChromeSubtleFg = ac("#9a9a9a", "#6a6a6a")

		colorAccent = ac("#325cc0", "#5f87d7")
		colorAccentFg = ac("#ffffff", "#0f0f0f")
		colorShadow = colorSurfaceBg
		colorCardMetaFg = colorChromeMutedFg
		colorFlashErrorBg = ac("#ff5f5f", "#ff5f5f")

		colorModalSurfaceBg = colorControlBg
		colorModalSurfaceFg = colorSurfaceFg
		colorModalHeaderBg = ac("#e9e9e9", "#262626")
		colorModalHeaderFg = colorSurfaceFg

		statusNonEndStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
		statusEndStyle = lipgloss.NewStyle().Foreground(ac("#22863a", "#97e023")).Bold(true)
		metaPriorityStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
		metaOnHoldStyle = lipgloss.NewStyle().Foreground(ac("#ff8b00", "#ffaf00")).Bold(true)
		metaDueStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaScheduleStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaAssignStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaCommentStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaTagStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)

		progressFillBg = colorAccent
		progressEmptyBg = colorCardBorder
		progressFillFg = colorAccentFg
		progressEmptyFg = colorSurfaceFg
	case appearanceDracula:
		currentAppearance = appearanceDracula
		resetAppearancePaletteToDefaults()

		// Dracula-ish (popular dark theme).
		// Provide both a light+dark palette so this theme remains readable when
		// the terminal follows OS light mode.
		colorSurfaceBg = ac("#f8f8f2", "#282a36")
		colorSurfaceFg = ac("#282a36", "#f8f8f2")
		colorControlBg = ac("#e9e9e2", "#1f202a")
		colorInputBg = ac("#e1e1db", "#1b1c25")
		colorSelectedBg = ac("#d7d7cf", "#44475a")
		colorSelectedFg = ac("#282a36", "#f8f8f2")
		colorSelectedBorder = ac("#6c4aa6", "#bd93f9")
		colorCardBorder = ac("#c9c9c1", "#44475a")
		colorMuted = ac("#4b5563", "#9aa0b1")
		colorChromeMutedFg = ac("#475569", "#c0c3d2")
		colorChromeSubtleFg = ac("#64748b", "#7a7f99")
		colorAccent = ac("#0ea5e9", "#8be9fd")
		colorAccentFg = ac("#f8f8f2", "#282a36")
		colorShadow = colorSurfaceBg
		colorCardMetaFg = colorChromeMutedFg
		colorFlashErrorBg = ac("#b91c1c", "#ff5555")

		colorModalSurfaceBg = colorControlBg
		colorModalSurfaceFg = colorSurfaceFg
		colorModalHeaderBg = ac("#d7d7cf", "#44475a")
		colorModalHeaderFg = colorSurfaceFg

		statusNonEndStyle = lipgloss.NewStyle().Foreground(ac("#b83280", "#ff79c6")).Bold(true) // pink
		statusEndStyle = lipgloss.NewStyle().Foreground(ac("#15803d", "#50fa7b")).Bold(true)    // green
		metaPriorityStyle = lipgloss.NewStyle().Foreground(ac("#0ea5e9", "#8be9fd")).Bold(true) // cyan
		metaOnHoldStyle = lipgloss.NewStyle().Foreground(ac("#b45309", "#ffb86c")).Bold(true)   // orange
		metaDueStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaScheduleStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaAssignStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaCommentStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaTagStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)

		progressFillBg = ac("#15803d", "#50fa7b")
		progressEmptyBg = ac("#c9c9c1", "#44475a")
		progressFillFg = ac("#f8f8f2", "#282a36")
		progressEmptyFg = ac("#282a36", "#f8f8f2")
	case appearanceGruvbox:
		currentAppearance = appearanceGruvbox
		resetAppearancePaletteToDefaults()

		// Gruvbox-ish (popular warm dark theme).
		// Include gruvbox light + gruvbox dark variants.
		colorSurfaceBg = ac("#fbf1c7", "#282828")
		colorSurfaceFg = ac("#3c3836", "#ebdbb2")
		colorControlBg = ac("#ebdbb2", "#1d2021")
		colorInputBg = ac("#ebdbb2", "#1d2021")
		colorSelectedBg = ac("#d5c4a1", "#3c3836")
		colorSelectedFg = ac("#3c3836", "#ebdbb2")
		colorSelectedBorder = ac("#b57614", "#d79921") // yellow
		colorCardBorder = ac("#d5c4a1", "#3c3836")
		colorMuted = ac("#665c54", "#a89984")
		colorChromeMutedFg = ac("#665c54", "#bdae93")
		colorChromeSubtleFg = ac("#7c6f64", "#928374")
		colorAccent = ac("#076678", "#83a598") // aqua
		colorAccentFg = ac("#fbf1c7", "#282828")
		colorShadow = colorSurfaceBg
		colorCardMetaFg = colorChromeMutedFg
		colorFlashErrorBg = ac("#9d0006", "#cc241d")

		colorModalSurfaceBg = colorControlBg
		colorModalSurfaceFg = colorSurfaceFg
		colorModalHeaderBg = ac("#d5c4a1", "#3c3836")
		colorModalHeaderFg = colorSurfaceFg

		statusNonEndStyle = lipgloss.NewStyle().Foreground(ac("#8f3f71", "#d3869b")).Bold(true) // purple-ish
		statusEndStyle = lipgloss.NewStyle().Foreground(ac("#79740e", "#b8bb26")).Bold(true)    // green
		metaPriorityStyle = lipgloss.NewStyle().Foreground(ac("#076678", "#83a598")).Bold(true) // aqua
		metaOnHoldStyle = lipgloss.NewStyle().Foreground(ac("#b57614", "#fabd2f")).Bold(true)   // yellow
		metaDueStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaScheduleStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaAssignStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaCommentStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaTagStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)

		progressFillBg = colorAccent
		progressEmptyBg = colorCardBorder
		progressFillFg = colorAccentFg
		progressEmptyFg = colorSurfaceFg
	case appearanceSolarized:
		currentAppearance = appearanceSolarized
		resetAppearancePaletteToDefaults()

		// Solarized Light + Solarized Dark.
		colorSurfaceBg = ac("#fdf6e3", "#002b36")
		colorSurfaceFg = ac("#073642", "#839496")
		colorControlBg = ac("#eee8d5", "#073642")
		colorInputBg = ac("#eee8d5", "#073642")
		colorSelectedBg = ac("#eee8d5", "#073642")
		colorSelectedFg = ac("#073642", "#eee8d5")
		colorSelectedBorder = ac("#268bd2", "#268bd2") // blue
		colorCardBorder = ac("#93a1a1", "#586e75")
		colorMuted = ac("#586e75", "#93a1a1")
		colorChromeMutedFg = ac("#586e75", "#93a1a1")
		colorChromeSubtleFg = ac("#93a1a1", "#586e75")
		colorAccent = ac("#268bd2", "#268bd2")
		colorAccentFg = ac("#fdf6e3", "#002b36")
		colorShadow = colorSurfaceBg
		colorCardMetaFg = colorMuted
		colorFlashErrorBg = ac("#dc322f", "#dc322f")

		colorModalSurfaceBg = colorSurfaceBg
		colorModalSurfaceFg = colorSurfaceFg
		colorModalHeaderBg = colorControlBg
		colorModalHeaderFg = colorSurfaceFg

		statusNonEndStyle = lipgloss.NewStyle().Foreground(ac("#d33682", "#d33682")).Bold(true) // magenta
		statusEndStyle = lipgloss.NewStyle().Foreground(ac("#859900", "#859900")).Bold(true)    // green
		metaPriorityStyle = lipgloss.NewStyle().Foreground(ac("#2aa198", "#2aa198")).Bold(true) // cyan
		metaOnHoldStyle = lipgloss.NewStyle().Foreground(ac("#b58900", "#b58900")).Bold(true)   // yellow
		metaDueStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaScheduleStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaAssignStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaCommentStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaTagStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)

		progressFillBg = colorAccent
		progressEmptyBg = colorCardBorder
		progressFillFg = colorAccentFg
		progressEmptyFg = colorSurfaceFg
	case appearanceTerminal:
		currentAppearance = appearanceTerminal
		resetAppearancePaletteToDefaults()
		// Use terminal theme colors (ANSI 0-15) for accents, while avoiding "painted"
		// UI surfaces so the terminal's background shows through.
		colorMuted = lipgloss.ANSIColor(8) // bright black (theme-defined gray)
		colorSurfaceBg = lipgloss.NoColor{}
		colorSurfaceFg = lipgloss.NoColor{}
		colorControlBg = lipgloss.NoColor{}
		colorInputBg = lipgloss.NoColor{}
		colorAccent = lipgloss.ANSIColor(4) // blue
		colorAccentFg = lipgloss.ANSIColor(15)
		colorShadow = lipgloss.NoColor{}

		// Status + metadata accents.
		statusNonEndStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(5)).Bold(true) // magenta
		statusEndStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(2)).Bold(true)    // green
		metaPriorityStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(6)).Bold(true) // cyan
		metaOnHoldStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(3)).Bold(true)   // yellow
		metaDueStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaScheduleStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaAssignStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaCommentStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaTagStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)

		// Progress cookie uses ANSI backgrounds for a theme-aligned bar.
		progressFillBg = lipgloss.ANSIColor(2)  // green
		progressEmptyBg = lipgloss.ANSIColor(8) // gray
		progressFillFg = lipgloss.ANSIColor(15) // bright white
		progressEmptyFg = lipgloss.ANSIColor(7) // white

		// Selection + borders: use theme-defined gray.
		colorSelectedBg = lipgloss.ANSIColor(8)
		colorSelectedFg = lipgloss.NoColor{}
		colorSelectedBorder = lipgloss.ANSIColor(8)
		colorCardBorder = lipgloss.ANSIColor(8)
		// Modals: avoid painting.
		colorModalSurfaceBg = lipgloss.NoColor{}
		colorModalSurfaceFg = lipgloss.NoColor{}
		colorModalHeaderBg = lipgloss.NoColor{}
		colorModalHeaderFg = lipgloss.NoColor{}
	case appearanceNeon:
		currentAppearance = appearanceNeon
		resetAppearancePaletteToDefaults()

		// High-contrast neon palette (explicit surfaces + accents).
		colorSurfaceBg = ac("#ffffff", "#0b1020")
		colorSurfaceFg = ac("#111827", "#f8fafc")
		colorControlBg = ac("#f3f4ff", "#111a33")
		colorInputBg = ac("#eef2ff", "#0d142b")
		colorSelectedBg = ac("#e9d5ff", "#2a1b3d")
		colorSelectedFg = colorSurfaceFg
		colorSelectedBorder = ac("#a100ff", "#ff4fd8")
		colorCardBorder = ac("#d1d5db", "#25324d")
		colorMuted = ac("#4b5563", "#a3adc2")
		colorChromeMutedFg = ac("#374151", "#cbd5e1")
		colorChromeSubtleFg = ac("#6b7280", "#94a3b8")
		colorAccent = ac("#005f87", "#00d7ff")
		colorAccentFg = ac("#ffffff", "#0b1020")
		colorShadow = colorSurfaceBg
		colorCardMetaFg = colorChromeMutedFg
		colorFlashErrorBg = ac("#dc2626", "#ff5555")

		colorModalSurfaceBg = colorControlBg
		colorModalSurfaceFg = colorSurfaceFg
		colorModalHeaderBg = ac("#c4b5fd", "#2a1b3d")
		colorModalHeaderFg = colorSurfaceFg

		statusNonEndStyle = lipgloss.NewStyle().Foreground(ac("#a100ff", "#ff4fd8")).Bold(true)
		statusEndStyle = lipgloss.NewStyle().Foreground(ac("#007a3d", "#3ddc84")).Bold(true)
		metaPriorityStyle = lipgloss.NewStyle().Foreground(ac("#005f87", "#00d7ff")).Bold(true)
		metaOnHoldStyle = lipgloss.NewStyle().Foreground(ac("#af5f00", "#ffaf00")).Bold(true)
		metaDueStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaScheduleStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaAssignStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaCommentStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaTagStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)

		progressFillBg = colorAccent
		progressEmptyBg = colorCardBorder
		progressFillFg = colorAccentFg
		progressEmptyFg = colorSurfaceFg
	case appearancePills:
		currentAppearance = appearancePills
		resetAppearancePaletteToDefaults()

		// "Pills" is a playful style where key metadata gets a pill-shaped highlight, but the
		// rest of the TUI still uses an explicit light/dark palette for consistent contrast.
		colorSurfaceBg = ac("#ffffff", "#0f172a")
		colorSurfaceFg = ac("#0f172a", "#e2e8f0")
		colorControlBg = ac("#f1f5f9", "#111827")
		colorInputBg = ac("#ffffff", "#0b1220")
		colorSelectedBg = ac("#dbeafe", "#1e293b")
		colorSelectedFg = colorSurfaceFg
		colorSelectedBorder = ac("#2563eb", "#60a5fa")
		colorCardBorder = ac("#e2e8f0", "#334155")
		colorMuted = ac("#475569", "#94a3b8")
		colorChromeMutedFg = colorMuted
		colorChromeSubtleFg = ac("#64748b", "#64748b")
		colorAccent = ac("#2563eb", "#60a5fa")
		colorAccentFg = ac("#ffffff", "#0b1220")
		colorShadow = colorSurfaceBg
		colorCardMetaFg = colorChromeMutedFg
		colorFlashErrorBg = ac("#dc2626", "#ef4444")

		colorModalSurfaceBg = colorControlBg
		colorModalSurfaceFg = colorSurfaceFg
		colorModalHeaderBg = ac("#dbeafe", "#1e293b")
		colorModalHeaderFg = colorSurfaceFg

		statusNonEndStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(ac("#0f172a", "#f8fafc")).
			Background(ac("#e9d5ff", "#7c3aed")).
			Bold(true)
		statusEndStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(ac("#0f172a", "#dcfce7")).
			Background(ac("#dcfce7", "#14532d")).
			Bold(true)
		metaPriorityStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(ac("#0f172a", "#fff7ed")).
			Background(ac("#ffedd5", "#c2410c")).
			Bold(true)
		metaOnHoldStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(ac("#0f172a", "#0b1220")).
			Background(ac("#fef3c7", "#f59e0b")).
			Bold(true)
		metaDueStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaScheduleStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaAssignStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaCommentStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)
		metaTagStyle = lipgloss.NewStyle().Foreground(colorChromeMutedFg)

		progressFillBg = colorAccent
		progressEmptyBg = colorCardBorder
		progressFillFg = colorAccentFg
		progressEmptyFg = colorSurfaceFg
	case appearanceMono:
		currentAppearance = appearanceMono
		resetAppearancePaletteToDefaults()
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
		colorModalSurfaceBg = colorSurfaceBg
		colorModalSurfaceFg = colorSurfaceFg
		colorModalHeaderBg = colorControlBg
		colorModalHeaderFg = colorSurfaceFg
	case appearanceCustom:
		if customProfile == nil {
			return
		}
		currentAppearance = appearanceCustom
		resetAppearancePaletteToDefaults()
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

		// Modals inherit from surface + control surfaces.
		colorModalSurfaceBg = colorSurfaceBg
		colorModalSurfaceFg = colorSurfaceFg
		colorModalHeaderBg = colorControlBg
		colorModalHeaderFg = colorSurfaceFg
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
	case appearanceAlabaster:
		return "Alabaster"
	case appearanceDracula, appearanceMidnight:
		return "Dracula"
	case appearanceGruvbox:
		return "Gruvbox"
	case appearanceSolarized, appearancePaper:
		return "Solarized"
	case appearanceTerminal:
		return "Terminal"
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
