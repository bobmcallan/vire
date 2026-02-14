// Package common provides shared utilities for Vire
package common

import "time"

// Freshness TTLs for data components, organized in three tiers:
//
// Tier 1 — Fast / Real-time: Quotes and live prices. Short TTL, always
// re-fetched on access. Never cached long-term.
//
// Tier 2 — Source data: EOD bars, fundamentals, news articles, filing lists.
// Time-based TTL. Re-fetched when stale. These are upstream facts that don't
// change once published.
//
// Tier 3 — Derived / Intelligence: Filing summaries, company timelines, news
// intelligence, signals. Rebuilt when source data changes or schema version
// bumps. Tagged with DataVersion on MarketData for schema-aware invalidation.
const (
	FreshnessTodayBar      = 1 * time.Hour
	FreshnessFundamentals  = 7 * 24 * time.Hour // 7 days
	FreshnessNews          = 6 * time.Hour
	FreshnessSignals       = 1 * time.Hour // matches today's bar
	FreshnessReport        = 1 * time.Hour
	FreshnessPortfolio     = 30 * time.Minute
	FreshnessNewsIntel     = 30 * 24 * time.Hour // 30 days — slow information
	FreshnessFilings       = 30 * 24 * time.Hour // 30 days — announcements don't change
	FreshnessTimeline      = 7 * 24 * time.Hour  // 7 days — rebuild when new summaries added or periodically
	FreshnessRealTimeQuote = 15 * time.Minute    // real-time quote data from EODHD
)

// IsFresh returns true if the given timestamp is within the TTL
func IsFresh(updated time.Time, ttl time.Duration) bool {
	if updated.IsZero() {
		return false
	}
	return time.Since(updated) < ttl
}
