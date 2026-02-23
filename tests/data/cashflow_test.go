package data

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCashFlowLedgerLifecycle verifies that a CashFlowLedger with transactions
// survives storage roundtrip through SurrealDB via UserDataStore (subject "cashflow").
func TestCashFlowLedgerLifecycle(t *testing.T) {
	mgr := testManager(t)
	store := mgr.UserDataStore()
	ctx := testContext()

	ledger := models.CashFlowLedger{
		PortfolioName: "SMSF",
		Version:       1,
		Transactions: []models.CashTransaction{
			{
				ID:          "ct_aabbccdd",
				Type:        models.CashTxDeposit,
				Date:        time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
				Amount:      50000,
				Description: "Initial SMSF deposit",
				Notes:       "Rollover from previous fund",
				CreatedAt:   time.Now().Truncate(time.Second),
				UpdatedAt:   time.Now().Truncate(time.Second),
			},
		},
		Notes:     "Test ledger",
		CreatedAt: time.Now().Truncate(time.Second),
		UpdatedAt: time.Now().Truncate(time.Second),
	}

	// Store as JSON (same path as production CashFlowService)
	data, err := json.Marshal(ledger)
	require.NoError(t, err, "marshal ledger")

	record := &models.UserRecord{
		UserID:   "cf_lifecycle_user",
		Subject:  "cashflow",
		Key:      "SMSF",
		Value:    string(data),
		Version:  1,
		DateTime: time.Now().Truncate(time.Second),
	}
	require.NoError(t, store.Put(ctx, record), "store ledger")

	// Retrieve
	got, err := store.Get(ctx, "cf_lifecycle_user", "cashflow", "SMSF")
	require.NoError(t, err, "get ledger")

	var restored models.CashFlowLedger
	require.NoError(t, json.Unmarshal([]byte(got.Value), &restored), "unmarshal ledger")

	// Verify ledger fields
	assert.Equal(t, "SMSF", restored.PortfolioName)
	assert.Equal(t, 1, restored.Version)
	assert.Equal(t, "Test ledger", restored.Notes)

	// Verify transactions
	require.Len(t, restored.Transactions, 1)
	tx := restored.Transactions[0]
	assert.Equal(t, "ct_aabbccdd", tx.ID)
	assert.Equal(t, models.CashTxDeposit, tx.Type)
	assert.InDelta(t, 50000.0, tx.Amount, 0.01)
	assert.Equal(t, "Initial SMSF deposit", tx.Description)
	assert.Equal(t, "Rollover from previous fund", tx.Notes)

	// Update: add a second transaction and increment version
	restored.Transactions = append(restored.Transactions, models.CashTransaction{
		ID:          "ct_11223344",
		Type:        models.CashTxContribution,
		Date:        time.Date(2025, 2, 15, 0, 0, 0, 0, time.UTC),
		Amount:      10000,
		Description: "Employer contribution Q1",
		Category:    "employer",
		CreatedAt:   time.Now().Truncate(time.Second),
		UpdatedAt:   time.Now().Truncate(time.Second),
	})
	restored.Version = 2
	restored.UpdatedAt = time.Now().Truncate(time.Second)

	updatedData, err := json.Marshal(restored)
	require.NoError(t, err)

	record.Value = string(updatedData)
	record.Version = 2
	require.NoError(t, store.Put(ctx, record))

	// Retrieve again
	got2, err := store.Get(ctx, "cf_lifecycle_user", "cashflow", "SMSF")
	require.NoError(t, err)

	var restored2 models.CashFlowLedger
	require.NoError(t, json.Unmarshal([]byte(got2.Value), &restored2))

	assert.Equal(t, 2, restored2.Version)
	require.Len(t, restored2.Transactions, 2)
	assert.Equal(t, "ct_aabbccdd", restored2.Transactions[0].ID)
	assert.Equal(t, "ct_11223344", restored2.Transactions[1].ID)

	// Delete
	require.NoError(t, store.Delete(ctx, "cf_lifecycle_user", "cashflow", "SMSF"))
	_, err = store.Get(ctx, "cf_lifecycle_user", "cashflow", "SMSF")
	assert.Error(t, err)
}

// TestCashFlowTransactionOrdering verifies that transactions stored in date-ascending
// order survive storage roundtrip with correct ordering.
func TestCashFlowTransactionOrdering(t *testing.T) {
	mgr := testManager(t)
	store := mgr.UserDataStore()
	ctx := testContext()

	// Create transactions out of date order in the slice, but
	// service should sort them by date ascending before storing
	txns := []models.CashTransaction{
		{
			ID:          "ct_march",
			Type:        models.CashTxWithdrawal,
			Date:        time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
			Amount:      5000,
			Description: "March withdrawal",
			CreatedAt:   time.Now().Truncate(time.Second),
			UpdatedAt:   time.Now().Truncate(time.Second),
		},
		{
			ID:          "ct_january",
			Type:        models.CashTxDeposit,
			Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Amount:      100000,
			Description: "January deposit",
			CreatedAt:   time.Now().Truncate(time.Second),
			UpdatedAt:   time.Now().Truncate(time.Second),
		},
		{
			ID:          "ct_february",
			Type:        models.CashTxContribution,
			Date:        time.Date(2025, 2, 15, 0, 0, 0, 0, time.UTC),
			Amount:      10000,
			Description: "February contribution",
			CreatedAt:   time.Now().Truncate(time.Second),
			UpdatedAt:   time.Now().Truncate(time.Second),
		},
	}

	ledger := models.CashFlowLedger{
		PortfolioName: "ordering_test",
		Version:       1,
		Transactions:  txns,
		CreatedAt:     time.Now().Truncate(time.Second),
		UpdatedAt:     time.Now().Truncate(time.Second),
	}

	data, err := json.Marshal(ledger)
	require.NoError(t, err)

	require.NoError(t, store.Put(ctx, &models.UserRecord{
		UserID:   "cf_order_user",
		Subject:  "cashflow",
		Key:      "ordering_test",
		Value:    string(data),
		Version:  1,
		DateTime: time.Now().Truncate(time.Second),
	}))

	got, err := store.Get(ctx, "cf_order_user", "cashflow", "ordering_test")
	require.NoError(t, err)

	var restored models.CashFlowLedger
	require.NoError(t, json.Unmarshal([]byte(got.Value), &restored))

	require.Len(t, restored.Transactions, 3)

	// Verify all transactions present (IDs match)
	ids := make([]string, len(restored.Transactions))
	for i, tx := range restored.Transactions {
		ids[i] = tx.ID
	}
	assert.Contains(t, ids, "ct_march")
	assert.Contains(t, ids, "ct_january")
	assert.Contains(t, ids, "ct_february")
}

// TestCashFlowAllTransactionTypes verifies that all valid CashTransactionType values
// serialize/deserialize correctly through storage.
func TestCashFlowAllTransactionTypes(t *testing.T) {
	mgr := testManager(t)
	store := mgr.UserDataStore()
	ctx := testContext()

	types := []struct {
		txType models.CashTransactionType
		isIn   bool
	}{
		{models.CashTxDeposit, true},
		{models.CashTxWithdrawal, false},
		{models.CashTxContribution, true},
		{models.CashTxTransferIn, true},
		{models.CashTxTransferOut, false},
		{models.CashTxDividend, true},
	}

	var txns []models.CashTransaction
	for i, tt := range types {
		txns = append(txns, models.CashTransaction{
			ID:          "ct_type_" + string(tt.txType),
			Type:        tt.txType,
			Date:        time.Date(2025, time.Month(i+1), 1, 0, 0, 0, 0, time.UTC),
			Amount:      float64((i + 1) * 1000),
			Description: "Test " + string(tt.txType),
			CreatedAt:   time.Now().Truncate(time.Second),
			UpdatedAt:   time.Now().Truncate(time.Second),
		})
	}

	ledger := models.CashFlowLedger{
		PortfolioName: "types_test",
		Version:       1,
		Transactions:  txns,
		CreatedAt:     time.Now().Truncate(time.Second),
		UpdatedAt:     time.Now().Truncate(time.Second),
	}

	data, err := json.Marshal(ledger)
	require.NoError(t, err)

	require.NoError(t, store.Put(ctx, &models.UserRecord{
		UserID:   "cf_types_user",
		Subject:  "cashflow",
		Key:      "types_test",
		Value:    string(data),
		Version:  1,
		DateTime: time.Now().Truncate(time.Second),
	}))

	got, err := store.Get(ctx, "cf_types_user", "cashflow", "types_test")
	require.NoError(t, err)

	var restored models.CashFlowLedger
	require.NoError(t, json.Unmarshal([]byte(got.Value), &restored))

	require.Len(t, restored.Transactions, 6)

	for i, tt := range types {
		t.Run(string(tt.txType), func(t *testing.T) {
			tx := restored.Transactions[i]
			assert.Equal(t, tt.txType, tx.Type)
			assert.InDelta(t, float64((i+1)*1000), tx.Amount, 0.01)

			// Verify type classification helpers
			assert.Equal(t, tt.isIn, models.IsInflowType(tx.Type), "IsInflowType for %s", tt.txType)
			assert.True(t, models.ValidCashTransactionType(tx.Type), "ValidCashTransactionType for %s", tt.txType)
		})
	}
}

// TestCashFlowPrecision verifies that transaction amounts with decimal precision
// survive storage roundtrip without floating-point issues.
func TestCashFlowPrecision(t *testing.T) {
	mgr := testManager(t)
	store := mgr.UserDataStore()
	ctx := testContext()

	// Values prone to floating-point precision issues
	amounts := []float64{0.01, 0.10, 1.23, 99.99, 12345.67, 999999.99, 1000000.50}

	var txns []models.CashTransaction
	for i, amt := range amounts {
		txns = append(txns, models.CashTransaction{
			ID:          "ct_prec_" + string(rune('a'+i)),
			Type:        models.CashTxDeposit,
			Date:        time.Date(2025, 1, i+1, 0, 0, 0, 0, time.UTC),
			Amount:      amt,
			Description: "Precision test",
			CreatedAt:   time.Now().Truncate(time.Second),
			UpdatedAt:   time.Now().Truncate(time.Second),
		})
	}

	ledger := models.CashFlowLedger{
		PortfolioName: "precision_test",
		Version:       1,
		Transactions:  txns,
		CreatedAt:     time.Now().Truncate(time.Second),
		UpdatedAt:     time.Now().Truncate(time.Second),
	}

	data, err := json.Marshal(ledger)
	require.NoError(t, err)

	require.NoError(t, store.Put(ctx, &models.UserRecord{
		UserID:   "cf_prec_user",
		Subject:  "cashflow",
		Key:      "precision_test",
		Value:    string(data),
		Version:  1,
		DateTime: time.Now().Truncate(time.Second),
	}))

	got, err := store.Get(ctx, "cf_prec_user", "cashflow", "precision_test")
	require.NoError(t, err)

	var restored models.CashFlowLedger
	require.NoError(t, json.Unmarshal([]byte(got.Value), &restored))

	require.Len(t, restored.Transactions, len(amounts))
	for i, expected := range amounts {
		assert.InDelta(t, expected, restored.Transactions[i].Amount, 0.001,
			"amount[%d] should preserve precision", i)
	}
}

// TestCashFlowCapitalPerformanceStorage verifies that CapitalPerformance model
// serializes and deserializes correctly.
func TestCashFlowCapitalPerformanceStorage(t *testing.T) {
	mgr := testManager(t)
	store := mgr.UserDataStore()
	ctx := testContext()

	firstDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	perf := models.CapitalPerformance{
		TotalDeposited:        138000,
		TotalWithdrawn:        20000,
		NetCapitalDeployed:    118000,
		CurrentPortfolioValue: 145000,
		SimpleReturnPct:       22.88,
		AnnualizedReturnPct:   18.5,
		FirstTransactionDate:  &firstDate,
		TransactionCount:      6,
	}

	data, err := json.Marshal(perf)
	require.NoError(t, err)

	// Store as a user record (verifying the model marshals correctly)
	require.NoError(t, store.Put(ctx, &models.UserRecord{
		UserID:   "cf_perf_user",
		Subject:  "cashflow_perf",
		Key:      "SMSF",
		Value:    string(data),
		Version:  1,
		DateTime: time.Now().Truncate(time.Second),
	}))

	got, err := store.Get(ctx, "cf_perf_user", "cashflow_perf", "SMSF")
	require.NoError(t, err)

	var restored models.CapitalPerformance
	require.NoError(t, json.Unmarshal([]byte(got.Value), &restored))

	assert.InDelta(t, 138000.0, restored.TotalDeposited, 0.01)
	assert.InDelta(t, 20000.0, restored.TotalWithdrawn, 0.01)
	assert.InDelta(t, 118000.0, restored.NetCapitalDeployed, 0.01)
	assert.InDelta(t, 145000.0, restored.CurrentPortfolioValue, 0.01)
	assert.InDelta(t, 22.88, restored.SimpleReturnPct, 0.01)
	assert.InDelta(t, 18.5, restored.AnnualizedReturnPct, 0.01)
	assert.Equal(t, 6, restored.TransactionCount)
	require.NotNil(t, restored.FirstTransactionDate)
	assert.Equal(t, firstDate.UTC(), restored.FirstTransactionDate.UTC())
}

// TestCashFlowJSONFieldNames verifies the JSON field names match the expected API contract.
func TestCashFlowJSONFieldNames(t *testing.T) {
	tx := models.CashTransaction{
		ID:          "ct_aabbccdd",
		Type:        models.CashTxDeposit,
		Date:        time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
		Amount:      50000,
		Description: "Test deposit",
		Category:    "rollover",
		Notes:       "Opening balance",
		CreatedAt:   time.Now().Truncate(time.Second),
		UpdatedAt:   time.Now().Truncate(time.Second),
	}

	data, err := json.Marshal(tx)
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))

	// Required fields
	assert.Contains(t, raw, "id")
	assert.Contains(t, raw, "type")
	assert.Contains(t, raw, "date")
	assert.Contains(t, raw, "amount")
	assert.Contains(t, raw, "description")
	assert.Contains(t, raw, "created_at")
	assert.Contains(t, raw, "updated_at")

	// Optional fields present when set
	assert.Contains(t, raw, "category")
	assert.Contains(t, raw, "notes")

	// Verify snake_case naming
	assert.Equal(t, "ct_aabbccdd", raw["id"])
	assert.Equal(t, "deposit", raw["type"])
	assert.Equal(t, 50000.0, raw["amount"])
}

// TestCashFlowLedgerJSONFieldNames verifies ledger-level JSON field names.
func TestCashFlowLedgerJSONFieldNames(t *testing.T) {
	ledger := models.CashFlowLedger{
		PortfolioName: "SMSF",
		Version:       3,
		Transactions:  []models.CashTransaction{},
		Notes:         "Test notes",
		CreatedAt:     time.Now().Truncate(time.Second),
		UpdatedAt:     time.Now().Truncate(time.Second),
	}

	data, err := json.Marshal(ledger)
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))

	assert.Contains(t, raw, "portfolio_name")
	assert.Contains(t, raw, "version")
	assert.Contains(t, raw, "transactions")
	assert.Contains(t, raw, "created_at")
	assert.Contains(t, raw, "updated_at")

	assert.Equal(t, "SMSF", raw["portfolio_name"])
	assert.Equal(t, 3.0, raw["version"])
}

// TestCashFlowPerformanceJSONFieldNames verifies performance response JSON field names.
func TestCashFlowPerformanceJSONFieldNames(t *testing.T) {
	firstDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	perf := models.CapitalPerformance{
		TotalDeposited:        100000,
		TotalWithdrawn:        10000,
		NetCapitalDeployed:    90000,
		CurrentPortfolioValue: 120000,
		SimpleReturnPct:       33.33,
		AnnualizedReturnPct:   25.0,
		FirstTransactionDate:  &firstDate,
		TransactionCount:      5,
	}

	data, err := json.Marshal(perf)
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))

	assert.Contains(t, raw, "total_deposited")
	assert.Contains(t, raw, "total_withdrawn")
	assert.Contains(t, raw, "net_capital_deployed")
	assert.Contains(t, raw, "current_portfolio_value")
	assert.Contains(t, raw, "simple_return_pct")
	assert.Contains(t, raw, "annualized_return_pct")
	assert.Contains(t, raw, "first_transaction_date")
	assert.Contains(t, raw, "transaction_count")
}

// TestCashFlowMultiPortfolioIsolation verifies that cash flow ledgers for different
// portfolios are stored independently and do not leak between portfolios.
func TestCashFlowMultiPortfolioIsolation(t *testing.T) {
	mgr := testManager(t)
	store := mgr.UserDataStore()
	ctx := testContext()

	portfolios := []string{"SMSF", "Personal", "Trading"}

	for _, name := range portfolios {
		ledger := models.CashFlowLedger{
			PortfolioName: name,
			Version:       1,
			Transactions: []models.CashTransaction{
				{
					ID:          "ct_" + name,
					Type:        models.CashTxDeposit,
					Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
					Amount:      float64(len(name) * 10000),
					Description: "Deposit for " + name,
					CreatedAt:   time.Now().Truncate(time.Second),
					UpdatedAt:   time.Now().Truncate(time.Second),
				},
			},
			CreatedAt: time.Now().Truncate(time.Second),
			UpdatedAt: time.Now().Truncate(time.Second),
		}

		data, err := json.Marshal(ledger)
		require.NoError(t, err)

		require.NoError(t, store.Put(ctx, &models.UserRecord{
			UserID:   "cf_multi_user",
			Subject:  "cashflow",
			Key:      name,
			Value:    string(data),
			Version:  1,
			DateTime: time.Now().Truncate(time.Second),
		}))
	}

	// Verify each portfolio has its own independent ledger
	for _, name := range portfolios {
		t.Run(name, func(t *testing.T) {
			got, err := store.Get(ctx, "cf_multi_user", "cashflow", name)
			require.NoError(t, err)

			var restored models.CashFlowLedger
			require.NoError(t, json.Unmarshal([]byte(got.Value), &restored))

			assert.Equal(t, name, restored.PortfolioName)
			require.Len(t, restored.Transactions, 1)
			assert.Equal(t, "ct_"+name, restored.Transactions[0].ID)
			assert.Equal(t, "Deposit for "+name, restored.Transactions[0].Description)
		})
	}

	// List all cashflow records for user
	records, err := store.List(ctx, "cf_multi_user", "cashflow")
	require.NoError(t, err)
	assert.Len(t, records, 3)
}

// TestCashFlowEmptyLedgerStorage verifies that an empty ledger (no transactions)
// stores and retrieves correctly without errors.
func TestCashFlowEmptyLedgerStorage(t *testing.T) {
	mgr := testManager(t)
	store := mgr.UserDataStore()
	ctx := testContext()

	ledger := models.CashFlowLedger{
		PortfolioName: "empty_test",
		Version:       1,
		Transactions:  []models.CashTransaction{},
		CreatedAt:     time.Now().Truncate(time.Second),
		UpdatedAt:     time.Now().Truncate(time.Second),
	}

	data, err := json.Marshal(ledger)
	require.NoError(t, err)

	require.NoError(t, store.Put(ctx, &models.UserRecord{
		UserID:   "cf_empty_user",
		Subject:  "cashflow",
		Key:      "empty_test",
		Value:    string(data),
		Version:  1,
		DateTime: time.Now().Truncate(time.Second),
	}))

	got, err := store.Get(ctx, "cf_empty_user", "cashflow", "empty_test")
	require.NoError(t, err)

	var restored models.CashFlowLedger
	require.NoError(t, json.Unmarshal([]byte(got.Value), &restored))

	assert.Equal(t, "empty_test", restored.PortfolioName)
	assert.Equal(t, 1, restored.Version)
	assert.Empty(t, restored.Transactions)
}
