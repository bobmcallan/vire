package navexa

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetHoldingTrades_NormalizesSellUnits(t *testing.T) {
	// Simulate Navexa API returning negative quantities for sells
	trades := []tradeData{
		{ID: 1, HoldingID: 100, Symbol: "ETPMAG", TradeType: "buy", TradeDate: "2023-01-10", Quantity: 179, Price: 111.22, Brokerage: 3.00, Value: 19908.38, CurrencyCode: "AUD"},
		{ID: 2, HoldingID: 100, Symbol: "ETPMAG", TradeType: "sell", TradeDate: "2024-03-15", Quantity: -175, Price: 152.39, Brokerage: 3.00, Value: -26668.25, CurrencyCode: "AUD"},
		{ID: 3, HoldingID: 100, Symbol: "ETPMAG", TradeType: "sell", TradeDate: "2024-06-20", Quantity: -65, Price: 152.22, Brokerage: 3.00, Value: -9894.30, CurrencyCode: "AUD"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(trades)
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	result, err := client.GetHoldingTrades(context.Background(), "100")
	if err != nil {
		t.Fatalf("GetHoldingTrades returned error: %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("expected 3 trades, got %d", len(result))
	}

	// Buy units should remain positive
	if result[0].Units != 179 {
		t.Errorf("buy units = %v, want 179", result[0].Units)
	}

	// Sell units should be normalized to positive (abs of -175)
	if result[1].Units != 175 {
		t.Errorf("sell units = %v, want 175 (was -175 from API)", result[1].Units)
	}

	// Sell units should be normalized to positive (abs of -65)
	if result[2].Units != 65 {
		t.Errorf("sell units = %v, want 65 (was -65 from API)", result[2].Units)
	}
}
