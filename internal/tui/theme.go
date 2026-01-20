package tui

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
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
	defaultColorMuted lipgloss.TerminalColor = ac("240", "243") // dark-ish gray on light; soft gray on dark
	colorMuted                               = defaultColorMuted

	// Used for headings/breadcrumbs and other secondary chrome.
	defaultColorChromeMutedFg lipgloss.TerminalColor = ac("240", "245")
	colorChromeMutedFg                               = defaultColorChromeMutedFg
	// Even more muted "display-only" rows (e.g. archived list context rows).
	defaultColorChromeSubtleFg lipgloss.TerminalColor = ac("241", "245")
	colorChromeSubtleFg                               = defaultColorChromeSubtleFg

	// Make the selection highlight more prominent against the surface background.
	// (Surface bg is 255/235, so bump contrast in both light+dark themes.)
	// Match the Alabaster highlight, which reads well across terminals.
	defaultColorSelectedBg                        = ac("#e9e9e9", "#262626")
	defaultColorSelectedFg                        = ac("235", "255")
	colorSelectedBg        lipgloss.TerminalColor = defaultColorSelectedBg
	colorSelectedFg        lipgloss.TerminalColor = defaultColorSelectedFg
	// Used for "selected" borders (cards): very dark on light terminals, very bright on dark terminals.
	defaultColorSelectedBorder                        = ac("232", "255")
	colorSelectedBorder        lipgloss.TerminalColor = defaultColorSelectedBorder
	// Used for unselected borders (cards): softer on light terminals so selection stands out.
	defaultColorCardBorder                        = ac("250", "243")
	colorCardBorder        lipgloss.TerminalColor = defaultColorCardBorder

	defaultColorSurfaceBg lipgloss.TerminalColor = ac("255", "235")
	colorSurfaceBg                               = defaultColorSurfaceBg
	defaultColorSurfaceFg lipgloss.TerminalColor = ac("235", "252")
	colorSurfaceFg                               = defaultColorSurfaceFg

	// Slightly elevated surface for controls/inputs so they remain visible on light terminals.
	defaultColorControlBg lipgloss.TerminalColor = ac("252", "235")
	colorControlBg                               = defaultColorControlBg
	defaultColorInputBg   lipgloss.TerminalColor = ac("254", "234")
	colorInputBg                                 = defaultColorInputBg

	defaultColorAccent lipgloss.TerminalColor = ac("27", "62") // blue
	colorAccent                               = defaultColorAccent
	// Foreground for text rendered on top of colorAccent backgrounds (e.g. input cursor).
	defaultColorAccentFg lipgloss.TerminalColor = ac("255", "235")
	colorAccentFg                               = defaultColorAccentFg
	// Avoid gray "shadow blocks" on light terminals; keep shadow only for dark theme.
	defaultColorShadow lipgloss.TerminalColor = ac("255", "236")
	colorShadow                               = defaultColorShadow

	// Card metadata (small secondary labels inside cards).
	defaultColorCardMetaFg lipgloss.TerminalColor = ac("238", "250")
	colorCardMetaFg                               = defaultColorCardMetaFg

	// Short-lived row flash feedback (e.g. permission denied).
	defaultColorFlashErrorBg lipgloss.TerminalColor = ac("196", "160") // red
	colorFlashErrorBg                               = defaultColorFlashErrorBg

	// Modal colors; by default these track the surface.
	defaultColorModalSurfaceBg lipgloss.TerminalColor = defaultColorSurfaceBg
	colorModalSurfaceBg                               = defaultColorModalSurfaceBg
	defaultColorModalSurfaceFg lipgloss.TerminalColor = defaultColorSurfaceFg
	colorModalSurfaceFg                               = defaultColorModalSurfaceFg
	defaultColorModalHeaderBg  lipgloss.TerminalColor = defaultColorControlBg
	colorModalHeaderBg                                = defaultColorModalHeaderBg
	defaultColorModalHeaderFg  lipgloss.TerminalColor = defaultColorSurfaceFg
	colorModalHeaderFg                                = defaultColorModalHeaderFg
)

func styleMuted() lipgloss.Style {
	return faintIfDark(lipgloss.NewStyle().Foreground(colorMuted))
}

// applyColorProfilePreference sets Lip Gloss's color profile for the interactive TUI.
//
// Note: termenv.EnvColorProfile respects CLICOLOR/CLICOLOR_FORCE, which is useful for
// non-interactive CLI output but can accidentally disable colors in a TUI. For the TUI,
// we only honor NO_COLOR and otherwise follow the terminal's capabilities.
func applyColorProfilePreference() {
	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		lipgloss.SetColorProfile(termenv.Ascii)
		return
	}

	// Start from termenv's best guess.
	profile := termenv.ColorProfile()

	// Heuristics:
	// - If TERM/COLORTERM indicate stronger support than the detector reports, trust
	//   the env. This helps terminals like macOS Terminal.app where color probing
	//   can under-report (leading to degraded "gray" colors).
	term := strings.ToLower(strings.TrimSpace(os.Getenv("TERM")))
	colorterm := strings.ToLower(strings.TrimSpace(os.Getenv("COLORTERM")))
	if strings.Contains(colorterm, "truecolor") || strings.Contains(colorterm, "24bit") {
		if profile != termenv.Ascii {
			profile = termenv.TrueColor
		}
	} else if strings.Contains(term, "256color") {
		if profile == termenv.Ascii {
			profile = termenv.ANSI256
		} else if profile == termenv.ANSI {
			profile = termenv.ANSI256
		}
	}

	lipgloss.SetColorProfile(profile)
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
			return
		}
	}

	// macOS Terminal.app often doesn't set COLORFGBG, and background probing can be unreliable.
	// As a fallback, use the OS appearance (Light vs Dark) when available.
	if runtime.GOOS == "darwin" {
		if dark, ok := macOSHasDarkAppearance(); ok {
			lipgloss.SetHasDarkBackground(dark)
			return
		}
	}
}

func macOSHasDarkAppearance() (dark bool, ok bool) {
	// `defaults read -g AppleInterfaceStyle` prints "Dark" in dark mode and returns exit status 1
	// in light mode (key missing).
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	out, err := exec.CommandContext(ctx, "defaults", "read", "-g", "AppleInterfaceStyle").CombinedOutput()
	if ctx.Err() != nil {
		return false, false
	}
	if err == nil {
		return strings.Contains(strings.ToLower(string(out)), "dark"), true
	}
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		return false, true
	}
	return false, false
}
