package common

import (
	"fmt"
	"os"
	"strings"

	"github.com/ternarybob/banner"
)

// PrintBanner displays the application startup banner to stderr.
func PrintBanner(config *Config, logger *Logger) {
	version := GetVersion()
	build := GetBuild()
	commit := GetGitCommit()
	serviceURL := fmt.Sprintf("http://%s:%d", config.Server.Host, config.Server.Port)
	storageAddr := config.Storage.Address

	lineColor := banner.ColorCyan
	textColor := banner.ColorBold + banner.ColorWhite
	width := 70
	hr := lineColor + strings.Repeat("═", width) + banner.ColorReset

	art := []string{
		` 888     888 8888888 8888888b.  8888888888`,
		` 888     888   888   888   Y88b 888`,
		` 888     888   888   888    888 888`,
		` Y88b   d88P   888   888   d88P 8888888`,
		`  Y88b d88P    888   8888888P'  888`,
		`   Y88o88P     888   888 T88b   888`,
		`    Y888P      888   888  T88b  888`,
		`     Y8P     8888888 888   T88b 8888888888`,
	}

	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "%s\n", hr)
	fmt.Fprintf(os.Stderr, "\n")
	for _, line := range art {
		fmt.Fprintf(os.Stderr, "%s%s%s\n", textColor, line, banner.ColorReset)
	}
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "%s  Investment Research & Portfolio Analysis%s\n", textColor, banner.ColorReset)
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "%s\n", hr)
	fmt.Fprintf(os.Stderr, "\n")

	kvPad := 16
	kvLines := [][2]string{
		{"Version", version},
		{"Build", build},
		{"Commit", commit},
		{"Environment", config.Environment},
		{"Service URL", serviceURL},
		{"Storage", storageAddr},
	}
	for _, kv := range kvLines {
		fmt.Fprintf(os.Stderr, "%s  %-*s %s%s\n", textColor, kvPad, kv[0], kv[1], banner.ColorReset)
	}

	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "%s\n", hr)
	fmt.Fprintf(os.Stderr, "\n")

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
	lineColor := banner.ColorCyan
	textColor := banner.ColorBold + banner.ColorWhite
	width := 42
	hr := lineColor + strings.Repeat("═", width) + banner.ColorReset

	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "%s\n", hr)
	fmt.Fprintf(os.Stderr, "%s  VIRE — SHUTTING DOWN%s\n", textColor, banner.ColorReset)
	fmt.Fprintf(os.Stderr, "%s\n", hr)
	fmt.Fprintf(os.Stderr, "\n")

	logger.Info().Msg("Application shutting down")
}
