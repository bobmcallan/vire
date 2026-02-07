// Package models defines data structures for Vire
package models

import (
	"encoding/gob"
	"fmt"
	"strings"
	"time"
)

func init() {
	gob.Register(PortfolioStrategy{})
	gob.Register(RiskAppetite{})
	gob.Register(TargetReturns{})
	gob.Register(IncomeRequirements{})
	gob.Register(SectorPreferences{})
	gob.Register(PositionSizing{})
	gob.Register(ReferenceStrategy{})
}

// AccountType categorizes portfolio accounts
type AccountType string

const (
	AccountTypeSMSF    AccountType = "smsf"    // Self-managed super fund
	AccountTypeTrading AccountType = "trading" // Standard trading account
)

// DefaultDisclaimer is pre-populated on new strategies.
const DefaultDisclaimer = "This portfolio strategy is a personal planning document and does not constitute financial advice. Always consult a licensed financial adviser before making investment decisions."

// PortfolioStrategy captures the investment strategy for a portfolio.
// Keyed by portfolio name (one strategy per portfolio).
type PortfolioStrategy struct {
	PortfolioName       string              `json:"portfolio_name" badgerhold:"key"`
	Version             int                 `json:"version"`             // Auto-incremented on save
	AccountType         AccountType         `json:"account_type"`        // AccountTypeSMSF or AccountTypeTrading
	InvestmentUniverse  []string            `json:"investment_universe"` // Exchange codes matching ticker suffixes: "AU", "US", etc.
	RiskAppetite        RiskAppetite        `json:"risk_appetite"`
	TargetReturns       TargetReturns       `json:"target_returns"`
	IncomeRequirements  IncomeRequirements  `json:"income_requirements"`
	SectorPreferences   SectorPreferences   `json:"sector_preferences"`
	PositionSizing      PositionSizing      `json:"position_sizing"`
	ReferenceStrategies []ReferenceStrategy `json:"reference_strategies"` // Named strategies displayed in ToMarkdown(), not used in AI prompts
	RebalanceFrequency  string              `json:"rebalance_frequency"`  // "monthly", "quarterly", "annually"
	Notes               string              `json:"notes"`                // Free-form markdown
	Disclaimer          string              `json:"disclaimer"`           // "Not financial advice" disclaimer
	CreatedAt           time.Time           `json:"created_at"`
	UpdatedAt           time.Time           `json:"updated_at"`
	LastReviewedAt      time.Time           `json:"last_reviewed_at"` // When strategy was last used in a review
}

// RiskAppetite defines the risk tolerance for a portfolio strategy
type RiskAppetite struct {
	Level          string  `json:"level"`            // "conservative", "moderate", "aggressive"
	MaxDrawdownPct float64 `json:"max_drawdown_pct"` // Maximum acceptable drawdown percentage
	Description    string  `json:"description"`
}

// TargetReturns defines the return objectives for a portfolio strategy
type TargetReturns struct {
	AnnualPct float64 `json:"annual_pct"`
	Timeframe string  `json:"timeframe"` // e.g. "3-5 years"
}

// IncomeRequirements defines dividend/income targets for a portfolio strategy
type IncomeRequirements struct {
	DividendYieldPct float64 `json:"dividend_yield_pct"`
	Description      string  `json:"description"`
}

// SectorPreferences defines preferred and excluded sectors
type SectorPreferences struct {
	Preferred []string `json:"preferred"`
	Excluded  []string `json:"excluded"`
}

// PositionSizing defines concentration limits for a portfolio strategy
type PositionSizing struct {
	MaxPositionPct float64 `json:"max_position_pct"` // Max single position %
	MaxSectorPct   float64 `json:"max_sector_pct"`   // Max sector %
}

// ReferenceStrategy is a named investment approach referenced in the strategy document
type ReferenceStrategy struct {
	Name        string `json:"name"`        // e.g. "dividend growth", "value investing", "momentum"
	Description string `json:"description"` // For display in ToMarkdown() only, not used in AI prompts
}

// StrategyWarning represents a devil's advocate challenge to a strategy setting
type StrategyWarning struct {
	Severity string `json:"severity"` // "high", "medium", "low"
	Field    string `json:"field"`    // Which field(s) triggered the warning
	Message  string `json:"message"`  // Human-readable warning
}

// ToMarkdown renders the strategy as a readable markdown document.
func (s *PortfolioStrategy) ToMarkdown() string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# Portfolio Strategy: %s\n\n", s.PortfolioName))

	// Account & Universe
	if s.AccountType != "" {
		b.WriteString(fmt.Sprintf("**Account Type:** %s\n", string(s.AccountType)))
	}
	if len(s.InvestmentUniverse) > 0 {
		b.WriteString(fmt.Sprintf("**Investment Universe:** %s\n", strings.Join(s.InvestmentUniverse, ", ")))
	}
	b.WriteString("\n")

	// Risk Appetite
	b.WriteString("## Risk Appetite\n\n")
	if s.RiskAppetite.Level != "" {
		b.WriteString(fmt.Sprintf("- **Level:** %s\n", s.RiskAppetite.Level))
	}
	if s.RiskAppetite.MaxDrawdownPct > 0 {
		b.WriteString(fmt.Sprintf("- **Max Drawdown:** %.1f%%\n", s.RiskAppetite.MaxDrawdownPct))
	}
	if s.RiskAppetite.Description != "" {
		b.WriteString(fmt.Sprintf("- %s\n", s.RiskAppetite.Description))
	}
	b.WriteString("\n")

	// Target Returns
	b.WriteString("## Target Returns\n\n")
	if s.TargetReturns.AnnualPct > 0 {
		b.WriteString(fmt.Sprintf("- **Annual Target:** %.1f%%\n", s.TargetReturns.AnnualPct))
	}
	if s.TargetReturns.Timeframe != "" {
		b.WriteString(fmt.Sprintf("- **Timeframe:** %s\n", s.TargetReturns.Timeframe))
	}
	b.WriteString("\n")

	// Income Requirements
	if s.IncomeRequirements.DividendYieldPct > 0 || s.IncomeRequirements.Description != "" {
		b.WriteString("## Income Requirements\n\n")
		if s.IncomeRequirements.DividendYieldPct > 0 {
			b.WriteString(fmt.Sprintf("- **Dividend Yield Target:** %.1f%%\n", s.IncomeRequirements.DividendYieldPct))
		}
		if s.IncomeRequirements.Description != "" {
			b.WriteString(fmt.Sprintf("- %s\n", s.IncomeRequirements.Description))
		}
		b.WriteString("\n")
	}

	// Sector Preferences
	if len(s.SectorPreferences.Preferred) > 0 || len(s.SectorPreferences.Excluded) > 0 {
		b.WriteString("## Sector Preferences\n\n")
		if len(s.SectorPreferences.Preferred) > 0 {
			b.WriteString(fmt.Sprintf("- **Preferred:** %s\n", strings.Join(s.SectorPreferences.Preferred, ", ")))
		}
		if len(s.SectorPreferences.Excluded) > 0 {
			b.WriteString(fmt.Sprintf("- **Excluded:** %s\n", strings.Join(s.SectorPreferences.Excluded, ", ")))
		}
		b.WriteString("\n")
	}

	// Position Sizing
	b.WriteString("## Position Sizing\n\n")
	if s.PositionSizing.MaxPositionPct > 0 {
		b.WriteString(fmt.Sprintf("- **Max Single Position:** %.1f%%\n", s.PositionSizing.MaxPositionPct))
	}
	if s.PositionSizing.MaxSectorPct > 0 {
		b.WriteString(fmt.Sprintf("- **Max Sector Allocation:** %.1f%%\n", s.PositionSizing.MaxSectorPct))
	}
	b.WriteString("\n")

	// Reference Strategies
	if len(s.ReferenceStrategies) > 0 {
		b.WriteString("## Reference Strategies\n\n")
		for _, rs := range s.ReferenceStrategies {
			if rs.Description != "" {
				b.WriteString(fmt.Sprintf("- **%s:** %s\n", rs.Name, rs.Description))
			} else {
				b.WriteString(fmt.Sprintf("- %s\n", rs.Name))
			}
		}
		b.WriteString("\n")
	}

	// Rebalancing
	if s.RebalanceFrequency != "" {
		b.WriteString(fmt.Sprintf("**Rebalancing:** %s\n\n", s.RebalanceFrequency))
	}

	// Notes
	if s.Notes != "" {
		b.WriteString("## Notes\n\n")
		b.WriteString(s.Notes)
		b.WriteString("\n\n")
	}

	// Disclaimer
	if s.Disclaimer != "" {
		b.WriteString("---\n\n")
		b.WriteString(fmt.Sprintf("*%s*\n\n", s.Disclaimer))
	}

	// Metadata
	b.WriteString("---\n\n")
	b.WriteString(fmt.Sprintf("Version %d", s.Version))
	if !s.CreatedAt.IsZero() {
		b.WriteString(fmt.Sprintf(" | Created %s", s.CreatedAt.Format("2006-01-02")))
	}
	if !s.UpdatedAt.IsZero() {
		b.WriteString(fmt.Sprintf(" | Updated %s", s.UpdatedAt.Format("2006-01-02")))
	}
	if !s.LastReviewedAt.IsZero() {
		b.WriteString(fmt.Sprintf(" | Last Reviewed %s", s.LastReviewedAt.Format("2006-01-02")))
	}
	b.WriteString("\n")

	return b.String()
}
