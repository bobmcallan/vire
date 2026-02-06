package common

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Version variables injected at build time via ldflags
var (
	Version   = "dev"
	Build     = "unknown"
	GitCommit = "unknown"
)

// GetVersion returns the semantic version string
func GetVersion() string {
	return Version
}

// GetBuild returns the build timestamp
func GetBuild() string {
	return Build
}

// GetGitCommit returns the short git commit hash
func GetGitCommit() string {
	return GitCommit
}

// GetFullVersion returns a formatted version string with all build info
func GetFullVersion() string {
	return fmt.Sprintf("%s (build: %s, commit: %s)", Version, Build, GitCommit)
}

// LoadVersionFromFile attempts to load version info from a .version file
// in the same directory as the binary. Values loaded from file are only used
// as fallbacks when ldflags weren't provided (i.e. still at defaults).
func LoadVersionFromFile() {
	exe, err := os.Executable()
	if err != nil {
		return
	}

	versionFile := filepath.Join(filepath.Dir(exe), ".version")
	f, err := os.Open(versionFile)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "version":
			if Version == "dev" {
				Version = val
			}
		case "build":
			if Build == "unknown" {
				Build = val
			}
		}
	}
}
