// Package asx provides a client for the ASX Markit Digital API
package asx

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

const (
	DefaultBaseURL   = "https://asx.api.markitdigital.com/asx-research/1.0"
	DefaultTimeout   = 30 * time.Second
	DefaultRateLimit = 5 // requests per second
)

// Client implements the ASXClient interface using the Markit Digital API
type Client struct {
	baseURL    string
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

// NewClient creates a new ASX Markit Digital API client.
// No API key is required — this is a public endpoint.
func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		baseURL: DefaultBaseURL,
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

// headerResponse represents the ASX Markit Digital header API response.
// The endpoint only reliably returns priceLast, priceChange, priceChangePercent,
// and volume. Fields like priceOpen, priceHigh, priceLow are NOT returned.
type headerResponse struct {
	Data struct {
		PriceLast          float64 `json:"priceLast"`
		PriceChange        float64 `json:"priceChange"`
		PriceChangePercent float64 `json:"priceChangePercent"`
		Volume             int64   `json:"volume"`
	} `json:"data"`
}

// GetRealTimeQuote retrieves a live price snapshot for an ASX-listed ticker.
// The ticker should be in EODHD format (e.g. "BHP.AU") — the .AU suffix is
// stripped before calling the ASX API.
func (c *Client) GetRealTimeQuote(ctx context.Context, ticker string) (*models.RealTimeQuote, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	// Strip exchange suffix: "BHP.AU" → "BHP", "ETPMAG.AU" → "ETPMAG"
	code := ticker
	if idx := strings.Index(ticker, "."); idx > 0 {
		code = ticker[:idx]
	}

	reqURL := fmt.Sprintf("%s/companies/%s/header", c.baseURL, strings.ToLower(code))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")

	c.logger.Debug().Str("ticker", code).Msg("ASX Markit API request")

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		c.logger.Error().Err(err).Str("ticker", code).Dur("elapsed", elapsed).Msg("ASX Markit API request failed")
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.logger.Warn().Str("ticker", code).Int("status", resp.StatusCode).Dur("elapsed", elapsed).Msg("ASX Markit API non-OK response")
		return nil, fmt.Errorf("ASX Markit API error: status %d for ticker %s", resp.StatusCode, code)
	}

	var apiResp headerResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	d := apiResp.Data

	c.logger.Info().Str("ticker", code).Float64("price", d.PriceLast).Int("status", resp.StatusCode).Dur("elapsed", elapsed).Msg("ASX Markit API call")

	return &models.RealTimeQuote{
		Code:      ticker,
		Close:     d.PriceLast,
		Change:    d.PriceChange,
		ChangePct: d.PriceChangePercent,
		Volume:    d.Volume,
		Timestamp: time.Now(),
		Source:    "asx",
	}, nil
}

// Ensure Client implements ASXClient
var _ interfaces.ASXClient = (*Client)(nil)
