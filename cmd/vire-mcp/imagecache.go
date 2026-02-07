package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bobmccarthy/vire/internal/common"
)

// ImageCache manages server-side caching of rendered chart images.
// Images are stored on disk and served via an HTTP endpoint.
type ImageCache struct {
	dir    string
	port   int
	logger *common.Logger
}

// NewImageCache creates an ImageCache that stores images under dir.
// The port is used to construct full URLs for external access.
func NewImageCache(dir string, port int, logger *common.Logger) *ImageCache {
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Warn().Err(err).Str("dir", dir).Msg("Failed to create image cache directory")
	}
	return &ImageCache{dir: dir, port: port, logger: logger}
}

// Put writes image data to disk and returns the URL path (e.g. /images/{name}).
// It cleans up older images with the same portfolio prefix, keeping only the latest.
func (c *ImageCache) Put(name string, data []byte) (string, error) {
	c.cleanOld(name)

	path := filepath.Join(c.dir, name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("write image %s: %w", name, err)
	}

	c.logger.Debug().Str("name", name).Int("bytes", len(data)).Msg("Cached chart image")
	return c.URL(name), nil
}

// Get reads a cached image from disk.
func (c *ImageCache) Get(name string) ([]byte, bool) {
	data, err := os.ReadFile(filepath.Join(c.dir, name))
	if err != nil {
		return nil, false
	}
	return data, true
}

// URL returns the relative URL path for a cached image.
func (c *ImageCache) URL(name string) string {
	return "/images/" + name
}

// FullURL returns an absolute HTTP URL for a cached image.
func (c *ImageCache) FullURL(name string) string {
	return fmt.Sprintf("http://localhost:%d/images/%s", c.port, name)
}

// Handler returns an http.Handler that serves cached images.
func (c *ImageCache) Handler() http.Handler {
	return http.StripPrefix("/images/", http.FileServer(http.Dir(c.dir)))
}

// ImageName generates a cache filename for a portfolio growth chart.
func ImageName(portfolio string) string {
	ts := time.Now().Format("20060102-1504")
	return strings.ToLower(portfolio) + "-growth-" + ts + ".png"
}

// cleanOld removes older images with the same portfolio prefix.
func (c *ImageCache) cleanOld(name string) {
	// Extract portfolio prefix (e.g. "smsf-growth-" from "smsf-growth-20260207-1348.png")
	prefix := ""
	if idx := strings.LastIndex(name, "-growth-"); idx >= 0 {
		prefix = name[:idx+len("-growth-")]
	}
	if prefix == "" {
		return
	}

	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}

	// Collect matching files, sort by name (timestamp order), remove all but skip current
	var matches []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) && e.Name() != name {
			matches = append(matches, e.Name())
		}
	}
	sort.Strings(matches)

	for _, old := range matches {
		if err := os.Remove(filepath.Join(c.dir, old)); err == nil {
			c.logger.Debug().Str("file", old).Msg("Cleaned old cached image")
		}
	}
}
