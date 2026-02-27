package models

import "time"

// GlossaryResponse is the top-level response for the active glossary endpoint.
type GlossaryResponse struct {
	PortfolioName string             `json:"portfolio_name"`
	GeneratedAt   time.Time          `json:"generated_at"`
	Categories    []GlossaryCategory `json:"categories"`
}

// GlossaryCategory groups related glossary terms.
type GlossaryCategory struct {
	Name  string         `json:"name"`
	Terms []GlossaryTerm `json:"terms"`
}

// GlossaryTerm defines a single term with a live example from the portfolio.
type GlossaryTerm struct {
	Term       string      `json:"term"`
	Label      string      `json:"label"`
	Definition string      `json:"definition"`
	Formula    string      `json:"formula,omitempty"`
	Value      interface{} `json:"value"`
	Example    string      `json:"example,omitempty"`
}
