package tui

import (
        "os"
        "strconv"
        "strings"

        "github.com/charmbracelet/lipgloss"
)

// Theme/palette helpers.
//
// The TUI must remain readable on both light and dark terminal backgrounds.
// We use lipgloss.AdaptiveColor where possible and only apply "faint" styling
// on dark backgrounds (faint text on light terminals often becomes illegible).

func ac(light, dark string) lipgloss.AdaptiveColor {
        return lipgloss.AdaptiveColor{Light: light, Dark: dark}
}

func faintIfDark(st lipgloss.Style) lipgloss.Style {
        if lipgloss.HasDarkBackground() {
                return st.Faint(true)
        }
        return st
}

// Common semantic colors used across the TUI.
var (
        colorMuted = ac("240", "243") // dark-ish gray on light; soft gray on dark

        // Make the selection highlight more prominent against the surface background.
        // (Surface bg is 255/235, so bump contrast in both light+dark themes.)
        colorSelectedBg = ac("250", "239")
        colorSelectedFg = ac("235", "255")

        colorSurfaceBg = ac("255", "235")
        colorSurfaceFg = ac("235", "252")

        // Slightly elevated surface for controls/inputs so they remain visible on light terminals.
        colorControlBg = ac("252", "235")
        colorInputBg   = ac("254", "234")

        colorAccent = ac("27", "62") // blue
        // Avoid gray "shadow blocks" on light terminals; keep shadow only for dark theme.
        colorShadow = ac("255", "236")
)

func styleMuted() lipgloss.Style {
        return faintIfDark(lipgloss.NewStyle().Foreground(colorMuted))
}

// applyThemePreference configures Lip Gloss's background detection.
//
// Some terminals don't reliably report their background, which can cause
// lipgloss.AdaptiveColor to pick the wrong variant (e.g. dark palette on a light theme).
//
// Priority:
// 1) CLARITY_TUI_THEME=light|dark|auto
// 2) CLARITY_TUI_DARKBG=true|false
// 3) COLORFGBG heuristic (common in terminals; format like "15;0" = fg;bg)
func applyThemePreference() {
        if v := strings.TrimSpace(os.Getenv("CLARITY_TUI_THEME")); v != "" {
                switch strings.ToLower(v) {
                case "light":
                        lipgloss.SetHasDarkBackground(false)
                        return
                case "dark":
                        lipgloss.SetHasDarkBackground(true)
                        return
                case "auto":
                        // fallthrough to heuristics/default
                default:
                        // Unknown value: ignore.
                }
        }

        if v := strings.TrimSpace(os.Getenv("CLARITY_TUI_DARKBG")); v != "" {
                if b, err := strconv.ParseBool(v); err == nil {
                        lipgloss.SetHasDarkBackground(b)
                        return
                }
        }

        // Heuristic: COLORFGBG is often "fg;bg" (sometimes more segments). Use last segment as bg.
        if v := strings.TrimSpace(os.Getenv("COLORFGBG")); v != "" {
                parts := strings.Split(v, ";")
                bgStr := strings.TrimSpace(parts[len(parts)-1])
                if bg, err := strconv.Atoi(bgStr); err == nil {
                        // Treat "lighter" backgrounds as non-dark. This is heuristic, but better than
                        // consistently choosing the wrong palette.
                        lipgloss.SetHasDarkBackground(bg < 7)
                }
        }
}
