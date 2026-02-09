// Package models defines data structures for Vire
package models

import (
	"fmt"
	"strings"
	"time"
)

// PlanItemType categorizes plan items
type PlanItemType string

const (
	PlanItemTypeTime  PlanItemType = "time"
	PlanItemTypeEvent PlanItemType = "event"
)

// PlanItemStatus tracks plan item lifecycle
type PlanItemStatus string

const (
	PlanItemStatusPending   PlanItemStatus = "pending"
	PlanItemStatusTriggered PlanItemStatus = "triggered"
	PlanItemStatusCompleted PlanItemStatus = "completed"
	PlanItemStatusExpired   PlanItemStatus = "expired"
	PlanItemStatusCancelled PlanItemStatus = "cancelled"
)

// PlanItem represents a single action item within a portfolio plan.
type PlanItem struct {
	ID          string          `json:"id"`
	Type        PlanItemType    `json:"type"`
	Description string          `json:"description"`
	Status      PlanItemStatus  `json:"status"`
	Deadline    *time.Time      `json:"deadline,omitempty"`   // time-based
	Conditions  []RuleCondition `json:"conditions,omitempty"` // event-based (reuses strategy RuleCondition)
	Ticker      string          `json:"ticker,omitempty"`     // event-based target ticker
	Action      RuleAction      `json:"action,omitempty"`
	TargetValue float64         `json:"target_value,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
	Notes       string          `json:"notes,omitempty"`
}

// PortfolioPlan is a versioned collection of time-based and event-based action items.
type PortfolioPlan struct {
	PortfolioName string     `json:"portfolio_name"`
	Version       int        `json:"version"`
	Items         []PlanItem `json:"items"`
	Notes         string     `json:"notes,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// ToMarkdown renders the plan as a readable markdown document.
func (p *PortfolioPlan) ToMarkdown() string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# Portfolio Plan: %s\n\n", p.PortfolioName))

	if len(p.Items) == 0 {
		b.WriteString("No plan items.\n\n")
	} else {
		// Separate by status
		pending := make([]PlanItem, 0)
		triggered := make([]PlanItem, 0)
		completed := make([]PlanItem, 0)
		other := make([]PlanItem, 0)

		for _, item := range p.Items {
			switch item.Status {
			case PlanItemStatusPending:
				pending = append(pending, item)
			case PlanItemStatusTriggered:
				triggered = append(triggered, item)
			case PlanItemStatusCompleted:
				completed = append(completed, item)
			default:
				other = append(other, item)
			}
		}

		if len(triggered) > 0 {
			b.WriteString("## Triggered\n\n")
			for _, item := range triggered {
				writePlanItem(&b, item)
			}
		}

		if len(pending) > 0 {
			b.WriteString("## Pending\n\n")
			for _, item := range pending {
				writePlanItem(&b, item)
			}
		}

		if len(completed) > 0 {
			b.WriteString("## Completed\n\n")
			for _, item := range completed {
				writePlanItem(&b, item)
			}
		}

		if len(other) > 0 {
			b.WriteString("## Other\n\n")
			for _, item := range other {
				writePlanItem(&b, item)
			}
		}
	}

	if p.Notes != "" {
		b.WriteString("## Notes\n\n")
		b.WriteString(p.Notes)
		b.WriteString("\n\n")
	}

	// Metadata
	b.WriteString("---\n\n")
	b.WriteString(fmt.Sprintf("Version %d", p.Version))
	if !p.CreatedAt.IsZero() {
		b.WriteString(fmt.Sprintf(" | Created %s", p.CreatedAt.Format("2006-01-02")))
	}
	if !p.UpdatedAt.IsZero() {
		b.WriteString(fmt.Sprintf(" | Updated %s", p.UpdatedAt.Format("2006-01-02")))
	}
	b.WriteString("\n")

	return b.String()
}

func writePlanItem(b *strings.Builder, item PlanItem) {
	statusIcon := "⬜"
	switch item.Status {
	case PlanItemStatusPending:
		statusIcon = "⬜"
	case PlanItemStatusTriggered:
		statusIcon = "🟡"
	case PlanItemStatusCompleted:
		statusIcon = "✅"
	case PlanItemStatusExpired:
		statusIcon = "⏰"
	case PlanItemStatusCancelled:
		statusIcon = "❌"
	}

	b.WriteString(fmt.Sprintf("- %s **[%s]** %s", statusIcon, item.ID, item.Description))

	if item.Type == PlanItemTypeTime && item.Deadline != nil {
		remaining := time.Until(*item.Deadline)
		if remaining > 0 {
			b.WriteString(fmt.Sprintf(" (due %s, %d days remaining)", item.Deadline.Format("2006-01-02"), int(remaining.Hours()/24)))
		} else {
			b.WriteString(fmt.Sprintf(" (due %s, **overdue**)", item.Deadline.Format("2006-01-02")))
		}
	}

	if item.Ticker != "" {
		b.WriteString(fmt.Sprintf(" | Ticker: %s", item.Ticker))
	}
	if item.Action != "" {
		b.WriteString(fmt.Sprintf(" | Action: %s", string(item.Action)))
	}

	b.WriteString("\n")

	if item.Notes != "" {
		b.WriteString(fmt.Sprintf("  - *%s*\n", item.Notes))
	}
}
