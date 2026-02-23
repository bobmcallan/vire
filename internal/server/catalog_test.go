package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

func TestBuildToolCatalog_ReturnsAllTools(t *testing.T) {
	catalog := buildToolCatalog()
	if len(catalog) != 47 {
		names := make([]string, len(catalog))
		for i, td := range catalog {
			names[i] = td.Name
		}
		t.Fatalf("expected 47 tools, got %d: %v", len(catalog), names)
	}
}

func TestBuildToolCatalog_AllToolsHaveRequiredFields(t *testing.T) {
	for _, td := range buildToolCatalog() {
		if td.Name == "" {
			t.Error("tool has empty name")
		}
		if td.Description == "" {
			t.Errorf("tool %q has empty description", td.Name)
		}
		if td.Method == "" {
			t.Errorf("tool %q has empty method", td.Name)
		}
		if td.Path == "" {
			t.Errorf("tool %q has empty path", td.Name)
		}
	}
}

func TestBuildToolCatalog_UniqueNames(t *testing.T) {
	seen := make(map[string]bool)
	for _, td := range buildToolCatalog() {
		if seen[td.Name] {
			t.Errorf("duplicate tool name: %q", td.Name)
		}
		seen[td.Name] = true
	}
}

func TestBuildToolCatalog_ValidMethods(t *testing.T) {
	validMethods := map[string]bool{
		"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true,
	}
	for _, td := range buildToolCatalog() {
		if !validMethods[td.Method] {
			t.Errorf("tool %q has invalid method %q", td.Name, td.Method)
		}
	}
}

func TestBuildToolCatalog_ParamLocations(t *testing.T) {
	validLocations := map[string]bool{
		"path": true, "query": true, "body": true,
	}
	for _, td := range buildToolCatalog() {
		for _, p := range td.Params {
			if !validLocations[p.In] {
				t.Errorf("tool %q param %q has invalid location %q", td.Name, p.Name, p.In)
			}
		}
	}
}

func TestBuildToolCatalog_ParamsHaveRequiredFields(t *testing.T) {
	for _, td := range buildToolCatalog() {
		for _, p := range td.Params {
			if p.Name == "" {
				t.Errorf("tool %q has param with empty name", td.Name)
			}
			if p.Type == "" {
				t.Errorf("tool %q param %q has empty type", td.Name, p.Name)
			}
			if p.Description == "" {
				t.Errorf("tool %q param %q has empty description", td.Name, p.Name)
			}
		}
	}
}

func TestBuildToolCatalog_KnownToolsPresent(t *testing.T) {
	catalog := buildToolCatalog()
	names := make(map[string]bool)
	for _, td := range catalog {
		names[td.Name] = true
	}

	expected := []string{
		"get_version", "get_config", "get_diagnostics",
		"submit_feedback",
		"list_users", "update_user_role",
		"list_portfolios", "set_default_portfolio",
		"get_portfolio", "get_portfolio_stock",
		"portfolio_compliance", "generate_report", "get_summary",
		"get_portfolio_strategy", "set_portfolio_strategy", "delete_portfolio_strategy",
		"get_portfolio_plan", "set_portfolio_plan",
		"add_plan_item", "update_plan_item", "remove_plan_item", "check_plan_status",
		"get_quote", "get_stock_data", "compute_indicators",
		"strategy_scanner", "stock_screen",
		"list_reports", "get_strategy_template",
	}

	for _, name := range expected {
		if !names[name] {
			t.Errorf("expected tool %q not found in catalog", name)
		}
	}
}

func TestHandleToolCatalog_ReturnsJSON(t *testing.T) {
	srv := newTestServer(&mockPortfolioService{})

	req := httptest.NewRequest(http.MethodGet, "/api/mcp/tools", nil)
	rec := httptest.NewRecorder()

	srv.handleToolCatalog(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", contentType)
	}

	var catalog []models.ToolDefinition
	if err := json.NewDecoder(rec.Body).Decode(&catalog); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(catalog) != 47 {
		t.Errorf("expected 47 tools in response, got %d", len(catalog))
	}
}

func TestHandleToolCatalog_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(&mockPortfolioService{})

	req := httptest.NewRequest(http.MethodPost, "/api/mcp/tools", nil)
	rec := httptest.NewRecorder()

	srv.handleToolCatalog(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", rec.Code)
	}
}

func TestHandlePortfolioStock_Found(t *testing.T) {
	portfolio := &models.Portfolio{
		Name:       "test",
		TotalValue: 100000,
		LastSynced: time.Now(),
		Holdings: []models.Holding{
			{Ticker: "BHP", Exchange: "ASX", Name: "BHP Group", Units: 100, MarketValue: 5000},
			{Ticker: "CBA", Exchange: "ASX", Name: "Commonwealth Bank", Units: 50, MarketValue: 7500},
		},
	}

	svc := &mockPortfolioService{
		getPortfolio: func(ctx context.Context, name string) (*models.Portfolio, error) {
			return portfolio, nil
		},
	}

	srv := newTestServer(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/test/stock/BHP", nil)
	rec := httptest.NewRecorder()

	srv.handlePortfolioStock(rec, req, "test", "BHP")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var holding models.Holding
	if err := json.NewDecoder(rec.Body).Decode(&holding); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if holding.Ticker != "BHP" {
		t.Errorf("expected ticker BHP, got %q", holding.Ticker)
	}
	if holding.Name != "BHP Group" {
		t.Errorf("expected name 'BHP Group', got %q", holding.Name)
	}
}

func TestHandlePortfolioStock_FoundByEODHDTicker(t *testing.T) {
	portfolio := &models.Portfolio{
		Name: "test",
		Holdings: []models.Holding{
			{Ticker: "BHP", Exchange: "ASX", Name: "BHP Group"},
		},
	}

	svc := &mockPortfolioService{
		getPortfolio: func(ctx context.Context, name string) (*models.Portfolio, error) {
			return portfolio, nil
		},
	}

	srv := newTestServer(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/test/stock/BHP.AU", nil)
	rec := httptest.NewRecorder()

	srv.handlePortfolioStock(rec, req, "test", "BHP.AU")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var holding models.Holding
	if err := json.NewDecoder(rec.Body).Decode(&holding); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if holding.Ticker != "BHP" {
		t.Errorf("expected ticker BHP, got %q", holding.Ticker)
	}
}

func TestHandlePortfolioStock_NotFound(t *testing.T) {
	portfolio := &models.Portfolio{
		Name: "test",
		Holdings: []models.Holding{
			{Ticker: "BHP", Exchange: "ASX"},
		},
	}

	svc := &mockPortfolioService{
		getPortfolio: func(ctx context.Context, name string) (*models.Portfolio, error) {
			return portfolio, nil
		},
	}

	srv := newTestServer(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/test/stock/MISSING", nil)
	rec := httptest.NewRecorder()

	srv.handlePortfolioStock(rec, req, "test", "MISSING")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}
}

func TestHandlePortfolioStock_PortfolioNotFound(t *testing.T) {
	svc := &mockPortfolioService{
		getPortfolio: func(ctx context.Context, name string) (*models.Portfolio, error) {
			return nil, errors.New("not found")
		},
	}

	srv := newTestServer(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/missing/stock/BHP", nil)
	rec := httptest.NewRecorder()

	srv.handlePortfolioStock(rec, req, "missing", "BHP")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}
}

func TestMatchHoldingTicker(t *testing.T) {
	h := &models.Holding{Ticker: "BHP", Exchange: "ASX"}

	tests := []struct {
		input string
		want  bool
	}{
		{"BHP", true},
		{"bhp", true},
		{"BHP.AU", true},
		{"bhp.au", true},
		{"CBA", false},
		{"CBA.AU", false},
		{"BH", false},
		{"BHPP", false},
	}

	for _, tt := range tests {
		got := matchHoldingTicker(tt.input, h)
		if got != tt.want {
			t.Errorf("matchHoldingTicker(%q, BHP/ASX) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
