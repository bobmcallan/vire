// Package common provides shared test infrastructure
package common

import (
	"context"
	"time"

	"github.com/bobmccarthy/vire/internal/interfaces"
	"github.com/bobmccarthy/vire/internal/models"
)

// MockEODHDClient implements EODHDClient for testing
type MockEODHDClient struct {
	EODData      map[string]*models.EODResponse
	Fundamentals map[string]*models.Fundamentals
	News         map[string][]*models.NewsItem
	Symbols      map[string][]*models.Symbol
	GetEODCalls  int
	GetFundCalls int
	GetNewsCalls int
}

// NewMockEODHDClient creates a mock EODHD client
func NewMockEODHDClient() *MockEODHDClient {
	return &MockEODHDClient{
		EODData:      make(map[string]*models.EODResponse),
		Fundamentals: make(map[string]*models.Fundamentals),
		News:         make(map[string][]*models.NewsItem),
		Symbols:      make(map[string][]*models.Symbol),
	}
}

func (m *MockEODHDClient) GetEOD(ctx context.Context, ticker string, opts ...interfaces.EODOption) (*models.EODResponse, error) {
	m.GetEODCalls++
	if data, ok := m.EODData[ticker]; ok {
		return data, nil
	}
	// Return sample data
	return &models.EODResponse{
		Data: generateSampleBars(365),
	}, nil
}

func (m *MockEODHDClient) GetFundamentals(ctx context.Context, ticker string) (*models.Fundamentals, error) {
	m.GetFundCalls++
	if data, ok := m.Fundamentals[ticker]; ok {
		return data, nil
	}
	return &models.Fundamentals{
		Ticker:    ticker,
		MarketCap: 50000000000,
		PE:        15.5,
		PB:        2.1,
		EPS:       3.45,
		Sector:    "Materials",
		Industry:  "Mining",
	}, nil
}

func (m *MockEODHDClient) GetTechnicals(ctx context.Context, ticker string, function string) (*models.TechnicalResponse, error) {
	return &models.TechnicalResponse{Data: make(map[string]interface{})}, nil
}

func (m *MockEODHDClient) GetNews(ctx context.Context, ticker string, limit int) ([]*models.NewsItem, error) {
	m.GetNewsCalls++
	if news, ok := m.News[ticker]; ok {
		return news, nil
	}
	return []*models.NewsItem{}, nil
}

func (m *MockEODHDClient) GetExchangeSymbols(ctx context.Context, exchange string) ([]*models.Symbol, error) {
	if symbols, ok := m.Symbols[exchange]; ok {
		return symbols, nil
	}
	return []*models.Symbol{}, nil
}

// MockNavexaClient implements NavexaClient for testing
type MockNavexaClient struct {
	Portfolios []models.NavexaPortfolio
	Holdings   map[string][]*models.NavexaHolding
}

// NewMockNavexaClient creates a mock Navexa client
func NewMockNavexaClient() *MockNavexaClient {
	return &MockNavexaClient{
		Portfolios: []models.NavexaPortfolio{},
		Holdings:   make(map[string][]*models.NavexaHolding),
	}
}

func (m *MockNavexaClient) GetPortfolios(ctx context.Context) ([]*models.NavexaPortfolio, error) {
	result := make([]*models.NavexaPortfolio, len(m.Portfolios))
	for i := range m.Portfolios {
		result[i] = &m.Portfolios[i]
	}
	return result, nil
}

func (m *MockNavexaClient) GetPortfolio(ctx context.Context, portfolioID string) (*models.NavexaPortfolio, error) {
	for i := range m.Portfolios {
		if m.Portfolios[i].ID == portfolioID {
			return &m.Portfolios[i], nil
		}
	}
	return nil, nil
}

func (m *MockNavexaClient) GetHoldings(ctx context.Context, portfolioID string) ([]*models.NavexaHolding, error) {
	if holdings, ok := m.Holdings[portfolioID]; ok {
		return holdings, nil
	}
	return []*models.NavexaHolding{}, nil
}

func (m *MockNavexaClient) GetPerformance(ctx context.Context, portfolioID string) (*models.NavexaPerformance, error) {
	return &models.NavexaPerformance{
		PortfolioID:      portfolioID,
		TotalReturn:      10000,
		TotalReturnPct:   12.5,
		AnnualisedReturn: 8.5,
	}, nil
}

// MockGeminiClient implements GeminiClient for testing
type MockGeminiClient struct {
	Responses map[string]string
}

// NewMockGeminiClient creates a mock Gemini client
func NewMockGeminiClient() *MockGeminiClient {
	return &MockGeminiClient{
		Responses: make(map[string]string),
	}
}

func (m *MockGeminiClient) GenerateContent(ctx context.Context, prompt string) (string, error) {
	if resp, ok := m.Responses[prompt]; ok {
		return resp, nil
	}
	return "Mock AI analysis: The stock shows mixed signals. Consider monitoring support levels.", nil
}

func (m *MockGeminiClient) GenerateWithURLContext(ctx context.Context, prompt string, urls []string) (string, error) {
	return m.GenerateContent(ctx, prompt)
}

func (m *MockGeminiClient) AnalyzeStock(ctx context.Context, ticker string, data *models.StockData) (string, error) {
	return "Mock analysis for " + ticker + ": Shows potential for recovery based on technical indicators.", nil
}

// Helper functions

func generateSampleBars(days int) []models.EODBar {
	bars := make([]models.EODBar, days)
	basePrice := 50.0
	for i := 0; i < days; i++ {
		date := time.Now().AddDate(0, 0, -i)
		// Simple random walk
		change := (float64(i%10) - 5) * 0.1
		price := basePrice + change
		bars[i] = models.EODBar{
			Date:     date,
			Open:     price - 0.5,
			High:     price + 1.0,
			Low:      price - 1.0,
			Close:    price,
			AdjClose: price,
			Volume:   1000000 + int64(i*10000),
		}
	}
	return bars
}
