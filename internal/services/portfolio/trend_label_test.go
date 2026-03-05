package portfolio

import (
	"testing"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestTrendMomentumLabel(t *testing.T) {
	tests := []struct {
		level models.TrendMomentumLevel
		want  string
	}{
		{models.TrendMomentumStrongUp, "Strong Uptrend"},
		{models.TrendMomentumUp, "Uptrend"},
		{models.TrendMomentumFlat, "Consolidating"},
		{models.TrendMomentumDown, "Downtrend"},
		{models.TrendMomentumStrongDown, "Strong Downtrend"},
		{"UNKNOWN_LEVEL", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			assert.Equal(t, tt.want, trendMomentumLabel(tt.level))
		})
	}
}
