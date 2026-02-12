// Package common provides shared utilities for Vire
package common

import (
	"fmt"
	"strings"

	"github.com/bobmcallan/vire/internal/models"
)

// FormatMoney formats a float as a dollar amount with comma separators
func FormatMoney(v float64) string {
	negative := v < 0
	if negative {
		v = -v
	}
	whole := int64(v)
	cents := int64((v-float64(whole))*100 + 0.5)
	if cents >= 100 {
		whole++
		cents -= 100
	}

	s := fmt.Sprintf("%d", whole)
	if len(s) > 3 {
		var parts []string
		for len(s) > 3 {
			parts = append([]string{s[len(s)-3:]}, parts...)
			s = s[:len(s)-3]
		}
		parts = append([]string{s}, parts...)
		s = strings.Join(parts, ",")
	}

	if negative {
		return fmt.Sprintf("-$%s.%02d", s, cents)
	}
	return fmt.Sprintf("$%s.%02d", s, cents)
}

// FormatSignedMoney formats a dollar amount with +/- prefix
func FormatSignedMoney(v float64) string {
	if v >= 0 {
		return "+" + FormatMoney(v)
	}
	return FormatMoney(v)
}

// FormatSignedPct formats a percentage with +/- prefix
func FormatSignedPct(v float64) string {
	if v >= 0 {
		return fmt.Sprintf("+%.2f%%", v)
	}
	return fmt.Sprintf("%.2f%%", v)
}

// FormatMarketCap formats market cap with appropriate suffix (M/B)
func FormatMarketCap(v float64) string {
	if v >= 1e9 {
		return fmt.Sprintf("$%.2fB", v/1e9)
	}
	return fmt.Sprintf("$%.2fM", v/1e6)
}

// IsETF determines if a holding is an ETF based on fundamentals or name
func IsETF(hr *models.HoldingReview) bool {
	if hr.Fundamentals != nil && hr.Fundamentals.IsETF {
		return true
	}

	name := strings.ToUpper(hr.Holding.Name)
	if strings.Contains(name, " ETF") || strings.HasSuffix(name, " ETF") {
		return true
	}

	if hr.Fundamentals != nil && hr.Fundamentals.Sector == "" && hr.Fundamentals.Industry == "" {
		return true
	}

	return false
}
