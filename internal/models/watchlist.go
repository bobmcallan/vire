package models

import (
	"fmt"
	"strings"
	"time"
)

// WatchlistVerdict categorizes stock assessment outcomes
type WatchlistVerdict string

const (
	WatchlistVerdictPass  WatchlistVerdict = "PASS"
	WatchlistVerdictWatch WatchlistVerdict = "WATCH"
	WatchlistVerdictFail  WatchlistVerdict = "FAIL"
)

// ValidWatchlistVerdict returns true if the verdict is one of PASS/WATCH/FAIL
func ValidWatchlistVerdict(v WatchlistVerdict) bool {
	switch v {
	case WatchlistVerdictPass, WatchlistVerdictWatch, WatchlistVerdictFail:
		return true
	}
	return false
}

// WatchlistItem represents a stock verdict on the watchlist
type WatchlistItem struct {
	Ticker     string           `json:"ticker"`      // e.g. "SGI.AU"
	Name       string           `json:"name"`        // Company name
	Verdict    WatchlistVerdict `json:"verdict"`     // PASS/WATCH/FAIL
	Reason     string           `json:"reason"`      // Summary reasoning
	KeyMetrics string           `json:"key_metrics"` // Revenue, PE, etc snapshot
	Notes      string           `json:"notes"`       // Additional notes
	ReviewedAt time.Time        `json:"reviewed_at"` // When verdict was last changed
	CreatedAt  time.Time        `json:"created_at"`  // First added
	UpdatedAt  time.Time        `json:"updated_at"`  // Last modified
}

// PortfolioWatchlist is a versioned collection of stock verdicts
type PortfolioWatchlist struct {
	PortfolioName string          `json:"portfolio_name"`
	Version       int             `json:"version"`
	Items         []WatchlistItem `json:"items"`
	Notes         string          `json:"notes,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// FindByTicker returns the item and index for a given ticker, or -1 if not found
func (w *PortfolioWatchlist) FindByTicker(ticker string) (*WatchlistItem, int) {
	for i, item := range w.Items {
		if strings.EqualFold(item.Ticker, ticker) {
			return &w.Items[i], i
		}
	}
	return nil, -1
}

// ToMarkdown renders the watchlist as a readable markdown document grouped by verdict
func (w *PortfolioWatchlist) ToMarkdown() string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# Watchlist: %s\n\n", w.PortfolioName))

	if len(w.Items) == 0 {
		b.WriteString("No watchlist items.\n\n")
	} else {
		pass := make([]WatchlistItem, 0)
		watch := make([]WatchlistItem, 0)
		fail := make([]WatchlistItem, 0)

		for _, item := range w.Items {
			switch item.Verdict {
			case WatchlistVerdictPass:
				pass = append(pass, item)
			case WatchlistVerdictWatch:
				watch = append(watch, item)
			case WatchlistVerdictFail:
				fail = append(fail, item)
			}
		}

		if len(pass) > 0 {
			b.WriteString("## PASS\n\n")
			for _, item := range pass {
				writeWatchlistItem(&b, item)
			}
			b.WriteString("\n")
		}

		if len(watch) > 0 {
			b.WriteString("## WATCH\n\n")
			for _, item := range watch {
				writeWatchlistItem(&b, item)
			}
			b.WriteString("\n")
		}

		if len(fail) > 0 {
			b.WriteString("## FAIL\n\n")
			for _, item := range fail {
				writeWatchlistItem(&b, item)
			}
			b.WriteString("\n")
		}
	}

	if w.Notes != "" {
		b.WriteString("## Notes\n\n")
		b.WriteString(w.Notes)
		b.WriteString("\n\n")
	}

	// Metadata
	b.WriteString("---\n\n")
	b.WriteString(fmt.Sprintf("Version %d | %d items", w.Version, len(w.Items)))
	if !w.CreatedAt.IsZero() {
		b.WriteString(fmt.Sprintf(" | Created %s", w.CreatedAt.Format("2006-01-02")))
	}
	if !w.UpdatedAt.IsZero() {
		b.WriteString(fmt.Sprintf(" | Updated %s", w.UpdatedAt.Format("2006-01-02")))
	}
	b.WriteString("\n")

	return b.String()
}

func writeWatchlistItem(b *strings.Builder, item WatchlistItem) {
	icon := "~"
	switch item.Verdict {
	case WatchlistVerdictPass:
		icon = "+"
	case WatchlistVerdictWatch:
		icon = "?"
	case WatchlistVerdictFail:
		icon = "-"
	}

	b.WriteString(fmt.Sprintf("- [%s] **%s**", icon, item.Ticker))
	if item.Name != "" {
		b.WriteString(fmt.Sprintf(" — %s", item.Name))
	}
	if !item.ReviewedAt.IsZero() {
		b.WriteString(fmt.Sprintf(" (reviewed %s)", item.ReviewedAt.Format("2006-01-02")))
	}
	b.WriteString("\n")

	if item.Reason != "" {
		b.WriteString(fmt.Sprintf("  - %s\n", item.Reason))
	}
	if item.KeyMetrics != "" {
		b.WriteString(fmt.Sprintf("  - Metrics: %s\n", item.KeyMetrics))
	}
	if item.Notes != "" {
		b.WriteString(fmt.Sprintf("  - *%s*\n", item.Notes))
	}
}
