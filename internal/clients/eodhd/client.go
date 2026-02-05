// Package eodhd provides a client for the EODHD API
package eodhd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/interfaces"
	"github.com/bobmccarthy/vire/internal/models"
)

const (
	DefaultBaseURL   = "https://eodhd.com/api"
	DefaultTimeout   = 30 * time.Second
	DefaultRateLimit = 10 // requests per second
)

// Client implements the EODHDClient interface
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

// NewClient creates a new EODHD client
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
	return fmt.Sprintf("EODHD API error: %s (status: %d, endpoint: %s)", e.Message, e.StatusCode, e.Endpoint)
}

// get performs a rate-limited GET request
func (c *Client) get(ctx context.Context, path string, params url.Values, result interface{}) error {
	// Wait for rate limiter
	if err := c.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limit wait: %w", err)
	}

	// Add API key
	if params == nil {
		params = url.Values{}
	}
	params.Set("api_token", c.apiKey)
	params.Set("fmt", "json")

	reqURL := fmt.Sprintf("%s%s?%s", c.baseURL, path, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.logger.Debug().Str("url", c.baseURL+path).Msg("EODHD API request")

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

// GetEOD retrieves end-of-day price data
func (c *Client) GetEOD(ctx context.Context, ticker string, opts ...interfaces.EODOption) (*models.EODResponse, error) {
	params := &interfaces.EODParams{
		Period: "d",
		Order:  "d", // descending (most recent first)
	}

	for _, opt := range opts {
		opt(params)
	}

	urlParams := url.Values{}
	urlParams.Set("period", params.Period)
	urlParams.Set("order", params.Order)

	if !params.From.IsZero() {
		urlParams.Set("from", params.From.Format("2006-01-02"))
	}
	if !params.To.IsZero() {
		urlParams.Set("to", params.To.Format("2006-01-02"))
	}

	path := fmt.Sprintf("/eod/%s", ticker)

	var bars []eodBarResponse
	if err := c.get(ctx, path, urlParams, &bars); err != nil {
		return nil, err
	}

	result := &models.EODResponse{
		Data: make([]models.EODBar, len(bars)),
	}

	for i, bar := range bars {
		date, _ := time.Parse("2006-01-02", bar.Date)
		result.Data[i] = models.EODBar{
			Date:     date,
			Open:     bar.Open,
			High:     bar.High,
			Low:      bar.Low,
			Close:    bar.Close,
			AdjClose: bar.AdjustedClose,
			Volume:   bar.Volume,
		}
	}

	return result, nil
}

// eodBarResponse represents the API response for EOD data
type eodBarResponse struct {
	Date          string  `json:"date"`
	Open          float64 `json:"open"`
	High          float64 `json:"high"`
	Low           float64 `json:"low"`
	Close         float64 `json:"close"`
	AdjustedClose float64 `json:"adjusted_close"`
	Volume        int64   `json:"volume"`
}

// GetFundamentals retrieves fundamental data
func (c *Client) GetFundamentals(ctx context.Context, ticker string) (*models.Fundamentals, error) {
	path := fmt.Sprintf("/fundamentals/%s", ticker)

	var resp fundamentalsResponse
	if err := c.get(ctx, path, nil, &resp); err != nil {
		return nil, err
	}

	// Detect if this is an ETF
	isETF := resp.General.Type == "ETF" ||
		(resp.General.Sector == "" && resp.General.Industry == "" && resp.ETFData.NetExpenseRatio > 0) ||
		strings.Contains(strings.ToUpper(resp.General.Name), " ETF")

	// Determine management style for ETFs
	managementStyle := ""
	if isETF {
		managementStyle = "Passive" // Most ETFs are passive; could enhance later
	}

	fundamentals := &models.Fundamentals{
		Ticker:            ticker,
		MarketCap:         resp.Highlights.MarketCapitalization,
		PE:                resp.Highlights.PERatio,
		PB:                resp.Valuation.PriceBookMRQ,
		EPS:               resp.Highlights.EarningsShare,
		DividendYield:     resp.Highlights.DividendYield,
		Beta:              resp.Technicals.Beta,
		SharesOutstanding: int64(resp.SharesStats.SharesOutstanding),
		SharesFloat:       int64(resp.SharesStats.SharesFloat),
		Sector:            resp.General.Sector,
		Industry:          resp.General.Industry,
		Description:       resp.General.Description,
		LastUpdated:       time.Now(),
		IsETF:             isETF,
		ExpenseRatio:      resp.ETFData.NetExpenseRatio,
		ManagementStyle:   managementStyle,
	}

	// Extract ETF holdings if available
	if isETF && len(resp.ETFData.Holdings) > 0 {
		holdings := make([]models.ETFHolding, 0, len(resp.ETFData.Holdings))
		for ticker, h := range resp.ETFData.Holdings {
			holdings = append(holdings, models.ETFHolding{
				Ticker: ticker,
				Name:   h.Name,
				Weight: h.AssetsPercent,
			})
		}
		// Sort by weight descending
		for i := 0; i < len(holdings)-1; i++ {
			for j := i + 1; j < len(holdings); j++ {
				if holdings[j].Weight > holdings[i].Weight {
					holdings[i], holdings[j] = holdings[j], holdings[i]
				}
			}
		}
		// Keep top 10
		if len(holdings) > 10 {
			holdings = holdings[:10]
		}
		fundamentals.TopHoldings = holdings
	}

	// Extract sector weights if available
	if isETF && len(resp.ETFData.SectorWeights) > 0 {
		sectors := make([]models.SectorWeight, 0, len(resp.ETFData.SectorWeights))
		for sector, w := range resp.ETFData.SectorWeights {
			if w.EquityPercent > 0 {
				sectors = append(sectors, models.SectorWeight{
					Sector: sector,
					Weight: w.EquityPercent,
				})
			}
		}
		// Sort by weight descending
		for i := 0; i < len(sectors)-1; i++ {
			for j := i + 1; j < len(sectors); j++ {
				if sectors[j].Weight > sectors[i].Weight {
					sectors[i], sectors[j] = sectors[j], sectors[i]
				}
			}
		}
		fundamentals.SectorWeights = sectors
	}

	// Extract country weights if available
	if isETF && len(resp.ETFData.WorldRegions) > 0 {
		countries := make([]models.CountryWeight, 0, len(resp.ETFData.WorldRegions))
		for country, w := range resp.ETFData.WorldRegions {
			if w.EquityPercent > 0 {
				countries = append(countries, models.CountryWeight{
					Country: country,
					Weight:  w.EquityPercent,
				})
			}
		}
		// Sort by weight descending
		for i := 0; i < len(countries)-1; i++ {
			for j := i + 1; j < len(countries); j++ {
				if countries[j].Weight > countries[i].Weight {
					countries[i], countries[j] = countries[j], countries[i]
				}
			}
		}
		fundamentals.CountryWeights = countries
	}

	return fundamentals, nil
}

// fundamentalsResponse represents the API response structure
type fundamentalsResponse struct {
	General struct {
		Code        string `json:"Code"`
		Name        string `json:"Name"`
		Type        string `json:"Type"` // "Common Stock", "ETF", etc.
		Sector      string `json:"Sector"`
		Industry    string `json:"Industry"`
		Description string `json:"Description"`
	} `json:"General"`
	Highlights struct {
		MarketCapitalization float64 `json:"MarketCapitalization"`
		PERatio              float64 `json:"PERatio"`
		EarningsShare        float64 `json:"EarningsShare"`
		DividendYield        float64 `json:"DividendYield"`
	} `json:"Highlights"`
	Valuation struct {
		PriceBookMRQ float64 `json:"PriceBookMRQ"`
	} `json:"Valuation"`
	SharesStats struct {
		SharesOutstanding float64 `json:"SharesOutstanding"`
		SharesFloat       float64 `json:"SharesFloat"`
	} `json:"SharesStats"`
	Technicals struct {
		Beta float64 `json:"Beta"`
	} `json:"Technicals"`
	// ETF-specific data from EODHD
	ETFData struct {
		NetExpenseRatio        float64 `json:"Net_Expense_Ratio"`
		AnnualHoldingsTurnover float64 `json:"Annual_Holdings_Turnover"`
		Holdings               map[string]struct {
			Name           string  `json:"Name"`
			AssetsPercent  float64 `json:"Assets_%"`
		} `json:"Holdings"`
		SectorWeights map[string]struct {
			EquityPercent float64 `json:"Equity_%"`
		} `json:"Sector_Weights"`
		WorldRegions map[string]struct {
			EquityPercent float64 `json:"Equity_%"`
		} `json:"World_Regions"`
	} `json:"ETF_Data"`
}

// GetTechnicals retrieves technical indicators
func (c *Client) GetTechnicals(ctx context.Context, ticker string, function string) (*models.TechnicalResponse, error) {
	path := fmt.Sprintf("/technical/%s", ticker)

	params := url.Values{}
	params.Set("function", function)

	var data []map[string]interface{}
	if err := c.get(ctx, path, params, &data); err != nil {
		return nil, err
	}

	result := make(map[string]interface{})
	if len(data) > 0 {
		result = data[0]
	}

	return &models.TechnicalResponse{
		Data: result,
	}, nil
}

// GetNews retrieves news for a ticker
func (c *Client) GetNews(ctx context.Context, ticker string, limit int) ([]*models.NewsItem, error) {
	path := "/news"

	params := url.Values{}
	params.Set("s", ticker)
	params.Set("limit", strconv.Itoa(limit))

	var newsResp []newsResponse
	if err := c.get(ctx, path, params, &newsResp); err != nil {
		return nil, err
	}

	news := make([]*models.NewsItem, len(newsResp))
	for i, item := range newsResp {
		publishedAt, _ := time.Parse("2006-01-02T15:04:05+00:00", item.Date)
		news[i] = &models.NewsItem{
			Title:       item.Title,
			URL:         item.Link,
			Source:      item.Source,
			PublishedAt: publishedAt,
			Sentiment:   item.Sentiment,
		}
	}

	return news, nil
}

type newsResponse struct {
	Date      string `json:"date"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Link      string `json:"link"`
	Source    string `json:"source"`
	Sentiment string `json:"sentiment"`
}

// GetExchangeSymbols retrieves all symbols for an exchange
func (c *Client) GetExchangeSymbols(ctx context.Context, exchange string) ([]*models.Symbol, error) {
	path := fmt.Sprintf("/exchange-symbol-list/%s", exchange)

	var symbols []models.Symbol
	if err := c.get(ctx, path, nil, &symbols); err != nil {
		return nil, err
	}

	result := make([]*models.Symbol, len(symbols))
	for i := range symbols {
		result[i] = &symbols[i]
	}

	return result, nil
}

// Ensure Client implements EODHDClient
var _ interfaces.EODHDClient = (*Client)(nil)
