package portfolio

import (
	"math"
	"strings"
	"testing"

	"github.com/bobmcallan/vire/internal/models"
)

func TestGenerateExternalBalanceID(t *testing.T) {
	id := generateExternalBalanceID()
	if !strings.HasPrefix(id, "eb_") {
		t.Errorf("ID should start with 'eb_', got %q", id)
	}
	// eb_ + 8 hex chars = 11 chars total
	if len(id) != 11 {
		t.Errorf("ID should be 11 chars, got %d: %q", len(id), id)
	}

	// IDs should be unique
	id2 := generateExternalBalanceID()
	if id == id2 {
		t.Errorf("IDs should be unique, got %q twice", id)
	}
}

func TestValidateExternalBalanceType(t *testing.T) {
	validTypes := []string{"cash", "accumulate", "term_deposit", "offset"}
	for _, typ := range validTypes {
		if !models.ValidateExternalBalanceType(typ) {
			t.Errorf("expected %q to be valid", typ)
		}
	}

	invalidTypes := []string{"", "savings", "CASH", "Cash", "loan", "mortgage"}
	for _, typ := range invalidTypes {
		if models.ValidateExternalBalanceType(typ) {
			t.Errorf("expected %q to be invalid", typ)
		}
	}
}

func TestRecomputeExternalBalanceTotal(t *testing.T) {
	tests := []struct {
		name     string
		balances []models.ExternalBalance
		want     float64
	}{
		{
			name:     "nil balances",
			balances: nil,
			want:     0,
		},
		{
			name:     "empty balances",
			balances: []models.ExternalBalance{},
			want:     0,
		},
		{
			name: "single balance",
			balances: []models.ExternalBalance{
				{ID: "eb_00000001", Type: "cash", Label: "Cash", Value: 50000},
			},
			want: 50000,
		},
		{
			name: "multiple balances",
			balances: []models.ExternalBalance{
				{ID: "eb_00000001", Type: "cash", Label: "ANZ Cash", Value: 44000},
				{ID: "eb_00000002", Type: "accumulate", Label: "Stake Accumulate", Value: 50000},
			},
			want: 94000,
		},
		{
			name: "zero value balance",
			balances: []models.ExternalBalance{
				{ID: "eb_00000001", Type: "cash", Label: "Empty Cash", Value: 0},
				{ID: "eb_00000002", Type: "cash", Label: "Some Cash", Value: 10000},
			},
			want: 10000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &models.Portfolio{ExternalBalances: tt.balances}
			recomputeExternalBalanceTotal(p)
			if !approxEqual(p.ExternalBalanceTotal, tt.want, 0.01) {
				t.Errorf("ExternalBalanceTotal = %.2f, want %.2f", p.ExternalBalanceTotal, tt.want)
			}
		})
	}
}

func TestRecomputeHoldingWeights(t *testing.T) {
	tests := []struct {
		name            string
		holdings        []models.Holding
		extBalanceTotal float64
		wantWeights     []float64
	}{
		{
			name: "no external balances - weights sum to 100",
			holdings: []models.Holding{
				{Ticker: "BHP", MarketValue: 50000},
				{Ticker: "CBA", MarketValue: 50000},
			},
			extBalanceTotal: 0,
			wantWeights:     []float64{50, 50},
		},
		{
			name: "with external balances - weights reduced",
			holdings: []models.Holding{
				{Ticker: "BHP", MarketValue: 50000},
				{Ticker: "CBA", MarketValue: 50000},
			},
			extBalanceTotal: 100000,
			// total denom = 100000 + 100000 = 200000
			// BHP weight = 50000/200000 * 100 = 25%
			// CBA weight = 50000/200000 * 100 = 25%
			wantWeights: []float64{25, 25},
		},
		{
			name:            "no holdings",
			holdings:        []models.Holding{},
			extBalanceTotal: 50000,
			wantWeights:     []float64{},
		},
		{
			name: "single holding with external balance",
			holdings: []models.Holding{
				{Ticker: "BHP", MarketValue: 100000},
			},
			extBalanceTotal: 50000,
			// total denom = 100000 + 50000 = 150000
			// weight = 100000/150000 * 100 = 66.67%
			wantWeights: []float64{66.67},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &models.Portfolio{
				Holdings:             tt.holdings,
				ExternalBalanceTotal: tt.extBalanceTotal,
			}
			recomputeHoldingWeights(p)

			for i, want := range tt.wantWeights {
				if !approxEqual(p.Holdings[i].Weight, want, 0.1) {
					t.Errorf("Holdings[%d].Weight = %.2f, want %.2f", i, p.Holdings[i].Weight, want)
				}
			}
		})
	}
}

func TestValidateExternalBalance(t *testing.T) {
	tests := []struct {
		name    string
		balance models.ExternalBalance
		wantErr bool
	}{
		{
			name:    "valid cash balance",
			balance: models.ExternalBalance{Type: "cash", Label: "ANZ Cash", Value: 44000},
			wantErr: false,
		},
		{
			name:    "valid accumulate with rate",
			balance: models.ExternalBalance{Type: "accumulate", Label: "Stake", Value: 50000, Rate: 0.05},
			wantErr: false,
		},
		{
			name:    "valid term deposit",
			balance: models.ExternalBalance{Type: "term_deposit", Label: "Term", Value: 100000, Rate: 0.045},
			wantErr: false,
		},
		{
			name:    "valid offset",
			balance: models.ExternalBalance{Type: "offset", Label: "Offset", Value: 200000},
			wantErr: false,
		},
		{
			name:    "missing type",
			balance: models.ExternalBalance{Label: "Cash", Value: 1000},
			wantErr: true,
		},
		{
			name:    "invalid type",
			balance: models.ExternalBalance{Type: "savings", Label: "Sav", Value: 1000},
			wantErr: true,
		},
		{
			name:    "missing label",
			balance: models.ExternalBalance{Type: "cash", Value: 1000},
			wantErr: true,
		},
		{
			name:    "negative value",
			balance: models.ExternalBalance{Type: "cash", Label: "Cash", Value: -100},
			wantErr: true,
		},
		{
			name:    "negative rate",
			balance: models.ExternalBalance{Type: "cash", Label: "Cash", Value: 1000, Rate: -0.05},
			wantErr: true,
		},
		{
			name:    "zero value is allowed",
			balance: models.ExternalBalance{Type: "cash", Label: "Empty Cash", Value: 0},
			wantErr: false,
		},
		// Hostile input cases
		{
			name:    "whitespace-only label",
			balance: models.ExternalBalance{Type: "cash", Label: "   ", Value: 1000},
			wantErr: true,
		},
		{
			name:    "tab-newline label",
			balance: models.ExternalBalance{Type: "cash", Label: "\t\n", Value: 1000},
			wantErr: true,
		},
		{
			name:    "label exceeds max length",
			balance: models.ExternalBalance{Type: "cash", Label: strings.Repeat("x", 201), Value: 1000},
			wantErr: true,
		},
		{
			name:    "label at max length is allowed",
			balance: models.ExternalBalance{Type: "cash", Label: strings.Repeat("x", 200), Value: 1000},
			wantErr: false,
		},
		{
			name:    "notes exceeds max length",
			balance: models.ExternalBalance{Type: "cash", Label: "Cash", Value: 1000, Notes: strings.Repeat("x", 1001)},
			wantErr: true,
		},
		{
			name:    "notes at max length is allowed",
			balance: models.ExternalBalance{Type: "cash", Label: "Cash", Value: 1000, Notes: strings.Repeat("x", 1000)},
			wantErr: false,
		},
		{
			name:    "NaN value",
			balance: models.ExternalBalance{Type: "cash", Label: "Cash", Value: math.NaN()},
			wantErr: true,
		},
		{
			name:    "positive infinity value",
			balance: models.ExternalBalance{Type: "cash", Label: "Cash", Value: math.Inf(1)},
			wantErr: true,
		},
		{
			name:    "negative infinity value",
			balance: models.ExternalBalance{Type: "cash", Label: "Cash", Value: math.Inf(-1)},
			wantErr: true,
		},
		{
			name:    "NaN rate",
			balance: models.ExternalBalance{Type: "cash", Label: "Cash", Value: 1000, Rate: math.NaN()},
			wantErr: true,
		},
		{
			name:    "positive infinity rate",
			balance: models.ExternalBalance{Type: "cash", Label: "Cash", Value: 1000, Rate: math.Inf(1)},
			wantErr: true,
		},
		{
			name:    "value exceeds 1e15 maximum",
			balance: models.ExternalBalance{Type: "cash", Label: "Cash", Value: 1e15 + 1},
			wantErr: true,
		},
		{
			name:    "value at 1e15 boundary is allowed",
			balance: models.ExternalBalance{Type: "cash", Label: "Cash", Value: 1e15},
			wantErr: false,
		},
		{
			name:    "MaxFloat64 value rejected",
			balance: models.ExternalBalance{Type: "cash", Label: "Cash", Value: math.MaxFloat64},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateExternalBalance(tt.balance)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
