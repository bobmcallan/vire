package portfolio

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/bobmcallan/vire/internal/models"
)

// --- Hostile input tests ---

func TestValidateExternalBalance_HostileInputs(t *testing.T) {
	tests := []struct {
		name    string
		balance models.ExternalBalance
		wantErr bool
		desc    string
	}{
		{
			name:    "whitespace-only label",
			balance: models.ExternalBalance{Type: "cash", Label: "   ", Value: 1000},
			wantErr: true,
			desc:    "label of only spaces should be rejected",
		},
		{
			name:    "tab-only label",
			balance: models.ExternalBalance{Type: "cash", Label: "\t\n", Value: 1000},
			wantErr: true,
			desc:    "label of only whitespace chars should be rejected",
		},
		{
			name:    "extremely long label",
			balance: models.ExternalBalance{Type: "cash", Label: strings.Repeat("A", 10000), Value: 1000},
			wantErr: true,
			desc:    "label over max length should be rejected",
		},
		{
			name:    "extremely long notes",
			balance: models.ExternalBalance{Type: "cash", Label: "Cash", Value: 1000, Notes: strings.Repeat("X", 10000)},
			wantErr: true,
			desc:    "notes over max length should be rejected",
		},
		{
			name:    "HTML injection in label",
			balance: models.ExternalBalance{Type: "cash", Label: "<script>alert('xss')</script>", Value: 1000},
			wantErr: false,
			desc:    "HTML in label is fine - Go JSON encoder escapes HTML",
		},
		{
			name:    "SQL injection in type",
			balance: models.ExternalBalance{Type: "cash'; DROP TABLE users;--", Label: "Evil", Value: 1000},
			wantErr: true,
			desc:    "SQL injection in type is rejected by type validation",
		},
		{
			name:    "unicode emoji in label",
			balance: models.ExternalBalance{Type: "cash", Label: "💰 Cash Account", Value: 1000},
			wantErr: false,
			desc:    "unicode in label is acceptable",
		},
		{
			name:    "null bytes in label",
			balance: models.ExternalBalance{Type: "cash", Label: "Cash\x00Account", Value: 1000},
			wantErr: false,
			desc:    "null bytes in label - JSON serialization will handle this",
		},
		{
			name:    "NaN value",
			balance: models.ExternalBalance{Type: "cash", Label: "Cash", Value: math.NaN()},
			wantErr: true,
			desc:    "NaN should be rejected",
		},
		{
			name:    "positive infinity value",
			balance: models.ExternalBalance{Type: "cash", Label: "Cash", Value: math.Inf(1)},
			wantErr: true,
			desc:    "Inf should be rejected",
		},
		{
			name:    "negative infinity value",
			balance: models.ExternalBalance{Type: "cash", Label: "Cash", Value: math.Inf(-1)},
			wantErr: true,
			desc:    "negative Inf should be rejected (also negative)",
		},
		{
			name:    "max float64 value",
			balance: models.ExternalBalance{Type: "cash", Label: "Cash", Value: math.MaxFloat64},
			wantErr: true,
			desc:    "max float64 should be rejected to prevent overflow in weight calculations",
		},
		{
			name:    "NaN rate",
			balance: models.ExternalBalance{Type: "cash", Label: "Cash", Value: 1000, Rate: math.NaN()},
			wantErr: true,
			desc:    "NaN rate should be rejected",
		},
		{
			name:    "infinity rate",
			balance: models.ExternalBalance{Type: "cash", Label: "Cash", Value: 1000, Rate: math.Inf(1)},
			wantErr: true,
			desc:    "Inf rate should be rejected",
		},
		{
			name:    "type with leading whitespace",
			balance: models.ExternalBalance{Type: " cash", Label: "Cash", Value: 1000},
			wantErr: true,
			desc:    "type with leading space should be rejected",
		},
		{
			name:    "type with trailing whitespace",
			balance: models.ExternalBalance{Type: "cash ", Label: "Cash", Value: 1000},
			wantErr: true,
			desc:    "type with trailing space should be rejected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateExternalBalance(tt.balance)
			if tt.wantErr && err == nil {
				t.Errorf("%s: expected error, got nil", tt.desc)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("%s: unexpected error: %v", tt.desc, err)
			}
		})
	}
}

// --- ID generation tests ---

func TestGenerateExternalBalanceID_Uniqueness(t *testing.T) {
	// Generate 1000 IDs and check for collisions
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		id := generateExternalBalanceID()
		if seen[id] {
			t.Fatalf("collision after %d IDs: %q", i, id)
		}
		seen[id] = true
	}
}

func TestGenerateExternalBalanceID_Format(t *testing.T) {
	for i := 0; i < 100; i++ {
		id := generateExternalBalanceID()
		if !strings.HasPrefix(id, "eb_") {
			t.Errorf("ID %q missing eb_ prefix", id)
		}
		if len(id) != 11 {
			t.Errorf("ID %q has length %d, want 11", id, len(id))
		}
		// Verify hex chars after prefix
		hexPart := id[3:]
		for _, c := range hexPart {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("ID %q contains non-hex char %q", id, string(c))
			}
		}
	}
}

// --- SetExternalBalances caller-supplied ID bypass ---

func TestSetExternalBalances_CallerSuppliedID(t *testing.T) {
	// SetExternalBalances preserves caller IDs if non-empty.
	// Verify that a hostile caller can set arbitrary IDs.
	// This isn't a security risk (storage is user-scoped), but could
	// violate ID format expectations.
	balances := []models.ExternalBalance{
		{ID: "HOSTILE_ID", Type: "cash", Label: "Evil", Value: 1000},
	}

	// Validate passes (no ID format check)
	for _, b := range balances {
		if err := validateExternalBalance(b); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
	}

	// The ID "HOSTILE_ID" does NOT have the eb_ prefix
	if strings.HasPrefix(balances[0].ID, "eb_") {
		t.Error("test setup error: hostile ID should not have eb_ prefix")
	}
}

// --- Weight calculation edge cases ---

func TestRecomputeHoldingWeights_ZeroDenominator(t *testing.T) {
	// Zero market value + zero external balance = zero denominator
	p := &models.Portfolio{
		Holdings: []models.Holding{
			{Ticker: "BHP", MarketValue: 0},
		},
		ExternalBalanceTotal: 0,
	}
	recomputeHoldingWeights(p)
	if p.Holdings[0].Weight != 0 {
		t.Errorf("expected 0 weight for zero denominator, got %.2f", p.Holdings[0].Weight)
	}
}

func TestRecomputeHoldingWeights_OnlyExternalBalances(t *testing.T) {
	// No holdings but external balances exist
	p := &models.Portfolio{
		Holdings:             []models.Holding{},
		ExternalBalanceTotal: 100000,
	}
	recomputeHoldingWeights(p)
	// No holdings to check, just verify no panic
}

func TestRecomputeHoldingWeights_LargeExternalBalance(t *testing.T) {
	// External balance dwarfs holdings — weights should be near-zero
	p := &models.Portfolio{
		Holdings: []models.Holding{
			{Ticker: "BHP", MarketValue: 1000},
		},
		ExternalBalanceTotal: 1e12, // $1 trillion
	}
	recomputeHoldingWeights(p)
	if p.Holdings[0].Weight > 0.001 {
		t.Errorf("weight should be near-zero, got %.6f", p.Holdings[0].Weight)
	}
}

// --- JSON serialization backward compatibility ---

func TestExternalBalances_BackwardCompatibility(t *testing.T) {
	// Old portfolio JSON without external_balances field should deserialize cleanly
	oldJSON := `{
		"id": "test",
		"name": "SMSF",
		"holdings": [],
		"total_value": 100000,
		"total_cost": 90000,
		"total_net_return": 10000,
		"total_net_return_pct": 11.11,
		"currency": "AUD",
		"last_synced": "2025-01-01T00:00:00Z",
		"created_at": "2025-01-01T00:00:00Z",
		"updated_at": "2025-01-01T00:00:00Z"
	}`

	var p models.Portfolio
	if err := json.Unmarshal([]byte(oldJSON), &p); err != nil {
		t.Fatalf("failed to unmarshal old portfolio JSON: %v", err)
	}

	// ExternalBalances should be nil (not populated)
	if p.ExternalBalances != nil {
		t.Errorf("ExternalBalances should be nil for old JSON, got %v", p.ExternalBalances)
	}

	// ExternalBalanceTotal should be zero (default)
	if p.ExternalBalanceTotal != 0 {
		t.Errorf("ExternalBalanceTotal should be 0 for old JSON, got %.2f", p.ExternalBalanceTotal)
	}
}

func TestExternalBalances_OmitEmpty(t *testing.T) {
	// Portfolio with no external balances should not include the field in JSON
	p := models.Portfolio{
		ID:   "test",
		Name: "SMSF",
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}

	// external_balances has omitempty so should be absent when nil
	if strings.Contains(string(data), "external_balances") {
		t.Error("external_balances should be omitted when nil")
	}

	// external_balance_total does NOT have omitempty, so it's always present
	if !strings.Contains(string(data), "external_balance_total") {
		t.Error("external_balance_total should always be present")
	}
}

func TestExternalBalances_RoundtripJSON(t *testing.T) {
	// Verify roundtrip serialization works correctly
	original := models.Portfolio{
		ID:   "test",
		Name: "SMSF",
		ExternalBalances: []models.ExternalBalance{
			{ID: "eb_12345678", Type: "cash", Label: "Cash", Value: 50000, Rate: 0.05, Notes: "Test notes"},
			{ID: "eb_abcdef01", Type: "term_deposit", Label: "Term", Value: 100000},
		},
		ExternalBalanceTotal: 150000,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}

	var restored models.Portfolio
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}

	if len(restored.ExternalBalances) != 2 {
		t.Fatalf("expected 2 balances, got %d", len(restored.ExternalBalances))
	}
	if restored.ExternalBalances[0].ID != "eb_12345678" {
		t.Errorf("ID mismatch: %q", restored.ExternalBalances[0].ID)
	}
	if restored.ExternalBalances[0].Rate != 0.05 {
		t.Errorf("Rate not preserved: %.4f", restored.ExternalBalances[0].Rate)
	}
	if restored.ExternalBalances[1].Notes != "" {
		t.Errorf("empty Notes should not be populated: %q", restored.ExternalBalances[1].Notes)
	}
	if !approxEqual(restored.ExternalBalanceTotal, 150000, 0.01) {
		t.Errorf("total mismatch: %.2f", restored.ExternalBalanceTotal)
	}
}

// --- Validation edge cases for types with case sensitivity ---

func TestValidateExternalBalanceType_CaseSensitivity(t *testing.T) {
	// Types are case-sensitive — only lowercase accepted
	cases := []struct {
		input string
		valid bool
	}{
		{"cash", true},
		{"Cash", false},
		{"CASH", false},
		{"cAsH", false},
		{"term_deposit", true},
		{"Term_Deposit", false},
		{"TERM_DEPOSIT", false},
		{"accumulate", true},
		{"ACCUMULATE", false},
		{"offset", true},
		{"OFFSET", false},
	}

	for _, tc := range cases {
		got := models.ValidateExternalBalanceType(tc.input)
		if got != tc.valid {
			t.Errorf("ValidateExternalBalanceType(%q) = %v, want %v", tc.input, got, tc.valid)
		}
	}
}

// --- Recompute total with extreme float values ---

func TestRecomputeExternalBalanceTotal_ExtremeValues(t *testing.T) {
	p := &models.Portfolio{
		ExternalBalances: []models.ExternalBalance{
			{Value: 1e15},  // $1 quadrillion
			{Value: 1e-10}, // tiny value
			{Value: 0},     // zero
		},
	}
	recomputeExternalBalanceTotal(p)
	// Should not lose precision on the large value
	if p.ExternalBalanceTotal < 1e15 {
		t.Errorf("total lost precision: %.2f", p.ExternalBalanceTotal)
	}
}

func TestRecomputeHoldingWeights_WeightsSumCheck(t *testing.T) {
	// When external balances exist, holding weights should NOT sum to 100
	p := &models.Portfolio{
		Holdings: []models.Holding{
			{Ticker: "BHP", MarketValue: 30000},
			{Ticker: "CBA", MarketValue: 30000},
			{Ticker: "WES", MarketValue: 40000},
		},
		ExternalBalanceTotal: 100000, // equal to total market value
	}
	recomputeHoldingWeights(p)

	totalWeight := 0.0
	for _, h := range p.Holdings {
		totalWeight += h.Weight
	}
	// Total market value = 100000, external = 100000, denom = 200000
	// Total weight should be ~50%
	if !approxEqual(totalWeight, 50, 1) {
		t.Errorf("total weight should be ~50%% when external equals market value, got %.2f%%", totalWeight)
	}
}
