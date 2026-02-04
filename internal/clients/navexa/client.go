// Package navexa provides a client for the Navexa API
package navexa

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/time/rate"

	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/interfaces"
	"github.com/bobmccarthy/vire/internal/models"
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

// get performs a rate-limited GET request
func (c *Client) get(ctx context.Context, path string, result interface{}) error {
	// Wait for rate limiter
	if err := c.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limit wait: %w", err)
	}

	reqURL := c.baseURL + path

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")

	c.logger.Debug().Str("url", path).Msg("Navexa API request")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    string(body),
			Endpoint:   path,
		}
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

// GetPortfolios retrieves all portfolios
func (c *Client) GetPortfolios(ctx context.Context) ([]*models.NavexaPortfolio, error) {
	var resp portfoliosResponse
	if err := c.get(ctx, "/v1/portfolios", &resp); err != nil {
		return nil, err
	}

	portfolios := make([]*models.NavexaPortfolio, len(resp.Data))
	for i, p := range resp.Data {
		portfolios[i] = &models.NavexaPortfolio{
			ID:           p.ID,
			Name:         p.Name,
			Currency:     p.Currency,
			TotalValue:   p.TotalValue,
			TotalCost:    p.TotalCost,
			TotalGain:    p.TotalGain,
			TotalGainPct: p.TotalGainPct,
		}
	}

	return portfolios, nil
}

type portfoliosResponse struct {
	Data []struct {
		ID           string  `json:"id"`
		Name         string  `json:"name"`
		Currency     string  `json:"currency"`
		TotalValue   float64 `json:"total_value"`
		TotalCost    float64 `json:"total_cost"`
		TotalGain    float64 `json:"total_gain"`
		TotalGainPct float64 `json:"total_gain_pct"`
	} `json:"data"`
}

// GetPortfolio retrieves a specific portfolio by ID
func (c *Client) GetPortfolio(ctx context.Context, portfolioID string) (*models.NavexaPortfolio, error) {
	var resp portfolioResponse
	path := fmt.Sprintf("/v1/portfolios/%s", portfolioID)
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}

	return &models.NavexaPortfolio{
		ID:           resp.Data.ID,
		Name:         resp.Data.Name,
		Currency:     resp.Data.Currency,
		TotalValue:   resp.Data.TotalValue,
		TotalCost:    resp.Data.TotalCost,
		TotalGain:    resp.Data.TotalGain,
		TotalGainPct: resp.Data.TotalGainPct,
	}, nil
}

type portfolioResponse struct {
	Data struct {
		ID           string  `json:"id"`
		Name         string  `json:"name"`
		Currency     string  `json:"currency"`
		TotalValue   float64 `json:"total_value"`
		TotalCost    float64 `json:"total_cost"`
		TotalGain    float64 `json:"total_gain"`
		TotalGainPct float64 `json:"total_gain_pct"`
	} `json:"data"`
}

// GetHoldings retrieves holdings for a portfolio
func (c *Client) GetHoldings(ctx context.Context, portfolioID string) ([]*models.NavexaHolding, error) {
	var resp holdingsResponse
	path := fmt.Sprintf("/v1/portfolios/%s/holdings", portfolioID)
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}

	holdings := make([]*models.NavexaHolding, len(resp.Data))
	for i, h := range resp.Data {
		holdings[i] = &models.NavexaHolding{
			ID:            h.ID,
			PortfolioID:   portfolioID,
			Ticker:        h.Ticker,
			Exchange:      h.Exchange,
			Name:          h.Name,
			Units:         h.Units,
			AvgCost:       h.AvgCost,
			TotalCost:     h.TotalCost,
			CurrentPrice:  h.CurrentPrice,
			MarketValue:   h.MarketValue,
			GainLoss:      h.GainLoss,
			GainLossPct:   h.GainLossPct,
			DividendYield: h.DividendYield,
			LastUpdated:   time.Now(),
		}
	}

	return holdings, nil
}

type holdingsResponse struct {
	Data []struct {
		ID            string  `json:"id"`
		Ticker        string  `json:"ticker"`
		Exchange      string  `json:"exchange"`
		Name          string  `json:"name"`
		Units         float64 `json:"units"`
		AvgCost       float64 `json:"avg_cost"`
		TotalCost     float64 `json:"total_cost"`
		CurrentPrice  float64 `json:"current_price"`
		MarketValue   float64 `json:"market_value"`
		GainLoss      float64 `json:"gain_loss"`
		GainLossPct   float64 `json:"gain_loss_pct"`
		DividendYield float64 `json:"dividend_yield"`
	} `json:"data"`
}

// GetPerformance retrieves portfolio performance metrics
func (c *Client) GetPerformance(ctx context.Context, portfolioID string) (*models.NavexaPerformance, error) {
	var resp performanceResponse
	path := fmt.Sprintf("/v1/portfolios/%s/performance", portfolioID)
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}

	return &models.NavexaPerformance{
		PortfolioID:      portfolioID,
		TotalReturn:      resp.Data.TotalReturn,
		TotalReturnPct:   resp.Data.TotalReturnPct,
		AnnualisedReturn: resp.Data.AnnualisedReturn,
		Volatility:       resp.Data.Volatility,
		SharpeRatio:      resp.Data.SharpeRatio,
		MaxDrawdown:      resp.Data.MaxDrawdown,
		PeriodReturns:    resp.Data.PeriodReturns,
		AsOfDate:         time.Now(),
	}, nil
}

type performanceResponse struct {
	Data struct {
		TotalReturn      float64            `json:"total_return"`
		TotalReturnPct   float64            `json:"total_return_pct"`
		AnnualisedReturn float64            `json:"annualised_return"`
		Volatility       float64            `json:"volatility"`
		SharpeRatio      float64            `json:"sharpe_ratio"`
		MaxDrawdown      float64            `json:"max_drawdown"`
		PeriodReturns    map[string]float64 `json:"period_returns"`
	} `json:"data"`
}

// Ensure Client implements NavexaClient
var _ interfaces.NavexaClient = (*Client)(nil)
