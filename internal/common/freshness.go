// Package common provides shared utilities for Vire
package common

import "time"

// Freshness TTLs for data components
const (
	FreshnessTodayBar     = 1 * time.Hour
	FreshnessFundamentals = 7 * 24 * time.Hour // 7 days
	FreshnessNews         = 6 * time.Hour
	FreshnessSignals      = 1 * time.Hour // matches today's bar
	FreshnessReport       = 1 * time.Hour
	FreshnessNewsIntel    = 30 * 24 * time.Hour // 30 days — slow information
	FreshnessFilings      = 30 * 24 * time.Hour // 30 days — announcements don't change
	FreshnessFilingsIntel = 90 * 24 * time.Hour // 90 days — only re-summarize when new filings
)

// IsFresh returns true if the given timestamp is within the TTL
func IsFresh(updated time.Time, ttl time.Duration) bool {
	if updated.IsZero() {
		return false
	}
	return time.Since(updated) < ttl
}
