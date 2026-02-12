// Package navexa provides a client for the Navexa API
package navexa

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/time/rate"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

const (
	DefaultBaseURL   = "https://api.navexa.com.au"
	DefaultTimeout   = 30 * time.Second
	DefaultRateLimit = 5 // requests per second
)

// Client implements the NavexaClient interface
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	logger     *common.Logger
	limiter    *rate.Limiter
}

// ClientOption configures the client
type ClientOption func(*Client)

// WithBaseURL sets the base URL
func WithBaseURL(baseURL string) ClientOption {
	return func(c *Client) {
		c.baseURL = baseURL
	}
}

// WithLogger sets the logger
func WithLogger(logger *common.Logger) ClientOption {
	return func(c *Client) {
		c.logger = logger
	}
}

// WithRateLimit sets the rate limit
func WithRateLimit(requestsPerSecond int) ClientOption {
	return func(c *Client) {
		c.limiter = rate.NewLimiter(rate.Limit(requestsPerSecond), requestsPerSecond)
	}
}

// WithTimeout sets the HTTP timeout
func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.httpClient.Timeout = timeout
	}
}

// NewClient creates a new Navexa client
func NewClient(apiKey string, opts ...ClientOption) *Client {
	c := &Client{
		baseURL: DefaultBaseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		limiter: rate.NewLimiter(rate.Limit(DefaultRateLimit), DefaultRateLimit),
		logger:  common.NewSilentLogger(),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// APIError represents an API error
type APIError struct {
	StatusCode int
	Message    string
	Endpoint   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("Navexa API error: %s (status: %d, endpoint: %s)", e.Message, e.StatusCode, e.Endpoint)
}

// get performs a rate-limited GET request with optional query parameters
func (c *Client) get(ctx context.Context, path string, params url.Values, result interface{}) error {
	// Wait for rate limiter
	if err := c.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limit wait: %w", err)
	}

	reqURL := c.baseURL + path
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")

	c.logger.Debug().Str("url", path).Msg("Navexa API request")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    string(body),
			Endpoint:   path,
		}
	}

	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

// GetPortfolios retrieves all portfolios
func (c *Client) GetPortfolios(ctx context.Context) ([]*models.NavexaPortfolio, error) {
	var resp []portfolioData
	if err := c.get(ctx, "/v1/portfolios", nil, &resp); err != nil {
		return nil, err
	}

	portfolios := make([]*models.NavexaPortfolio, len(resp))
	for i, p := range resp {
		portfolios[i] = &models.NavexaPortfolio{
			ID:          fmt.Sprintf("%d", p.ID),
			Name:        p.Name,
			Currency:    p.BaseCurrencyCode,
			DateCreated: p.DateCreated,
		}
	}

	return portfolios, nil
}

type portfolioData struct {
	ID               int    `json:"id"`
	Name             string `json:"name"`
	DateCreated      string `json:"dateCreated"`
	BaseCurrencyCode string `json:"baseCurrencyCode"`
}

// GetPortfolio retrieves a specific portfolio by ID
func (c *Client) GetPortfolio(ctx context.Context, portfolioID string) (*models.NavexaPortfolio, error) {
	var resp portfolioData
	path := fmt.Sprintf("/v1/portfolios/%s", portfolioID)
	if err := c.get(ctx, path, nil, &resp); err != nil {
		return nil, err
	}

	return &models.NavexaPortfolio{
		ID:          fmt.Sprintf("%d", resp.ID),
		Name:        resp.Name,
		Currency:    resp.BaseCurrencyCode,
		DateCreated: resp.DateCreated,
	}, nil
}

// GetHoldings retrieves holdings for a portfolio
func (c *Client) GetHoldings(ctx context.Context, portfolioID string) ([]*models.NavexaHolding, error) {
	var resp []holdingData
	path := fmt.Sprintf("/v1/portfolios/%s/holdings", portfolioID)
	if err := c.get(ctx, path, nil, &resp); err != nil {
		return nil, err
	}

	holdings := make([]*models.NavexaHolding, len(resp))
	for i, h := range resp {
		holdings[i] = &models.NavexaHolding{
			ID:          fmt.Sprintf("%d", h.ID),
			PortfolioID: portfolioID,
			Ticker:      h.Symbol,
			Exchange:    h.DisplayExchange,
			Name:        h.Name,
			LastUpdated: time.Now(),
		}
	}

	return holdings, nil
}

type holdingData struct {
	ID              int    `json:"id"`
	Symbol          string `json:"symbol"`
	Exchange        string `json:"exchange"`
	DisplayExchange string `json:"displayExchange"`
	Name            string `json:"name"`
	CurrencyCode    string `json:"currencyCode"`
	HoldingTypeID   int    `json:"holdingTypeId"`
	PortfolioID     int    `json:"portfolioId"`
}

// GetPerformance retrieves portfolio performance metrics with holding-level detail
func (c *Client) GetPerformance(ctx context.Context, portfolioID, fromDate, toDate string) (*models.NavexaPerformance, error) {
	path := fmt.Sprintf("/v1/portfolios/%s/performance", portfolioID)

	params := url.Values{}
	params.Set("from", fromDate)
	params.Set("to", toDate)
	params.Set("isPortfolioGroup", "false")
	params.Set("groupBy", "holding")
	params.Set("showLocalCurrency", "false")

	var resp performanceResponse
	if err := c.get(ctx, path, params, &resp); err != nil {
		return nil, err
	}

	// Derive cost from active holdings only (portfolio-level totalValue includes closed positions)
	var totalCost float64
	for _, h := range resp.Holdings {
		if h.TotalQuantity > 0 {
			totalCost += h.TotalReturn.TotalValue - h.TotalReturn.CapitalGain
		}
	}

	return &models.NavexaPerformance{
		PortfolioID:    portfolioID,
		TotalValue:     resp.TotalReturn.TotalValue,
		TotalCost:      totalCost,
		TotalReturn:    resp.TotalReturn.CapitalGain + resp.TotalReturn.DividendReturn,
		TotalReturnPct: resp.TotalReturn.ReturnPct,
		AsOfDate:       time.Now(),
	}, nil
}

// GetEnrichedHoldings retrieves holdings with financial data via the performance endpoint
func (c *Client) GetEnrichedHoldings(ctx context.Context, portfolioID, fromDate, toDate string) ([]*models.NavexaHolding, error) {
	path := fmt.Sprintf("/v1/portfolios/%s/performance", portfolioID)

	params := url.Values{}
	params.Set("from", fromDate)
	params.Set("to", toDate)
	params.Set("isPortfolioGroup", "false")
	params.Set("groupBy", "holding")
	params.Set("showLocalCurrency", "false")

	c.logger.Debug().
		Str("portfolio_id", portfolioID).
		Str("from", fromDate).
		Str("to", toDate).
		Str("path", path).
		Msg("GetEnrichedHoldings: calling Navexa performance API")

	var resp performanceResponse
	if err := c.get(ctx, path, params, &resp); err != nil {
		return nil, err
	}

	c.logger.Debug().
		Int("holdings_count", len(resp.Holdings)).
		Msg("GetEnrichedHoldings: received response")

	holdings := make([]*models.NavexaHolding, 0, len(resp.Holdings))
	for _, h := range resp.Holdings {
		exchange := h.DisplayExchange
		if exchange == "" {
			exchange = h.Exchange
		}

		// Market value from real price × quantity (closed positions: 0 × price = 0)
		marketValue := h.CurrentPrice * h.TotalQuantity

		c.logger.Debug().
			Str("ticker", h.Symbol).
			Float64("current_price", h.CurrentPrice).
			Float64("quantity", h.TotalQuantity).
			Float64("market_value", marketValue).
			Msg("GetEnrichedHoldings: holding price from Navexa")

		holdings = append(holdings, &models.NavexaHolding{
			ID:               fmt.Sprintf("%d", h.ID),
			PortfolioID:      portfolioID,
			Ticker:           h.Symbol,
			Exchange:         exchange,
			Name:             h.Name,
			Units:            h.TotalQuantity,
			CurrentPrice:     h.CurrentPrice,
			MarketValue:      marketValue,
			DividendReturn:   h.TotalReturn.Dividends,
			GainLoss:         h.TotalReturn.CapitalGain,
			GainLossPct:      h.TotalReturn.CapitalGainPct, // IRR p.a. from Navexa
			CapitalGainPct:   h.TotalReturn.CapitalGainPct, // IRR p.a. from Navexa
			TotalReturnValue: h.TotalReturn.CapitalGain + h.TotalReturn.Dividends,
			TotalReturnPct:   h.TotalReturn.ReturnPct, // IRR p.a. from Navexa
			Currency:         h.CurrencyCode,
			// Cost fields and TWRR are calculated in SyncPortfolio
			LastUpdated: time.Now(),
		})
	}

	return holdings, nil
}

type performanceHolding struct {
	ID              int     `json:"id"`
	Symbol          string  `json:"symbol"`
	Name            string  `json:"name"`
	Exchange        string  `json:"exchange"`
	DisplayExchange string  `json:"displayExchange"`
	CurrentPrice    float64 `json:"currentPrice"`
	TotalQuantity   float64 `json:"totalQuantity"`
	HoldingWeight   float64 `json:"holdingWeight"`
	CurrencyCode    string  `json:"currencyCode"`
	TotalReturn     struct {
		TotalValue     float64 `json:"totalValue"`
		CapitalGain    float64 `json:"capitalGainValue"`
		CapitalGainPct float64 `json:"capitalGainPercent"`
		Dividends      float64 `json:"dividendReturnValue"`
		ReturnPct      float64 `json:"totalReturnPercent"`
	} `json:"totalReturn"`
}

type performanceTotalReturn struct {
	TotalValue     float64 `json:"totalValue"`
	CapitalGain    float64 `json:"capitalGainValue"`
	DividendReturn float64 `json:"dividendReturnValue"`
	ReturnPct      float64 `json:"totalReturnPercent"`
}

type performanceResponse struct {
	Holdings    []performanceHolding   `json:"holdings"`
	TotalReturn performanceTotalReturn `json:"totalReturn"`
}

// GetHoldingTrades retrieves all trades for a specific holding
func (c *Client) GetHoldingTrades(ctx context.Context, holdingID string) ([]*models.NavexaTrade, error) {
	var resp []tradeData
	path := fmt.Sprintf("/v1/holdings/%s/trades", holdingID)
	if err := c.get(ctx, path, nil, &resp); err != nil {
		return nil, err
	}

	trades := make([]*models.NavexaTrade, len(resp))
	for i, t := range resp {
		trades[i] = &models.NavexaTrade{
			ID:        fmt.Sprintf("%d", t.ID),
			HoldingID: fmt.Sprintf("%d", t.HoldingID),
			Symbol:    t.Symbol,
			Type:      t.TradeType,
			Date:      t.TradeDate,
			Units:     math.Abs(t.Quantity),
			Price:     t.Price,
			Fees:      t.Brokerage,
			Value:     t.Value,
			Currency:  t.CurrencyCode,
		}
	}

	return trades, nil
}

type tradeData struct {
	ID           int     `json:"id"`
	HoldingID    int     `json:"holdingId"`
	Symbol       string  `json:"symbol"`
	TradeType    string  `json:"tradeType"`
	TradeDate    string  `json:"tradeDate"`
	Quantity     float64 `json:"quantity"`
	Price        float64 `json:"price"`
	Brokerage    float64 `json:"brokerage"`
	Value        float64 `json:"value"`
	CurrencyCode string  `json:"currencyCode"`
}

// Ensure Client implements NavexaClient
var _ interfaces.NavexaClient = (*Client)(nil)
