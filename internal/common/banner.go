package common

import (
	"fmt"
	"os"
	"strings"

	"github.com/ternarybob/banner"
)

// borderChars returns the double-style border characters.
func borderChars() banner.BorderChars {
	// Use the banner library's exported struct directly.
	return banner.BorderChars{
		TopLeft:     "\u2554", // ╔
		TopRight:    "\u2557", // ╗
		BottomLeft:  "\u255a", // ╚
		BottomRight: "\u255d", // ╝
		Horizontal:  "\u2550", // ═
		Vertical:    "\u2551", // ║
		LeftJoin:    "\u2560", // ╠
		RightJoin:   "\u2563", // ╣
	}
}

// applyColor wraps text with an ANSI color code and reset.
func applyColor(color, text string) string {
	if color != "" {
		return color + text + banner.ColorReset
	}
	return text
}

// fmtTopLine builds the top border line for the given width.
func fmtTopLine(bc banner.BorderChars, width int) string {
	inner := width - 2
	return bc.TopLeft + strings.Repeat(bc.Horizontal, inner) + bc.TopRight
}

// fmtBottomLine builds the bottom border line for the given width.
func fmtBottomLine(bc banner.BorderChars, width int) string {
	inner := width - 2
	return bc.BottomLeft + strings.Repeat(bc.Horizontal, inner) + bc.BottomRight
}

// fmtSeparator builds a separator line for the given width.
func fmtSeparator(bc banner.BorderChars, width int) string {
	inner := width - 2
	return bc.LeftJoin + strings.Repeat(bc.Horizontal, inner) + bc.RightJoin
}

// stderrLine writes a colorized border line to stderr.
func stderrLine(borderColor string, borderLine string) {
	fmt.Fprintf(os.Stderr, "%s\n", applyColor(borderColor, borderLine))
}

// stderrTextLine writes a line with colored border and colored text to stderr.
func stderrTextLine(borderColor, textColor string, bc banner.BorderChars, width int, text string, centered bool, bold bool) {
	inner := width - 4
	var padded string
	if centered {
		if len(text) > inner {
			text = text[:inner]
		}
		total := inner - len(text)
		left := total / 2
		right := total - left
		padded = strings.Repeat(" ", left) + text + strings.Repeat(" ", right)
	} else {
		if len(text) > inner {
			text = text[:inner]
		}
		padded = text + strings.Repeat(" ", inner-len(text))
	}

	bPrefix := applyColor(borderColor, bc.Vertical)
	bSuffix := applyColor(borderColor, bc.Vertical)

	tc := textColor
	if bold {
		tc = banner.ColorBold + tc
	}
	content := applyColor(tc, " "+padded+" ")

	fmt.Fprintf(os.Stderr, "%s%s%s\n", bPrefix, content, bSuffix)
}

// stderrKVLine writes a key-value line with colored border and text to stderr.
func stderrKVLine(borderColor, textColor string, bc banner.BorderChars, width int, key, value string, padding int, bold bool) {
	formatted := fmt.Sprintf("%-*s %s", padding, key+":", value)
	stderrTextLine(borderColor, textColor, bc, width, formatted, false, bold)
}

// PrintBanner displays the application startup banner to stderr.
func PrintBanner(config *Config, logger *Logger) {
	version := GetVersion()
	build := GetBuild()
	commit := GetGitCommit()
	serviceURL := fmt.Sprintf("http://%s:%d", config.Server.Host, config.Server.Port)
	storageAddr := config.Storage.Address

	bc := borderChars()
	borderColor := banner.ColorCyan
	textColor := banner.ColorWhite
	width := 70
	bold := true

	fmt.Fprintf(os.Stderr, "\n")
	stderrLine(borderColor, fmtTopLine(bc, width))
	stderrTextLine(borderColor, textColor, bc, width, "VIRE", true, bold)
	stderrTextLine(borderColor, textColor, bc, width, "Investment Research & Portfolio Analysis", true, bold)
	stderrLine(borderColor, fmtSeparator(bc, width))
	stderrKVLine(borderColor, textColor, bc, width, "Version", version, 15, bold)
	stderrKVLine(borderColor, textColor, bc, width, "Build", build, 15, bold)
	stderrKVLine(borderColor, textColor, bc, width, "Commit", commit, 15, bold)
	stderrKVLine(borderColor, textColor, bc, width, "Environment", config.Environment, 15, bold)
	stderrKVLine(borderColor, textColor, bc, width, "Service URL", serviceURL, 15, bold)
	stderrKVLine(borderColor, textColor, bc, width, "Storage", storageAddr, 15, bold)
	stderrLine(borderColor, fmtBottomLine(bc, width))
	fmt.Fprintf(os.Stderr, "\n")

	// Log structured startup information through arbor
	logger.Info().
		Str("version", version).
		Str("build", build).
		Str("commit", commit).
		Str("environment", config.Environment).
		Str("service_url", serviceURL).
		Str("storage_address", storageAddr).
		Msg("Application started")
}

// PrintShutdownBanner displays the application shutdown banner to stderr.
func PrintShutdownBanner(logger *Logger) {
	bc := borderChars()
	borderColor := banner.ColorCyan
	textColor := banner.ColorWhite
	width := 42
	bold := true

	fmt.Fprintf(os.Stderr, "\n")
	stderrLine(borderColor, fmtTopLine(bc, width))
	stderrTextLine(borderColor, textColor, bc, width, "SHUTTING DOWN", true, bold)
	stderrTextLine(borderColor, textColor, bc, width, "VIRE", true, bold)
	stderrLine(borderColor, fmtBottomLine(bc, width))
	fmt.Fprintf(os.Stderr, "\n")

	// Log shutdown through arbor
	logger.Info().Msg("Application shutting down")
}
